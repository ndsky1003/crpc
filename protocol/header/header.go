package header

import "github.com/ndsky1003/crpc/v3/coder"

// T 定义调用类型
type T byte

const (
	TypeVerify    T = 1 // 握手/鉴权
	TypeCall      T = 2 // 普通请求
	TypeReply     T = 3 // 响应
	TypeBroadcast T = 4 // 广播
)

// Header 协议头
type Header struct {
	Version     uint32
	Seq         uint64
	Type        T
	MetaCoderT  coder.T //meta coder
	ReqCoderT   coder.T //req coder
	ResCoderT   coder.T //res coder
	FromService string  // 来源服务名
	ToService   string  // 目标服务名
	Module      string
	Method      string // 方法名
	MetaLen     uint32
	BodyLen     uint32
	Checksum    uint32
}
