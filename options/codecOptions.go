package options

import (
	"github.com/ndsky1003/crpc/coder"
	"github.com/ndsky1003/crpc/compressor"
	"github.com/ndsky1003/crpc/serializer"
)

type CodecOptions struct {
	CoderType    *coder.CoderType
	CompressType *compressor.CompressType

	serializer.Serializer
}

func Codec() *CodecOptions {
	return new(CodecOptions)
}

func (this *CodecOptions) SetCoderType(t coder.CoderType) *CodecOptions {
	this.CoderType = &t
	return this
}

func (this *CodecOptions) SetCompressorType(t compressor.CompressType) *CodecOptions {
	this.CompressType = &t
	return this
}

func (this *CodecOptions) SetSerializer(s serializer.Serializer) *CodecOptions {
	this.Serializer = s
	return this
}

func (this *CodecOptions) Merge(opts ...*CodecOptions) *CodecOptions {
	for _, opt := range opts {
		this.merge(opt)
	}
	return this
}

func (this *CodecOptions) merge(opt *CodecOptions) {
	if opt.CoderType != nil {
		this.CoderType = opt.CoderType
	}
	if opt.CompressType != nil {
		this.CompressType = opt.CompressType
	}
	if opt.Serializer != nil {
		this.Serializer = opt.Serializer
	}
}
