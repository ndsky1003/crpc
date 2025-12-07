package headerstatus

type T uint8

// 1 开 0 关
const (
	OK    T = 1 << iota //数据是true/false
	Debug               //这条数据是否打开调试日志
)

func (this T) SetOn(t T) T {
	return this | t
}

func (this T) SetOff(t T) T {
	return this &^ t
}

func (this T) isOn(t T) bool {
	return this&t != 0
}

// func (this T) isOff(t T) bool {
// 	return !this.isOn(t)
// }

func (this T) IsOk() bool {
	return this.isOn(OK)
}

func (this T) IsDebug() bool {
	return this.isOn(Debug)
}
