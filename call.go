package crpc

type Call struct {
	Service string
	Module  string
	Method  string
	Args    any
	Reply   any
	Error   error
	Done    chan *Call
}
