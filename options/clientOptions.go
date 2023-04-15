package options

import (
	"time"

	"github.com/ndsky1003/crpc/coder"
	"github.com/ndsky1003/crpc/compressor"
	"github.com/ndsky1003/crpc/serializer"
)

type ClientOptions struct {
	CoderType    *coder.CoderType
	CompressType *compressor.CompressType
	serializer.Serializer

	Timeout       *time.Duration
	CheckInterval *time.Duration
	HeartInterval *time.Duration
	ChunksSize    *int
	IsStopHeart   *bool
	Secret        *string
}

func Client() *ClientOptions {
	return new(ClientOptions)
}

func (this *ClientOptions) SetSecret(s string) *ClientOptions {
	if this == nil {
		return this
	}
	this.Secret = &s
	return this
}

func (this *ClientOptions) SetCoderType(t coder.CoderType) *ClientOptions {
	if this == nil {
		return this
	}
	this.CoderType = &t
	return this
}

func (this *ClientOptions) SetCompressorType(t compressor.CompressType) *ClientOptions {
	if this == nil {
		return this
	}
	this.CompressType = &t
	return this
}

func (this *ClientOptions) SetSerializer(s serializer.Serializer) *ClientOptions {
	this.Serializer = s
	return this
}

func (this *ClientOptions) SetTimeout(t time.Duration) *ClientOptions {
	if this == nil {
		return this
	}
	this.Timeout = &t
	return this
}

func (this *ClientOptions) SetCheckInterval(t time.Duration) *ClientOptions {
	if this == nil {
		return this
	}
	this.CheckInterval = &t
	return this
}

func (this *ClientOptions) SetChunksMaxSize(t int) *ClientOptions {
	if this == nil {
		return this
	}
	this.ChunksSize = &t
	return this
}

func (this *ClientOptions) SetHeartInterval(t time.Duration) *ClientOptions {
	if this == nil {
		return this
	}
	this.HeartInterval = &t
	return this
}

func (this *ClientOptions) SetIsStopHeart(is bool) *ClientOptions {
	if this == nil {
		return this
	}
	this.IsStopHeart = &is
	return this
}

func (this *ClientOptions) Merge(opts ...*ClientOptions) *ClientOptions {
	for _, opt := range opts {
		this.merge(opt)
	}
	return this
}

func (this *ClientOptions) merge(opt *ClientOptions) {
	if opt.CoderType != nil {
		this.CoderType = opt.CoderType
	}
	if opt.CompressType != nil {
		this.CompressType = opt.CompressType
	}
	if opt.Secret != nil {
		this.Secret = opt.Secret
	}
	if opt.Timeout != nil {
		this.Timeout = opt.Timeout
	}
	if opt.CheckInterval != nil {
		this.CheckInterval = opt.CheckInterval
	}
	if opt.ChunksSize != nil {
		this.ChunksSize = opt.ChunksSize
	}
	if opt.HeartInterval != nil {
		this.HeartInterval = opt.HeartInterval
	}
	if opt.IsStopHeart != nil {
		this.IsStopHeart = opt.IsStopHeart
	}
	if opt.Serializer != nil {
		this.Serializer = opt.Serializer
	}
}
