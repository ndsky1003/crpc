package errors

type T = uint16

// client
const (
	ClientInternal         T = 400 //参数错误
	ClientUnauthorized     T = 401 //未授权
	ClientInvalidArgs      T = 402 //参数错误
	CodeClientForbidden    T = 403 //禁止访问
	ClientNotFound         T = 404 //找不到
	ClientMethodNotAllowed T = 405 //方法禁用
	ClientTooManyRequests  T = 429 //请求过多
	ClientSendChanExhaust  T = 497 //请求队列已经耗尽
	ClientCanceled         T = 498 //请求取消
	ClientStandardError    T = 499 //客户端标准错误
)

// server
const (
	ServerInternal           T = 500 //服务器内部错误
	ServerServiceUnavailable T = 503 //服务不可用
	ServerDeadlineExceeded   T = 504 //网关超时
	ServerServiceNotFound    T = 505 //服务未找到
	ServerStandardError      T = 599 //服务器标准错误
)

// remote
const (
	RemoteInternal     T = 600 //远程内部错误
	RemoteTimeout      T = 601 //远程超时错误
	RemoteStadardError T = 699 //远程标准错误
)
