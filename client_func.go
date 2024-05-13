package crpc

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/ndsky1003/crpc/options"
)

func (this *Client) RegisterFunc(funcname string, function any) error {
	if funcname == "" {
		return errors.New("name is empty")
	}
	return this.register_func(funcname, function)
}

const func_module_name = "func"

var func_module = &module{
	name:    func_module_name,
	rcvr:    reflect.ValueOf(nil),
	typ:     reflect.TypeOf(nil),
	methods: map[string]*methodType{},
}

func (this *Client) register_func(name string, function any) error {
	mname := name
	mvalue := reflect.ValueOf(function)
	mtype := mvalue.Type()
	if mtype.Kind() != reflect.Func {
		return errors.New("rpc.Register: " + name + " not a func")
	}
	// Method needs three ins:  *args, *reply.
	if mtype.NumIn() != 2 {
		err := fmt.Errorf("rpc.Register: method %q has %d input parameters; needs exactly three\n", mname, mtype.NumIn())
		return err
	}
	// First arg need not be a pointer.
	argType := mtype.In(0)
	if !isExportedOrBuiltinType(argType) {
		err := fmt.Errorf("rpc.Register: argument type of method %q is not exported: %q\n", mname, argType)
		return err
	}
	// Second arg must be a pointer.
	replyType := mtype.In(1)
	if replyType.Kind() != reflect.Pointer {
		err := fmt.Errorf("rpc.Register: reply type of method %q is not a pointer: %q\n", mname, replyType)
		return err
	}
	// Reply type must be exported.
	if !isExportedOrBuiltinType(replyType) {
		err := fmt.Errorf("rpc.Register: reply type of method %q is not exported: %q\n", mname, replyType)
		return err
	}
	// Method needs one out.
	if mtype.NumOut() != 1 {
		err := fmt.Errorf("rpc.Register: method %q has %d output parameters; needs exactly one\n", mname, mtype.NumOut())
		return err
	}
	// The return type of the method must be error.
	if returnType := mtype.Out(0); returnType != typeOfError {
		err := fmt.Errorf("rpc.Register: return type of method %q is %q, must be error\n", mname, returnType)
		return err
	}

	method := reflect.Method{
		Name: mname,
		Type: mtype,
		Func: mvalue,
	}
	func_module.methods[mname] = &methodType{method: method, is_func: true, ArgType: argType, ReplyType: replyType}

	this.moduleMap.Store(func_module_name, func_module)
	return nil
}

func (this *Client) CallFunc(server string, function string, req, ret any, opts ...*options.SendOptions) error {
	return this.Call(server, fmt.Sprintf("%v.%v", func_module_name, function), req, ret, opts...)
}
