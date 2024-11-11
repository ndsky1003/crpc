package crpc

type Call struct {
	Service string
	Module  string
	Method  string
	Req     any
	Ret     any
	Err     error
	opt     *option
	Done    chan *Call
}

func (this *Call) done() {
	select {
	case this.Done <- this:
	default:
	}
}
