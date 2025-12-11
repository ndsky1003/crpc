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

// dynamic_service 动态服务容器，用于托管通过 RegisterFunc 或 Struct 扫描注册的函数
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

	var args []reflect.Value

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

	results := info.fn.Call(args)

	errIdx := len(results) - 1
	if errIdx < 0 {
		return nil, nil // Should not happen
	}

	if !results[errIdx].IsNil() {
		return nil, results[errIdx].Interface().(error)
	}

	if len(results) > 1 {
		return results[0].Interface(), nil
	}

	return nil, nil
}

func (c *Client) registerStructMethods(serviceName string, rcvrVal reflect.Value) error {
	rcvrType := rcvrVal.Type()

	var svc *dynamic_service
	val, ok := c.serviceMap.Load(serviceName)
	if ok {
		if ds, ok := val.(*dynamic_service); ok {
			svc = ds
		} else {
			return fmt.Errorf("crpc: service %s already registered as a different type", serviceName)
		}
	} else {
		svc = new_dynamic_service()
		c.serviceMap.Store(serviceName, svc)
	}

	svc.Lock()
	defer svc.Unlock()

	registeredCount := 0
	for i := 0; i < rcvrType.NumMethod(); i++ {
		method := rcvrType.Method(i)
		// mType := method.Type
		mName := method.Name

		if method.PkgPath != "" {
			continue
		}

		// 注意：rcvrVal.Method(i) 绑定了接收者，所以解析时 fnType 入参里不包含 receiver
		// 这与 reflect.Type.Method(i).Type 不同，后者包含 receiver
		fnVal := rcvrVal.Method(i)
		if fnVal.Kind() != reflect.Func {
			continue
		}

		// 使用 mType 获取 NumIn 等信息时需要注意，Method struct 里的 Type 包含了 Receiver
		// 但我们调用 fnVal (Bound Method) 时不需要传 Receiver。
		// 最稳妥的方式是直接分析 fnVal.Type()
		meta, err := parseMethodMeta(mName, fnVal.Type())
		if err != nil {
			continue
		}
		meta.fn = fnVal // 绑定实际调用的函数值

		svc.methods[mName] = meta
		registeredCount++
	}

	if registeredCount == 0 {
		return fmt.Errorf("crpc: %s has no exported methods satisfying rpc signature", serviceName)
	}

	return nil
}

func (c *Client) RegisterFunc(moduleName string, fn any, name ...string) error {
	fnVal := reflect.ValueOf(fn)
	if fnVal.Kind() != reflect.Func {
		return fmt.Errorf("RegisterFunction: fn must be a function")
	}

	// 1. 确定方法名
	var methodName string
	if len(name) > 0 && name[0] != "" {
		methodName = name[0]
	} else {
		pc := fnVal.Pointer()
		f := runtime.FuncForPC(pc)
		if f == nil {
			return fmt.Errorf("RegisterFunction: cannot get function info, please specify method name")
		}
		fullName := f.Name() // e.g. "pkg.MyFunc" or "pkg.func1"

		if idx := strings.LastIndex(fullName, "."); idx >= 0 {
			methodName = fullName[idx+1:]
		} else {
			methodName = fullName
		}

		methodName = strings.TrimSuffix(methodName, "-fm")

		if methodName == "" {
			return fmt.Errorf("RegisterFunction: resolved method name is empty, please specify it")
		}
	}

	meta, err := parseMethodMeta(methodName, fnVal.Type())
	if err != nil {
		return fmt.Errorf("RegisterFunction: %v", err)
	}
	meta.fn = fnVal

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

func parseMethodMeta(name string, fnType reflect.Type) (*method_meta, error) {
	meta := &method_meta{}

	numIn := fnType.NumIn()
	idx := 0

	if numIn > idx && fnType.In(idx).Implements(reflect.TypeOf((*context.Context)(nil)).Elem()) {
		meta.hasCtx = true
		idx++
	}

	remaining := numIn - idx
	switch remaining {
	case 2:
		// (Meta, Req)
		meta.hasMeta = true
		meta.metaType = fnType.In(idx)
		meta.reqType = fnType.In(idx + 1)
	case 1:
		// (Req)
		meta.reqType = fnType.In(idx)
	default:
		return nil, fmt.Errorf("invalid argument count for %s, expected [Ctx], [Meta], Req", name)
	}

	numOut := fnType.NumOut()
	if numOut == 0 || numOut > 2 {
		return nil, fmt.Errorf("return values for %s must be (error) or (res, error)", name)
	}

	if !fnType.Out(numOut - 1).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		return nil, fmt.Errorf("last return value for %s must be error", name)
	}

	return meta, nil
}
