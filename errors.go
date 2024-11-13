package crpc

import "errors"

var (
	VerifyError             = errors.New("Client VerifyError")
	ReadError               = errors.New("Client ReadError")
	ServerReadError         = errors.New("Server ReadError")
	UnzipError              = errors.New("Client UnzipError")
	WriteError              = errors.New("Client WriteError")
	ModuleFuncError         = errors.New("Client ModuleFunc must like rpc.func")
	ServerError             = errors.New("ServerError")
	FuncError               = errors.New("FuncError")
	ReqTimeOutError         = errors.New("ReqTimeoutError")
	ErrCoderRawBodyMustData = errors.New("CoderRawBodyMustData")
	ErrCusstomNoReceiveType = errors.New("ErrCusstomNoReceiveType")
)
