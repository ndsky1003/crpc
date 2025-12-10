package headerflags

type T uint8

const None T = 0
const (
	Debug     T = 1 << iota //这条数据是否打开调试日志
	UUID                    //表示头文件信息里是否携带UUID,转发回路的时候需要用到,需要知道回到哪个UUID,UUID标识一个tcpid
	EOS                     //End-Of-Stream
	Broadcast               //这条是广播消息
	Handshake               //握手/鉴权
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

func (f T) IsBroadcast() bool {
	return f.Has(Broadcast)
}

func (f T) IsHandshake() bool {
	return f.Has(Handshake)
}
