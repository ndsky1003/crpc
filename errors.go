package crpc

import "errors"

var (
	VerifyError     = errors.New("Client VerifyError")
	ReadError       = errors.New("Client ReadError")
	WriteError      = errors.New("Client WriteError")
	ModuleFuncError = errors.New("Client ModuleFunc must like rpc.func")
	ServerError     = errors.New("ServerError")
	FuncErr         = errors.New("FuncErr")
)
