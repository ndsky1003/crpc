package options

import "github.com/ndsky1003/crpc/serializer"

type ServerOptions struct {
	serializer.Serializer
	Secret *string
}

func Server() *ServerOptions {
	return new(ServerOptions)
}

func (this *ServerOptions) SetSerializer(s serializer.Serializer) *ServerOptions {
	this.Serializer = s
	return this
}
func (this *ServerOptions) SetSecret(s string) *ServerOptions {
	if this == nil {
		return this
	}
	this.Secret = &s
	return this
}

func (this *ServerOptions) Merge(opts ...*ServerOptions) *ServerOptions {
	for _, opt := range opts {
		this.merge(opt)
	}
	return this
}

func (this *ServerOptions) merge(opt *ServerOptions) {
	if opt.Secret != nil {
		this.Secret = opt.Secret
	}
	if opt.Serializer != nil {
		this.Serializer = opt.Serializer
	}
}
