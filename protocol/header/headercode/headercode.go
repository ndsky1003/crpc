package headercode

type T uint8

const (
	OK     T = iota // 成功
	Failed          // 错误,具体的错误code,是通过errors里面的code定义
)

func (t T) IsOK() bool {
	return t == OK
}
