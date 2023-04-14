package crpc

import "github.com/ndsky1003/crpc/header/headertype"

type Call struct {
	HeaderType headertype.Type
	Service    string
	Module     string
	Method     string
	Req        any
	Ret        any
	Error      error
	Done       chan *Call
}

func (this *Call) done() {
	select {
	case this.Done <- this:
	default:
	}
}
