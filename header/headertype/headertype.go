package headertype

type Type uint16

const (
	Ping Type = 1 << iota
	Pong
	Verify //用于连接校验
	Req
	Reply_Success
	Reply_Error
	Msg //MQ

	Res = Reply_Success | Reply_Error //最底部
)

var m = map[Type]string{

	Ping:          "Ping",
	Pong:          "Pong",
	Verify:        "Verify",
	Req:           "Req",
	Reply_Success: "Reply_Success",
	Reply_Error:   "Reply_Error",
	Msg:           "Msg",
	Res:           "Res",
}

func (this Type) String() string {
	return m[this]
}
