package headertype

type T uint16

const (
	Ping T = 1 << iota
	Pong
	Verify //用于连接校验
	Req
	Res_Success
	Res_Err_Standard //标准错误 *errors.errorString *fmt.wrapError 目前知道的就这2个
	Res_Err_Custom   //自定义错误
	Msg              //MQ
	Chunks           //发送文件的时候

	Res = Res_Success | Res_Err_Standard | Res_Err_Custom | Pong //最底部
)

var m = map[T]string{
	Ping:             "Ping",
	Pong:             "Pong",
	Verify:           "Verify",
	Req:              "Req",
	Res_Success:      "Res_Success",
	Res_Err_Standard: "Res_Err_Standard",
	Res_Err_Custom:   "Res_Err_Custom",
	Msg:              "Msg",
	Res:              "Res",
	Chunks:           "Chunks",
}

func (this T) String() string {
	return m[this]
}
