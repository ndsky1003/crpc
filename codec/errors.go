package codec

import "errors"

var (
	CompressorTypeMismatchError = errors.New("request and response Compressor type mismatch")
	UnexpectedChecksumError     = errors.New("unexpected checksum")
)
