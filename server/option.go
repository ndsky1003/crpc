package server

import (
	"github.com/ndsky1003/crpc/v3/comm/ut"
	"github.com/ndsky1003/net/server"
)

func Options() *Option {
	return &Option{}
}

type Option struct {
	Secret        *string
	GroupReplicas *int
	server.Option
}

func (this *Option) WithConn(fn func(*server.Option)) *Option {
	if fn != nil {
		fn(&this.Option)
	}
	return this
}

func (this *Option) SetSecret(s string) *Option {
	this.Secret = &s
	return this
}

func (this *Option) SetGroupReplicas(s int) *Option {
	this.GroupReplicas = &s
	return this
}

func (this *Option) merge(delta *Option) *Option {
	if this == nil || delta == nil {
		return nil
	}

	ut.ResolveOption(&this.Secret, delta.Secret)
	ut.ResolveOption(&this.GroupReplicas, delta.GroupReplicas)

	this.Option = this.Option.Merge(&delta.Option)
	return this
}

func (this Option) Merge(opts ...*Option) Option {
	for _, opt := range opts {
		this.merge(opt)
	}
	return this
}
