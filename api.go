package crpc

import (
	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/crpc/v3/protocol"
)

func RegisterCoder(t coder.T, c coder.Coder) {
	coder.RegisterCoder(t, c)
}

type Error = protocol.Error

func NewError(code int32, msg string) *Error {
	return protocol.NewError(code, msg)
}
