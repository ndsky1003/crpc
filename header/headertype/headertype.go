package headertype

type Type = uint16

const (
	Ping Type = 1 << iota
	Pong
	Verify //用于连接校验
	Req
	Reply_Success
	Reply_Error

	Res = Reply_Success | Reply_Error //最底部
)
