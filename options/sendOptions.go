package options

import (
	"time"

	"github.com/ndsky1003/crpc/coder"
	"github.com/ndsky1003/crpc/compressor"
	"github.com/ndsky1003/crpc/serializer"
)

type SendOptions struct {
	CoderType    *coder.CoderType
	CompressType *compressor.CompressType
	serializer.Serializer
	Timeout    *time.Duration
	ChunksSize *int
	IsSendRaw  *bool //有一种情况,就是发送的时候已经是data,就是已经marshal的数据,但是反解的时候又需要codetype.
	//eg:后台发送json数据,直到有戏服才需要unmarshal
}

func Send() *SendOptions {
	return new(SendOptions)
}

func (this *SendOptions) SetCoderType(t coder.CoderType) *SendOptions {
	if this == nil {
		return this
	}
	this.CoderType = &t
	return this
}

func (this *SendOptions) SetCompressorType(t compressor.CompressType) *SendOptions {
	if this == nil {
		return this
	}
	this.CompressType = &t
	return this
}

func (this *SendOptions) SetSerializer(s serializer.Serializer) *SendOptions {
	if this == nil {
		return this
	}
	this.Serializer = s
	return this
}

func (this *SendOptions) SetTimeout(t time.Duration) *SendOptions {
	if this == nil {
		return this
	}
	this.Timeout = &t
	return this
}

func (this *SendOptions) SetChunksMaxSize(t int) *SendOptions {
	if this == nil {
		return this
	}
	this.ChunksSize = &t
	return this
}

func (this *SendOptions) SetIsSendRaw(b bool) *SendOptions {
	if this == nil {
		return this
	}
	this.IsSendRaw = &b
	return this
}

func (this *SendOptions) Merge(opts ...*SendOptions) *SendOptions {
	for _, opt := range opts {
		this.merge(opt)
	}
	return this
}

func (this *SendOptions) merge(opt *SendOptions) {
	if opt.CoderType != nil {
		this.CoderType = opt.CoderType
	}
	if opt.CompressType != nil {
		this.CompressType = opt.CompressType
	}
	if opt.Timeout != nil {
		this.Timeout = opt.Timeout
	}
	if opt.ChunksSize != nil {
		this.ChunksSize = opt.ChunksSize
	}
	if opt.Serializer != nil {
		this.Serializer = opt.Serializer
	}

	if opt.IsSendRaw != nil {
		this.IsSendRaw = opt.IsSendRaw
	}
}

// 只覆盖nil，最终发送的属性是client有一个默认属性，发送可以具体指定发送的属性
func (this *SendOptions) OverrideNil(coderT *coder.CoderType, compressT *compressor.CompressType, serializer serializer.Serializer, timeout *time.Duration, chunkSize *int) {
	if this.CoderType == nil {
		this.CoderType = coderT
	}

	if this.CompressType == nil {
		this.CompressType = compressT
	}

	if this.Serializer == nil {
		this.Serializer = serializer
	}

	if this.Timeout == nil {
		this.Timeout = timeout
	}

	if this.ChunksSize == nil {
		this.ChunksSize = chunkSize
	}
}
