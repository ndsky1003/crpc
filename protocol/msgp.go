//go:generate msgp --tests=false
//msgp:replace uuid.UUID [16]byte
package protocol

import (
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type VerifyReq struct {
	UUID   uuid.UUID
	Name   string
	Weight int
}

type VerifyRes struct {
	UUID    uuid.UUID
	Message string
}

//msgp:ignore JwtClaims
type JwtClaims struct {
	Data []byte `json:"d"`
	jwt.RegisteredClaims
}

// FileTransfer 用于文件传输的结构体
// 建议配合 coder.Msgp 使用以获得最佳性能
type FileTransfer struct {
	FileName string `msg:"n"` // 文件名 (相对路径)
	Data     []byte `msg:"d"` // 文件块数据
	Offset   int64  `msg:"o"` // 当前块在文件中的偏移量
	IsFinish bool   `msg:"f"` // 是否是最后一块
}
