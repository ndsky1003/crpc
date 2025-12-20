package client

import (
	"context"
	"time"

	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/crpc/v3/comm/ut"
	"github.com/ndsky1003/crpc/v3/compressor"
	"github.com/ndsky1003/net/v2/client"
)

func Options() *Option {
	return &Option{}
}

type Option struct {
	Meta                 any                         // 用户自定义元数据,会被传递到服务端
	Weight               *int                        // 权重,用于负载均衡
	Timeout              *time.Duration              // 超时时间 ,ctx的优先级更高,没有ctx则使用该值,兜底
	Secret               *string                     // 用于JWT签名的密钥
	HashKey              *string                     // 用于一致性哈希负载均衡的HashKey
	VerifyJwtExpire      *time.Duration              // 用于验证JWT过期时间,如果不设置则使用默认值15秒
	MetaCoderT           *coder.T                    // 用于元数据的编解码器类型
	ReqCoderT            *coder.T                    // 用于请求体的编解码器类型
	ResCoderT            *coder.T                    // 用于响应体的编解码器类型
	CompressT            *compressor.T               // 用于压缩算法类型
	Debug                *bool                       // 是否开启调试模式,开启后会打印更多日志
	Broadcast            *bool                       // 是否为广播调用
	BroadcastChanCap     *int                        // 广播调用时,每个响应结果的chan容量
	BroadcastResNewFunc  func() any                  // 用于广播调用时创建返回值对象
	BroadcastResCallBack func(any, error, bool) bool // 返回true表示继续广播,返回false表示停止广播
	//链路追踪 从ctx => header => net => header => ctx
	GenTraceID  func(context.Context) string                              // 用于生成TraceID的函数,发送的时候用于获取上下文中的traceid
	WithTraceID func(ctx context.Context, traceID string) context.Context // 用于将TraceID添加到context中的函数
	//链路追踪 从ctx => header => net => header => ctx
	client.Option
}

func (this *Option) WithConn(fn func(*client.Option)) *Option {
	if fn != nil {
		fn(&this.Option)
	}
	return this
}

func (this *Option) SetBroadcast() *Option {
	t := true
	this.Broadcast = &t
	return this
}

func (o *Option) SetMeta(meta any) *Option {
	if o == nil {
		return o
	}
	o.Meta = meta
	return o
}

func (o *Option) SetSecret(secret string) *Option {
	o.Secret = &secret
	return o
}

func (o *Option) SetHashKey(s string) *Option {
	o.HashKey = &s
	return o
}

func (o *Option) SetVerifyJwtExpire(t time.Duration) *Option {
	o.VerifyJwtExpire = &t
	return o
}

func (o *Option) SetTimeout(d time.Duration) *Option {
	o.Timeout = &d
	return o
}

func (o *Option) SetWeight(weight int) *Option {
	o.Weight = &weight
	return o
}

func (o *Option) SetBroadcastChanCap(a int) *Option {
	o.BroadcastChanCap = &a
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

func (o *Option) SetBroadcaseResCallBack(f func(any, error, bool) bool) *Option {
	if o == nil {
		return o
	}
	o.BroadcastResCallBack = f
	return o
}

func (o *Option) SetGenTraceID(f func(context.Context) string) *Option {
	if o == nil {
		return o
	}
	o.GenTraceID = f
	return o
}

func (o *Option) SetWithTraceID(f func(ctx context.Context, traceID string) context.Context) *Option {
	if o == nil {
		return o
	}
	o.WithTraceID = f
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
	ut.ResolveOption(&o.Weight, other.Weight)
	ut.ResolveOption(&o.BroadcastChanCap, other.BroadcastChanCap)
	ut.ResolveOption(&o.Secret, other.Secret)
	ut.ResolveOption(&o.HashKey, other.HashKey)
	ut.ResolveOption(&o.VerifyJwtExpire, other.VerifyJwtExpire)
	ut.ResolveOption(&o.Timeout, other.Timeout)
	ut.ResolveOption(&o.Debug, other.Debug)
	ut.ResolveOption(&o.MetaCoderT, other.MetaCoderT)
	ut.ResolveOption(&o.ReqCoderT, other.ReqCoderT)
	ut.ResolveOption(&o.ResCoderT, other.ResCoderT)
	ut.ResolveOption(&o.CompressT, other.CompressT)
	ut.ResolveOption(&o.Broadcast, other.Broadcast)
	if other.BroadcastResNewFunc != nil {
		o.BroadcastResNewFunc = other.BroadcastResNewFunc
	}

	if other.BroadcastResCallBack != nil {
		o.BroadcastResCallBack = other.BroadcastResCallBack
	}

	if other.GenTraceID != nil {
		o.GenTraceID = other.GenTraceID
	}

	if other.WithTraceID != nil {
		o.WithTraceID = other.WithTraceID
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
