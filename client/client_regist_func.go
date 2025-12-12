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
	fn reflect.Value

	// 参数相关
	hasCtx   bool         // 是否包含 context.Context 参数
	hasMeta  bool         // 是否包含 Meta 参数
	hasReq   bool         // 是否包含 Request 参数
	metaType reflect.Type // Meta参数类型
	reqType  reflect.Type // 请求参数类型

	// 返回值相关
	returnVal bool // 是否返回结果值 (Res)
	returnErr bool // 是否返回错误 (error)
	valIndex  int  // 结果值在返回列表中的索引
	errIndex  int  // 错误在返回列表中的索引
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

	// 1. Arg: Context
	if info.hasCtx {
		args = append(args, reflect.ValueOf(ctx))
	}

	// 2. Arg: Meta
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

	// 3. Arg: Request
	if info.hasReq {
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
		if len(bodyData) > 0 {
			if err := coder.Unmarshal(reqCoderT, bodyData, ptr.Interface()); err != nil {
				return nil, errors.New(errors.RemoteInternal, "unmarshal req error: "+err.Error())
			}
		}
		args = append(args, reqVal)
	}

	// 调用函数
	results := info.fn.Call(args)

	// 处理返回结果
	var res any
	var err error

	// 检查 Error
	if info.returnErr {
		errVal := results[info.errIndex]
		if !errVal.IsNil() {
			err = errVal.Interface().(error)
		}
	}

	// 检查 Value (只有在没有错误时才需要处理返回值，或者根据业务逻辑处理)
	if err == nil && info.returnVal {
		resVal := results[info.valIndex]
		if resVal.IsValid() {
			res = resVal.Interface()
		}
	}

	return res, err
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
		mName := method.Name

		if method.PkgPath != "" {
			continue
		}

		// 注意：rcvrVal.Method(i) 绑定了接收者，所以解析时 fnType 入参里不包含 receiver
		fnVal := rcvrVal.Method(i)
		if fnVal.Kind() != reflect.Func {
			continue
		}

		meta, err := parseMethodMeta(mName, fnVal.Type())
		if err != nil {
			// 严格模式下可以报错，这里选择跳过不符合签名的方法，但在日志里最好体现
			// fmt.Printf("crpc: method %s skipped: %v\n", mName, err)
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

// parseMethodMeta 解析函数签名
// 规则：
// 入参:
// 0个: ()
// 1个: (Ctx) 或 (Req)
// 2个: (Ctx, Req) 或 (Meta, Req)
// 3个: (Ctx, Meta, Req)
//
// 出参:
// 0个: ()
// 1个: (error) 或 (Res)
// 2个: (Res, error)
func parseMethodMeta(name string, fnType reflect.Type) (*method_meta, error) {
	meta := &method_meta{}
	numIn := fnType.NumIn()
	ctxType := reflect.TypeOf((*context.Context)(nil)).Elem()
	errType := reflect.TypeOf((*error)(nil)).Elem()

	idx := 0

	// 1. 解析第一个参数，判断是否为 Context
	if numIn > 0 && fnType.In(0).Implements(ctxType) {
		meta.hasCtx = true
		idx++
	}

	// 计算剩余参数数量
	remaining := numIn - idx

	switch remaining {
	case 0:
		// (Ctx) 或 ()
		// 无 Meta, 无 Req
	case 1:
		// (Ctx, Req) 或 (Req)
		// 规则：剩下的这一个一定是 Req (因为如果是 Meta，后面必须跟 Req，构不成1个参数的情况)
		// 除非你想支持 (Ctx, Meta) 但没有 Req，这在 RPC 里比较少见，通常认为单个就是 Payload
		meta.hasReq = true
		meta.reqType = fnType.In(idx)
	case 2:
		// (Ctx, Meta, Req) 或 (Meta, Req)
		meta.hasMeta = true
		meta.metaType = fnType.In(idx)
		meta.hasReq = true
		meta.reqType = fnType.In(idx + 1)
	default:
		return nil, fmt.Errorf("invalid argument count for %s, signature mismatch", name)
	}

	// --- 解析返回值 ---
	numOut := fnType.NumOut()
	switch numOut {
	case 0:
		// func()
		meta.returnVal = false
		meta.returnErr = false
	case 1:
		// func() error 或 func() Res
		if fnType.Out(0).Implements(errType) {
			meta.returnErr = true
			meta.errIndex = 0
		} else {
			meta.returnVal = true
			meta.valIndex = 0
		}
	case 2:
		// func() (Res, error)
		// 强制要求第二个是 error
		if !fnType.Out(1).Implements(errType) {
			return nil, fmt.Errorf("last return value for %s must be error in 2-return signature", name)
		}
		meta.returnVal = true
		meta.valIndex = 0
		meta.returnErr = true
		meta.errIndex = 1
	default:
		return nil, fmt.Errorf("invalid return values count for %s, support 0, 1 or 2", name)
	}

	return meta, nil
}
