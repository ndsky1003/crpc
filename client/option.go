package client

import (
	"time"

	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/crpc/v3/comm/ut"
	"github.com/ndsky1003/crpc/v3/compressor"
	"github.com/ndsky1003/net/client"
)

func Options() *Option {
	return &Option{}
}

type Option struct {
	Meta                 any
	Name                 *string
	Weight               *int
	Secret               *string
	TraceID              *string
	VerifyJwtExpire      *time.Duration
	MetaCoderT           *coder.T
	ReqCoderT            *coder.T
	ResCoderT            *coder.T
	CompressT            *compressor.T
	Debug                *bool
	BroadcastResNewFunc  func() any            // 用于广播调用时创建返回值对象
	BroadcastResCallBack func(any, error) bool // 返回true表示继续广播,返回false表示停止广播
	client.Option
}

func (this *Option) WithConn(fn func(*client.Option)) *Option {
	if fn != nil {
		fn(&this.Option)
	}
	return this
}

func (o *Option) SetMeta(meta any) *Option {
	if o == nil {
		return o
	}
	o.Meta = meta
	return o
}

func (o *Option) SetName(name string) *Option {
	o.Name = &name
	return o
}

func (o *Option) SetSecret(secret string) *Option {
	o.Secret = &secret
	return o
}

func (o *Option) SetTraceID(traceID string) *Option {
	o.TraceID = &traceID
	return o
}

func (o *Option) SetVerifyJwtExpire(t time.Duration) *Option {
	o.VerifyJwtExpire = &t
	return o
}

func (o *Option) SetWeight(weight int) *Option {
	o.Weight = &weight
	return o
}

func (o *Option) SetMetaCoderT(t coder.T) *Option {
	if o == nil {
		return o
	}
	o.MetaCoderT = &t
	return o
}

func (o *Option) SetReqCoderT(t coder.T) *Option {
	if o == nil {
		return o
	}
	o.ReqCoderT = &t
	return o
}

func (o *Option) SetResCoderT(t coder.T) *Option {
	if o == nil {
		return o
	}
	o.ResCoderT = &t
	return o
}

func (o *Option) SetCompressT(t compressor.T) *Option {
	if o == nil {
		return o
	}
	o.CompressT = &t
	return o
}

func (o *Option) SetDebug(b bool) *Option {
	if o == nil {
		return o
	}
	o.Debug = &b
	return o
}

func (o *Option) SetBroadcaseResNewFunc(f func() any) *Option {
	if o == nil {
		return o
	}
	o.BroadcastResNewFunc = f
	return o
}

func (o *Option) SetBroadcaseResCallBack(f func(any, error) bool) *Option {
	if o == nil {
		return o
	}
	o.BroadcastResCallBack = f
	return o
}

// 自动生成merge方法
func (o *Option) merge(other *Option) *Option {
	if other == nil || o == nil {
		return o
	}
	if other.Meta != nil {
		o.Meta = other.Meta
	}
	ut.ResolveOption(&o.Name, other.Name)
	ut.ResolveOption(&o.Weight, other.Weight)
	ut.ResolveOption(&o.Secret, other.Secret)
	ut.ResolveOption(&o.TraceID, other.TraceID)
	ut.ResolveOption(&o.VerifyJwtExpire, other.VerifyJwtExpire)
	ut.ResolveOption(&o.Debug, other.Debug)
	ut.ResolveOption(&o.MetaCoderT, other.MetaCoderT)
	ut.ResolveOption(&o.ReqCoderT, other.ReqCoderT)
	ut.ResolveOption(&o.ResCoderT, other.ResCoderT)
	ut.ResolveOption(&o.CompressT, other.CompressT)
	if other.BroadcastResNewFunc != nil {
		o.BroadcastResNewFunc = other.BroadcastResNewFunc
	}

	if other.BroadcastResCallBack != nil {
		o.BroadcastResCallBack = other.BroadcastResCallBack
	}

	o.Option = o.Option.Merge(&other.Option)

	return o
}

// NOTE: 不要返回指针
// 全局对象来merge,
// 先浅拷贝全局对象,然后再修改这个拷贝对象,最后返回这个拷贝对象,这是避免逃逸到堆上,性能最高
func (o Option) Merge(opts ...*Option) Option {
	for _, opt := range opts {
		o.merge(opt)
	}
	return o
}
