package headercode

type T uint8

const (
	OK              T = 0 // 成功
	ServiceNotFound T = 1 // 找不到服务
	MethodNotFound  T = 2 // 找不到方法
	ServerPanic     T = 3 // 服务端代码炸了
	RequestTimeout  T = 4 // 处理超时
	RateLimit       T = 5 // 被限流了
)

func (t T) IsOK() bool {
	return t == OK
}
