package crpc

import (
	"errors"
	"go/token"
	"log"
	"reflect"
	"sync"
)

// copy from official rpc
func (this *Client) Register(rcvr any) error {
	return this.register(rcvr, "", false)
}

//		func (p *Person) Func(meta int, req int) (res int, err error)
//		func (p *Person) Func1(meta int, req int) (err error)
//		func (p *Person) Func2(req int) (res int, err error)
//	 func (p *Person) Func3(req int) (err error)
//
// 支持以上4种结构
func (this *Client) RegisterName(name string, rcvr any) error {
	return this.register(rcvr, name, true)
}

const logRegisterError = false

func isExportedOrBuiltinType(t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return token.IsExported(t.Name()) || t.PkgPath() == ""
}

var typeOfError = reflect.TypeOf((*error)(nil)).Elem()

type methodType struct {
	sync.Mutex // protects counters
	is_func    bool
	method     reflect.Method
	MetaType   reflect.Type
	ArgsNum    uint8
	ArgType    reflect.Type
	RetNum     uint8
}

type module struct {
	name    string                 // name of service
	rcvr    reflect.Value          // receiver of methods for the service
	typ     reflect.Type           // type of the receiver
	methods map[string]*methodType // registered methods
}

func (this *Client) register(rcvr any, name string, useName bool) error {
	if name == func_module_name {
		return errors.New("module name can not be " + func_module_name)
	}
	m := new(module)
	m.typ = reflect.TypeOf(rcvr)
	m.rcvr = reflect.ValueOf(rcvr)
	sname := name
	if !useName {
		sname = reflect.Indirect(m.rcvr).Type().Name()
	}
	if sname == "" {
		s := "rpc.Register: no service name for type " + m.typ.String()
		return errors.New(s)
	}
	if !useName && !token.IsExported(sname) {
		s := "rpc.Register: type " + sname + " is not exported"
		return errors.New(s)
	}
	m.name = sname
	// Install the methods
	m.methods = suitableMethods(m.typ, logRegisterError)

	if _, dup := this.moduleMap.LoadOrStore(sname, m); dup {
		return errors.New("rpc: service already defined: " + sname)
	}
	return nil
}

// suitableMethods returns suitable Rpc methods of typ. It will log
// errors if logErr is true.
func suitableMethods(typ reflect.Type, logErr bool) map[string]*methodType {
	methods := make(map[string]*methodType)
	for m := 0; m < typ.NumMethod(); m++ {
		method := typ.Method(m)
		mtype := method.Type
		mname := method.Name
		// Method must be exported.
		if !method.IsExported() {
			continue
		}
		// Method needs two   ins: receiver, *args
		// Method needs three ins: receiver,*meta, *args
		argsNum := mtype.NumIn()
		if !(argsNum != 2 || argsNum != 3) {
			if logErr {
				log.Printf("rpc.Register: method %q has %d input parameters; needs exactly three or two \n", mname, mtype.NumIn())
			}
			continue
		}
		var argsIndex int
		// First arg need not be a pointer.
		var metaType reflect.Type
		if argsNum == 3 {
			argsIndex++
			metaType = mtype.In(argsIndex)
			if !isExportedOrBuiltinType(metaType) {
				if logErr {
					log.Printf("rpc.Register: argument type of method %q is not exported: %q\n", mname, metaType)
				}
				continue
			}
		}

		argsIndex++
		argType := mtype.In(argsIndex)
		// Reply type must be exported.
		if !isExportedOrBuiltinType(argType) {
			if logErr {
				log.Printf("rpc.Register: reply type of method %q is not exported: %q\n", mname, argType)
			}
			continue
		}
		// Method needs one or two out. (error) or (ret,error)
		retNum := mtype.NumOut()
		if !(retNum == 1 || retNum == 2) {
			if logErr {
				log.Printf("rpc.Register: method %q has %d output parameters; needs exactly one or two\n", mname, retNum)
			}
			continue
		}
		// The return type of the method must be error.
		if returnType := mtype.Out(retNum - 1); returnType != typeOfError {
			if logErr {
				log.Printf("rpc.Register: return type of method %q is %q, must be error\n", mname, returnType)
			}
			continue
		}
		methods[mname] = &methodType{method: method, ArgsNum: uint8(argsNum), MetaType: metaType, ArgType: argType, RetNum: uint8(mtype.NumOut())}
	}
	return methods
}
