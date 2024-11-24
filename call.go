package crpc

import "github.com/ndsky1003/crpc/v2/header"

type Call struct {
	Service string
	Module  string
	Method  string
	Req     any
	Ret     any
	Err     error
	opt     *Option
	Done    chan *Call
}

func (this *Call) done() {
	select {
	case this.Done <- this:
	default:
	}
}

type send_msg struct {
	h    *header.Header
	meta any
	body any
}
