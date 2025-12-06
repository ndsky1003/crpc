//go:generate msgp --tests=false
package protocol

import "fmt"

type VerifyReq struct {
	Name   string
	Weight int
}
type VerifyRes struct {
	Message string
}

type Error struct {
	Code int32  `msg:"c"` // 错误码 (用于程序逻辑判断，如 1001=UserNotFound)
	Msg  string `msg:"m"` // 错误信息 (用于日志和人类阅读)
	Data []byte `msg:"d"` // (可选) 附加数据，例如具体的校验失败字段
}

// 实现 error 接口，这样它在服务端可以被当做普通 error 返回
func (e *Error) Error() string {
	return fmt.Sprintf("rpc_err code=%d msg=%s", e.Code, e.Msg)
}

// 辅助构造函数
func NewError(code int32, msg string) *Error {
	return &Error{Code: code, Msg: msg}
}
