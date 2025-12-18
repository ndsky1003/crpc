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

type dynamic_service struct {
	methods map[string]*method_meta
	sync.RWMutex
}

func new_dynamic_service() *dynamic_service {
	return &dynamic_service{
		methods: make(map[string]*method_meta),
	}
}

type method_meta struct {
	fn reflect.Value

	hasCtx   bool
	hasMeta  bool
	hasReq   bool
	metaType reflect.Type
	reqType  reflect.Type

	returnVal bool
	returnErr bool
	valIndex  int
	errIndex  int
}

// HandleMsg 动态分发
func (s *dynamic_service) HandleMsg(ctx context.Context, method string, metaCoderT coder.T, reqCoderT coder.T, metaData, bodyData any) (any, error) {
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

		// 检查是否为有效的本地对象 (非 nil 且 非 []byte)
		isLocalObj := false
		if metaData != nil {
			if _, isBytes := metaData.([]byte); !isBytes {
				isLocalObj = true
			}
		}

		if isLocalObj {
			// --- 本地调用优化: 直接使用对象，避免 reflect.New + Set ---
			val := reflect.ValueOf(metaData)

			// 使用公共函数适配类型
			adaptedVal, err := adaptValueForType(val, info.metaType)
			if err != nil {
				return nil, errors.New(errors.RemoteInternal, fmt.Sprintf("local call meta type mismatch: %v", err))
			}
			metaVal = adaptedVal
		} else {
			// --- 远程调用: 走反序列化 ---
			isPtr := info.metaType.Kind() == reflect.Pointer
			if isPtr {
				metaVal = reflect.New(info.metaType.Elem())
			} else {
				metaVal = reflect.New(info.metaType).Elem()
			}

			if bytes, ok := metaData.([]byte); ok && len(bytes) > 0 {
				ptr := metaVal
				if !isPtr {
					ptr = metaVal.Addr()
				}
				if err := coder.Unmarshal(metaCoderT, bytes, ptr.Interface()); err != nil {
					return nil, errors.New(errors.RemoteInternal, "unmarshal meta error: "+err.Error())
				}
			}
		}
		args = append(args, metaVal)
	}

	// 3. Arg: Request
	if info.hasReq {
		var reqVal reflect.Value

		// 检查是否为有效的本地对象 (非 nil 且 非 []byte)
		isLocalObj := false
		if bodyData != nil {
			if _, isBytes := bodyData.([]byte); !isBytes {
				isLocalObj = true
			}
		}

		if isLocalObj {
			// --- 本地调用优化 ---
			val := reflect.ValueOf(bodyData)

			// 使用公共函数适配类型
			adaptedVal, err := adaptValueForType(val, info.reqType)
			if err != nil {
				return nil, errors.New(errors.RemoteInternal, fmt.Sprintf("local call req type mismatch: %v", err))
			}
			reqVal = adaptedVal

		} else {
			// --- 远程调用 ---
			isPtr := info.reqType.Kind() == reflect.Pointer
			if isPtr {
				reqVal = reflect.New(info.reqType.Elem())
			} else {
				reqVal = reflect.New(info.reqType).Elem()
			}

			if bytes, ok := bodyData.([]byte); ok && len(bytes) > 0 {
				ptr := reqVal
				if !isPtr {
					ptr = reqVal.Addr()
				}
				if err := coder.Unmarshal(reqCoderT, bytes, ptr.Interface()); err != nil {
					return nil, errors.New(errors.RemoteInternal, "unmarshal req error: "+err.Error())
				}
			}
		}
		args = append(args, reqVal)
	}

	// 调用函数
	results := info.fn.Call(args)

	var res any
	var err error

	if info.returnErr {
		errVal := results[info.errIndex]
		if !errVal.IsNil() {
			err = errVal.Interface().(error)
		}
	}

	if err == nil && info.returnVal {
		resVal := results[info.valIndex]
		if resVal.IsValid() {
			res = resVal.Interface()
		}
	}

	return res, err
}

// adaptValueForType 适配值到目标类型，减少重复代码
func adaptValueForType(val reflect.Value, targetType reflect.Type) (reflect.Value, error) {
	if val.Type().AssignableTo(targetType) {
		return val, nil
	}

	if val.Kind() == reflect.Pointer && val.Elem().Type().AssignableTo(targetType) {
		// 想要值，给了指针 -> 解引用
		return val.Elem(), nil
	}

	if val.Kind() != reflect.Pointer && reflect.PointerTo(val.Type()).AssignableTo(targetType) {
		// 想要指针，给了值 -> 创建指针
		newPtr := reflect.New(val.Type())
		newPtr.Elem().Set(val)
		return newPtr, nil
	}

	return reflect.Value{}, fmt.Errorf("type mismatch: cannot assign %v to %v", val.Type(), targetType)
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

		fnVal := rcvrVal.Method(i)
		if fnVal.Kind() != reflect.Func {
			continue
		}

		meta, err := parseMethodMeta(mName, fnVal.Type())
		if err != nil {
			continue
		}
		meta.fn = fnVal

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

	var methodName string
	if len(name) > 0 && name[0] != "" {
		methodName = name[0]
	} else {
		pc := fnVal.Pointer()
		f := runtime.FuncForPC(pc)
		if f == nil {
			return fmt.Errorf("RegisterFunction: cannot get function info, please specify method name")
		}
		fullName := f.Name()
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
	ctxType := reflect.TypeOf((*context.Context)(nil)).Elem()
	errType := reflect.TypeOf((*error)(nil)).Elem()

	idx := 0

	if numIn > 0 && fnType.In(0).Implements(ctxType) {
		meta.hasCtx = true
		idx++
	}

	remaining := numIn - idx

	switch remaining {
	case 0:
	case 1:
		meta.hasReq = true
		meta.reqType = fnType.In(idx)
	case 2:
		meta.hasMeta = true
		meta.metaType = fnType.In(idx)
		meta.hasReq = true
		meta.reqType = fnType.In(idx + 1)
	default:
		return nil, fmt.Errorf("invalid argument count for %s, signature mismatch", name)
	}

	numOut := fnType.NumOut()
	switch numOut {
	case 0:
		meta.returnVal = false
		meta.returnErr = false
	case 1:
		if fnType.Out(0).Implements(errType) {
			meta.returnErr = true
			meta.errIndex = 0
		} else {
			meta.returnVal = true
			meta.valIndex = 0
		}
	case 2:
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
