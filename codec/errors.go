package codec

import "errors"

var (
	CompressorTypeMismatchError = errors.New("codec request and response Compressor type mismatch")
	UnexpectedChecksumError     = errors.New("codec unexpected checksum")
	WriteError                  = errors.New("codec WriteError")
	ReadError                   = errors.New("codec ReadError")
	ReadHeaderError             = errors.New("header size greater than FrozeMaxHeaderSize")
)
