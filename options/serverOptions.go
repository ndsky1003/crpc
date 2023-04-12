package options

type ServerOptions struct {
}

func Server() *ServerOptions {
	return new(ServerOptions)
}

func (this *ServerOptions) Merge(opts ...*ServerOptions) *ServerOptions {
	for _, opt := range opts {
		this.merge(opt)
	}
	return this
}

func (this *ServerOptions) merge(opt *ServerOptions) {
}
