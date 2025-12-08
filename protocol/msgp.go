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
