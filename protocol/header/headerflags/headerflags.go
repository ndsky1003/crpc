package headerflags

type T uint8

const None T = 0
const (
	Debug T = 1 << iota //这条数据是否打开调试日志
	UUID
	EOS //End-Of-Stream
)

func (f *T) Add(flag T) *T {
	return f.With(flag)
}

func (f *T) Remove(flag T) *T {
	*f &^= flag
	return f
}

// 也可以支持链式 (可选)
func (f *T) With(flag T) *T {
	*f |= flag
	return f
}

// --- 判断操作 (值接收者) ---

func (f T) Has(flag T) bool {
	return f&flag != 0
}

func (f T) HasUUID() bool {
	return f.Has(UUID)
}

func (f T) IsDebug() bool {
	return f.Has(Debug)
}

func (f T) IsEOS() bool {
	return f.Has(EOS)
}
