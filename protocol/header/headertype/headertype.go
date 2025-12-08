package headertype

type T uint8

// NOTE:
// 核心技巧：所有的请求类型定义为奇数
// 核心技巧：所有的响应类型定义为偶数
const (
	None T = 0
	// --- 握手与鉴权 ---
	VerifyReq T = 1
	VerifyRes T = 2

	// --- 普通 RPC ---
	Req T = 3
	Res T = 4

	// --- 广播 ---
	BroadcastReq T = 5
	BroadcastRes T = 6

	// --- 单向消息 ---
	// Send 比较特殊，它是请求但不需要回包。
	// 为了 IsReq 逻辑统一，建议把它设为奇数
	Send T = 7
)

func (t T) IsReq() bool {
	if t == None {
		return false
	}
	return t&1 == 1
}

func (t T) IsRes() bool {
	if t == None {
		return false
	}
	// 偶数判断：最后一位是 0
	return t&1 == 0
}

func (t T) String() string {
	switch t {
	case VerifyReq:
		return "VerifyReq"
	case VerifyRes:
		return "VerifyRes"
	case Req:
		return "Req"
	case Res:
		return "Res"
	case BroadcastReq:
		return "BroadcastReq"
	case BroadcastRes:
		return "BroadcastRes"
	case Send:
		return "Send" // 补上了
	case None:
		return "None"
	default:
		return "Unknown"
	}
}
