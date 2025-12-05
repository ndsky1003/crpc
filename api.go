package crpc

import "github.com/ndsky1003/crpc/v3/coder"

func RegisterCoder(t coder.T, c coder.Coder) {
	coder.RegisterCoder(t, c)
}
