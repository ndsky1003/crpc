package crpc

type Call struct {
	Service string
	Module  string
	Method  string
	Req     any
	Ret     []byte
	ErrRet  []byte //响应回来的错误
	Err     error  //发送触发的错误
	opt     *option
	Done    chan *Call
}

func (this *Call) done() {
	select {
	case this.Done <- this:
	default:
	}
}
