package server

import (
	"time"

	"github.com/ndsky1003/crpc/v3/comm/ut"
	"github.com/ndsky1003/net/server"
)

func Options() *Option {
	return &Option{}
}

type Option struct {
	Secret                     *string
	GroupReplicas              *int
	SendTimeout                *time.Duration //消息上来发送给别的端点的超时时间
	BroadcastCounterExpiration *time.Duration //counter item 的过期时间
	WorkerSize                 *int
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

func (this *Option) SetWorkerSize(s int) *Option {
	this.WorkerSize = &s
	return this
}

func (this *Option) SetBroadcastCounterExpiration(s time.Duration) *Option {
	this.BroadcastCounterExpiration = &s
	return this
}

func (this *Option) SetSendTimeout(s time.Duration) *Option {
	this.SendTimeout = &s
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
	ut.ResolveOption(&this.SendTimeout, delta.SendTimeout)
	ut.ResolveOption(&this.BroadcastCounterExpiration, delta.BroadcastCounterExpiration)
	ut.ResolveOption(&this.WorkerSize, delta.WorkerSize)

	this.Option = this.Option.Merge(&delta.Option)
	return this
}

func (this Option) Merge(opts ...*Option) Option {
	for _, opt := range opts {
		this.merge(opt)
	}
	return this
}
