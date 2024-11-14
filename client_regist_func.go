package crpc

import (
	"errors"
	"fmt"
	"reflect"
)

func (this *Client) RegisterFunc(funcname string, function any) error {
	if funcname == "" {
		return errors.New("name is empty")
	}
	return this.register_func(funcname, function)
}

const func_module_name = "func"

func (this *Client) register_func(name string, function any) error {
	mname := name
	mvalue := reflect.ValueOf(function)
	mtype := mvalue.Type()

	if mtype.Kind() != reflect.Func {
		return errors.New("rpc.Register: " + name + " not a func")
	}
	// Method needs three ins: *meta *args.
	argsNum := mtype.NumIn()
	if !(argsNum == 1 || argsNum == 2) {
		err := fmt.Errorf("rpc.Register: method %q has %d input parameters; needs exactly 1 or 2\n", mname, mtype.NumIn())
		return err
	}
	var argsIndex int
	// First arg need not be a pointer.
	var metaType reflect.Type
	if argsNum == 2 {
		metaType = mtype.In(argsIndex)
		if !isExportedOrBuiltinType(metaType) {
			err := fmt.Errorf("rpc.Register: argument type of method %q is not exported: %q\n", mname, metaType)
			return err
		}
		argsIndex++
	}
	// Second arg must be a pointer.
	argType := mtype.In(argsIndex)
	// Reply type must be exported.
	if !isExportedOrBuiltinType(argType) {
		err := fmt.Errorf("rpc.Register: args type of method %q is not exported: %q\n", mname, argType)
		return err
	}

	// Method needs one or two out. (error) or (ret,error)
	retNum := mtype.NumOut()
	if !(retNum == 1 || retNum == 2) {
		err := fmt.Errorf("rpc.Register: method %q has %d output parameters; needs exactly one or two\n", mname, retNum)
		return err
	}
	// The return type of the method must be error.
	if returnType := mtype.Out(retNum - 1); returnType != typeOfError {
		err := fmt.Errorf("rpc.Register: return type of method %q is %q, must be error\n", mname, returnType)
		return err
	}

	method := reflect.Method{
		Name: mname,
		Type: mtype,
		Func: mvalue,
	}
	mt := &methodType{method: method, is_func: true, ArgsNum: uint8(argsNum), MetaType: metaType, ArgType: argType, RetNum: uint8(retNum)}
	if retNum == 2 {
		mt.RetType = mtype.Out(0)
	}
	if v, ok := this.moduleMap.Load(func_module_name); ok {
		if vv, ok1 := v.(*module); ok1 {
			vv.methods[mname] = mt
		}
	} else {
		func_module := &module{
			name:    func_module_name,
			methods: map[string]*methodType{},
		}
		func_module.methods[mname] = mt
		this.moduleMap.Store(func_module_name, func_module)
	}
	return nil
}
