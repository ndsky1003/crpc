//go:generate msgp --tests=false
package errors

import (
	"fmt"
)

var ModuleFuncError = New(ClientInternal, "invalid module/function format")

type Error struct {
	Code    uint16 `msg:"c"`          // 错误码 (用于程序逻辑判断，如 1001=UserNotFound)
	Msg     string `msg:"m"`          // 错误信息 (用于日志和人类阅读)
	Data    []byte `msg:"d,allownil"` // (可选) 附加数据，例如具体的校验失败字段
	TraceID string `msg:"t"`          // (可选) 分布式追踪 ID
}

// 实现 error 接口，这样它在服务端可以被当做普通 error 返回
func (e *Error) Error() string {
	if e.Code == ClientStandardError || e.Code == ServerStandardError || e.Code == RemoteStadardError {
		return e.Msg
	}
	return fmt.Sprintf("code=%d msg=%s", e.Code, e.Msg)
}

func (e *Error) WithData(data []byte) *Error {
	e.Data = data
	return e
}

func (e *Error) WithTraceID(traceID string) *Error {
	e.TraceID = traceID
	return e
}

// 辅助构造函数
func New(code T, msg string, args ...any) *Error {
	if len(args) == 0 {
		return &Error{Code: code, Msg: msg}
	}
	return &Error{Code: code, Msg: fmt.Sprintf(msg, args...)}
}
