package crpc

import (
	"time"

	"github.com/ndsky1003/crpc/v2/coder"
	"github.com/ndsky1003/crpc/v2/compressor"
)

type option struct {
	CoderType    *coder.CoderType
	CompressType *compressor.CompressType

	Timeout       *time.Duration //这个发送的超时时间,版本1是中心超市,现在做客户端超时
	CheckInterval *time.Duration //检测是否连接的间隔
	HeartInterval *time.Duration //心跳间隔,负数默认不开启心跳检测
	ChunksSize    *int           //发送文件时,文件大小
	Secret        *string
}

func Option() *option {
	return new(option)
}

func (this *option) SetSecret(s string) *option {
	if this == nil {
		return this
	}
	this.Secret = &s
	return this
}

func (this *option) SetCoderType(t coder.CoderType) *option {
	if this == nil {
		return this
	}
	this.CoderType = &t
	return this
}

func (this *option) SetCompressorType(t compressor.CompressType) *option {
	if this == nil {
		return this
	}
	this.CompressType = &t
	return this
}

func (this *option) SetTimeout(t time.Duration) *option {
	if this == nil {
		return this
	}
	this.Timeout = &t
	return this
}

func (this *option) SetCheckInterval(t time.Duration) *option {
	if this == nil {
		return this
	}
	this.CheckInterval = &t
	return this
}

func (this *option) SetChunksMaxSize(t int) *option {
	if this == nil {
		return this
	}
	this.ChunksSize = &t
	return this
}

func (this *option) SetHeartInterval(t time.Duration) *option {
	if this == nil {
		return this
	}
	this.HeartInterval = &t
	return this
}

func (this *option) Merge(opts ...*option) *option {
	for _, opt := range opts {
		this.merge(opt)
	}
	return this
}

func (this *option) merge(opt *option) {
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
}
