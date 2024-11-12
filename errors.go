package crpc

import "errors"

var (
	VerifyError             = errors.New("Client VerifyError")
	ReadError               = errors.New("Client ReadError")
	UnzipError              = errors.New("Client UnzipError")
	WriteError              = errors.New("Client WriteError")
	ModuleFuncError         = errors.New("Client ModuleFunc must like rpc.func")
	ServerError             = errors.New("ServerError")
	FuncError               = errors.New("FuncError")
	ReqTimeOutError         = errors.New("ReqTimeoutError")
	ErrCoderRawBodyMustData = errors.New("CoderRawBodyMustData")
	ErrNoReceiveType        = errors.New("ErrNoReceiveType")
)
