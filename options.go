package crpc

import (
	"time"

	"github.com/ndsky1003/crpc/v2/coder"
	"github.com/ndsky1003/crpc/v2/compressor"
)

type Option struct {
	Meta any //放在header中的透传信息
	//client
	MetaCoderT    *coder.T       //meta数据的编解码器
	ReqCoderT     *coder.T       //请求数据的编解码器
	ResCoderT     *coder.T       //响应数据的编解码器,还是自定义错误的解码器,之所以请求与返回需要不同的编解码,是有文件上传的场景,上传有个自定义的编码方式
	CompressT     *compressor.T  //压缩数据的编解码器
	Timeout       *time.Duration //这个发送的超时时间,版本1是中心超市,现在做客户端超时
	CheckInterval *time.Duration //检测是否连接的间隔
	HeartInterval *time.Duration //心跳间隔,负数默认不开启心跳检测
	ChunksSize    *int           //发送文件时,每次分片文件大小
	RetErr        error          //返回一个自定义的错误
	Weight        *int           //权重 ,负数不参只保留一个链接,且绝对值越大，权重越大
	//server
	Secret *string
}

func Options() *Option {
	return new(Option)
}

func (this *Option) SetMetaData(meta any) *Option {
	if this == nil {
		return this
	}
	this.Meta = meta
	return this
}

func (this *Option) RegistRetErr(r error) *Option {
	if this == nil {
		return this
	}
	this.RetErr = r
	return this
}

func (this *Option) SetSecret(s string) *Option {
	if this == nil {
		return this
	}
	this.Secret = &s
	return this
}

func (this *Option) SetCoderT(t coder.T) *Option {
	if this == nil {
		return this
	}
	this.MetaCoderT = &t
	this.ReqCoderT = &t
	this.ResCoderT = &t
	return this
}

func (this *Option) SetMetaCoderT(t coder.T) *Option {
	if this == nil {
		return this
	}
	this.MetaCoderT = &t
	return this
}

func (this *Option) SetReqCoderT(t coder.T) *Option {
	if this == nil {
		return this
	}
	this.ReqCoderT = &t
	return this
}

func (this *Option) SetResCoderT(t coder.T) *Option {
	if this == nil {
		return this
	}
	this.ResCoderT = &t
	return this
}

func (this *Option) SetCompressT(t compressor.T) *Option {
	if this == nil {
		return this
	}
	this.CompressT = &t
	return this
}

func (this *Option) SetTimeout(t time.Duration) *Option {
	if this == nil {
		return this
	}
	this.Timeout = &t
	return this
}

func (this *Option) SetCheckInterval(t time.Duration) *Option {
	if this == nil {
		return this
	}
	this.CheckInterval = &t
	return this
}

func (this *Option) SetChunksMaxSize(t int) *Option {
	if this == nil {
		return this
	}
	this.ChunksSize = &t
	return this
}

func (this *Option) SetWeight(t int) *Option {
	if this == nil {
		return this
	}
	this.Weight = &t
	return this
}

// < 0 将没有心跳
func (this *Option) SetHeartInterval(t time.Duration) *Option {
	if this == nil {
		return this
	}
	this.HeartInterval = &t
	return this
}

func (this *Option) Merge(opts ...*Option) *Option {
	for _, opt := range opts {
		this.merge(opt)
	}
	return this
}

func (this *Option) merge(opt *Option) {
	if opt == nil {
		return
	}
	if opt.Meta != nil {
		this.Meta = opt.Meta
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

	if opt.Weight != nil {
		this.Weight = opt.Weight
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

type option_server struct {
	Secret *string
}

func OptionServer() *option_server {
	return new(option_server)
}

func (this *option_server) SetSecret(s string) *option_server {
	if this == nil {
		return this
	}
	this.Secret = &s
	return this
}

func (this *option_server) Merge(opts ...*option_server) *option_server {
	for _, opt := range opts {
		this.merge(opt)
	}
	return this
}

func (this *option_server) merge(opt *option_server) {
	if opt == nil {
		return
	}
	if opt.Secret != nil {
		this.Secret = opt.Secret
	}
}
