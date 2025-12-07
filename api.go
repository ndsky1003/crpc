package crpc

import (
	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/crpc/v3/protocol/errors"
)

func RegisterCoder(t coder.T, c coder.Coder) {
	coder.RegisterCoder(t, c)
}

type Error = errors.Error

func NewError(code uint16, msg string, args ...any) *Error {
	return errors.New(code, msg, args...)
}
