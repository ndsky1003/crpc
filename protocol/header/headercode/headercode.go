package headercode

type T uint8

const (
	OK                    T = iota // 成功
	Failed                         // 一般错误
	FailedServiceNotFound          // 找不到服务
	FailedMethodNotFound           // 找不到方法
	FailedServerPanic              // 服务端代码炸了
	FailedRequestTimeout           // 处理超时
	FailedRateLimit                // 被限流了
	FailedServiceUnavailable
)

func (t T) IsOK() bool {
	return t == OK
}
