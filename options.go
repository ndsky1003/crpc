package crpc

import (
	"time"

	"github.com/ndsky1003/crpc/v2/coder"
	"github.com/ndsky1003/crpc/v2/compressor"
)

type option struct {
	MetaData any //放在header中的透传信息
	//client
	MetaCoderT    *coder.T       //meta数据的编解码器
	ReqCoderT     *coder.T       //请求数据的编解码器
	ResCoderT     *coder.T       //响应数据的编解码器
	CompressT     *compressor.T  //压缩数据的编解码器
	Timeout       *time.Duration //这个发送的超时时间,版本1是中心超市,现在做客户端超时
	CheckInterval *time.Duration //检测是否连接的间隔
	HeartInterval *time.Duration //心跳间隔,负数默认不开启心跳检测
	ChunksSize    *int           //发送文件时,文件大小
	RetErr        error          //返回一个自定义的错误
	//server
	Secret *string
}

func Option() *option {
	return new(option)
}

func (this *option) SetMetaData(metaData any) *option {
	if this == nil {
		return this
	}
	this.MetaData = metaData
	return this
}

func (this *option) RegistRetErr(r error) *option {
	if this == nil {
		return this
	}
	this.RetErr = r
	return this
}

func (this *option) SetSecret(s string) *option {
	if this == nil {
		return this
	}
	this.Secret = &s
	return this
}

func (this *option) SetMetaCoderT(t coder.T) *option {
	if this == nil {
		return this
	}
	this.MetaCoderT = &t
	return this
}
func (this *option) SetReqCoderT(t coder.T) *option {
	if this == nil {
		return this
	}
	this.ReqCoderT = &t
	return this
}

func (this *option) SetResCoderT(t coder.T) *option {
	if this == nil {
		return this
	}
	this.ResCoderT = &t
	return this
}

func (this *option) SetCompressT(t compressor.T) *option {
	if this == nil {
		return this
	}
	this.CompressT = &t
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
	if opt.MetaData != nil {
		this.MetaData = opt.MetaData
	}

	if opt.MetaCoderT != nil {
		this.MetaCoderT = opt.MetaCoderT
	}

	if opt.ReqCoderT != nil {
		this.ReqCoderT = opt.ReqCoderT
	}

	if opt.ResCoderT != nil {
		this.ResCoderT = opt.ResCoderT
	}

	if opt.CompressT != nil {
		this.CompressT = opt.CompressT
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

	if opt.RetErr != nil {
		this.RetErr = opt.RetErr
	}

	if opt.Secret != nil {
		this.Secret = opt.Secret
	}
}
