package headertype

// T 定义调用类型
type T uint8

const (
	VerifyReq T = 1 << iota // 握手/鉴权
	VerifyRes
	Req          // 普通请求
	Res          // 普通响应
	BroadcastReq // 广播请求
	BroadcastRes // 广播响应
)

var m = map[T]string{
	VerifyReq:    "VerifyReq",
	VerifyRes:    "VerifyRes",
	Req:          "Req",
	Res:          "Res",
	BroadcastReq: "BroadcastReq",
	BroadcastRes: "BroadcastRes",
}

func (this T) String() string {
	return m[this]
}

var req_all = VerifyReq | Req | BroadcastReq

func (this T) IsReq() bool {
	return this&Req != 0
}

func (this T) IsRes() bool {
	return !this.IsReq()
}
