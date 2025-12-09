package client

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"

	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/crpc/v3/protocol/errors"
)

// dynamic_service 动态服务容器，用于托管通过 RegisterFunction 注册的函数
// 它实现了 client_handler 接口，在 serviceMap 中扮演一个 Module 的角色
type dynamic_service struct {
	methods map[string]*method_meta
	sync.RWMutex
}

func new_dynamic_service() *dynamic_service {
	return &dynamic_service{
		methods: make(map[string]*method_meta),
	}
}

// method_meta 存储反射调用需要的元数据
type method_meta struct {
	fn       reflect.Value
	reqType  reflect.Type // 请求参数类型
	metaType reflect.Type // Meta参数类型
	hasCtx   bool         // 是否包含 context.Context 参数
	hasMeta  bool         // 是否包含 Meta 参数
}

// HandleMsg 实现 client_handler 接口，通过反射分发调用
func (s *dynamic_service) HandleMsg(ctx context.Context, method string, metaCoderT coder.T, reqCoderT coder.T, metaData, bodyData []byte) (any, error) {
	s.RLock()
	info, ok := s.methods[method]
	s.RUnlock()

	if !ok {
		return nil, errors.New(errors.RemoteInternal, fmt.Sprintf("method %s not found in dynamic module", method))
	}

	// 1. 构造参数列表
	var args []reflect.Value

	// Arg: Context
	if info.hasCtx {
		args = append(args, reflect.ValueOf(ctx))
	}

	// Arg: Meta
	if info.hasMeta {
		var metaVal reflect.Value
		// 确定是创建指针还是值
		if info.metaType.Kind() == reflect.Pointer {
			metaVal = reflect.New(info.metaType.Elem())
		} else {
			metaVal = reflect.New(info.metaType).Elem()
		}

		if len(metaData) > 0 {
			// Unmarshal 需要传入指针
			ptr := metaVal
			if metaVal.Kind() != reflect.Pointer {
				ptr = metaVal.Addr()
			}
			if err := coder.Unmarshal(metaCoderT, metaData, ptr.Interface()); err != nil {
				return nil, errors.New(errors.RemoteInternal, "unmarshal meta error: "+err.Error())
			}
		}
		args = append(args, metaVal)
	}

	// Arg: Request
	{
		var reqVal reflect.Value
		if info.reqType.Kind() == reflect.Pointer {
			reqVal = reflect.New(info.reqType.Elem())
		} else {
			reqVal = reflect.New(info.reqType).Elem()
		}

		// Unmarshal Req
		ptr := reqVal
		if reqVal.Kind() != reflect.Pointer {
			ptr = reqVal.Addr()
		}
		if err := coder.Unmarshal(reqCoderT, bodyData, ptr.Interface()); err != nil {
			return nil, errors.New(errors.RemoteInternal, "unmarshal req error: "+err.Error())
		}
		args = append(args, reqVal)
	}

	// 2. 反射调用
	results := info.fn.Call(args)

	// 3. 处理返回值
	// 约定：最后一个返回值必须是 error
	errIdx := len(results) - 1
	if errIdx < 0 {
		return nil, nil // Should not happen
	}

	// 检查 error
	if !results[errIdx].IsNil() {
		return nil, results[errIdx].Interface().(error)
	}

	// 如果有返回值 (res, error)
	if len(results) > 1 {
		return results[0].Interface(), nil
	}

	return nil, nil
}

// RegisterFunction 注册一个普通函数作为 RPC 处理程序
// moduleName: 模块名 (对应 call 时的 "module.method" 中的 module)
// fn: 处理函数，签名应类似于 func([ctx], [meta], req) ([res], err)
// name: (可选) 指定方法名。如果不填，尝试使用函数名。匿名函数必须指定。
func (c *Client) RegisterFunction(moduleName string, fn any, name ...string) error {
	fnVal := reflect.ValueOf(fn)
	fnType := fnVal.Type()

	if fnType.Kind() != reflect.Func {
		return fmt.Errorf("RegisterFunction: fn must be a function")
	}

	// 1. 确定方法名
	var methodName string
	if len(name) > 0 && name[0] != "" {
		methodName = name[0]
	} else {
		// 自动获取函数名
		pc := fnVal.Pointer()
		f := runtime.FuncForPC(pc)
		if f == nil {
			return fmt.Errorf("RegisterFunction: cannot get function info, please specify method name")
		}
		fullName := f.Name() // e.g. "pkg.MyFunc" or "pkg.func1"

		// 提取最后一部分
		if idx := strings.LastIndex(fullName, "."); idx >= 0 {
			methodName = fullName[idx+1:]
		} else {
			methodName = fullName
		}

		// 去除可能的 -fm 后缀 (method value)
		methodName = strings.TrimSuffix(methodName, "-fm")

		if methodName == "" {
			return fmt.Errorf("RegisterFunction: resolved method name is empty, please specify it")
		}
	}

	// 2. 分析参数
	numIn := fnType.NumIn()
	meta := &method_meta{
		fn: fnVal,
	}

	idx := 0
	// Check Context (Context must be the first arg if present)
	if numIn > idx && fnType.In(idx).Implements(reflect.TypeOf((*context.Context)(nil)).Elem()) {
		meta.hasCtx = true
		idx++
	}

	// Remaining args: (Meta, Req) or (Req)
	remaining := numIn - idx
	if remaining == 2 {
		meta.hasMeta = true
		meta.metaType = fnType.In(idx)
		meta.reqType = fnType.In(idx + 1)
	} else if remaining == 1 {
		meta.reqType = fnType.In(idx)
	} else {
		return fmt.Errorf("RegisterFunction: invalid argument count for %s.%s, expected [Ctx], [Meta], Req", moduleName, methodName)
	}

	// 3. 校验返回值
	numOut := fnType.NumOut()
	if numOut == 0 || numOut > 2 {
		return fmt.Errorf("RegisterFunction: return values for %s.%s must be (error) or (res, error)", moduleName, methodName)
	}
	// Last return must be error
	if !fnType.Out(numOut - 1).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		return fmt.Errorf("RegisterFunction: last return value for %s.%s must be error", moduleName, methodName)
	}

	// 4. 注册到 Client 的 serviceMap
	var svc *dynamic_service

	val, ok := c.serviceMap.Load(moduleName)
	if ok {
		if ds, ok := val.(*dynamic_service); ok {
			svc = ds
		} else {
			return fmt.Errorf("RegisterFunction: module %s already registered as a static struct/interface", moduleName)
		}
	} else {
		svc = new_dynamic_service()
		c.serviceMap.Store(moduleName, svc)
	}

	svc.Lock()
	svc.methods[methodName] = meta
	svc.Unlock()

	return nil
}
