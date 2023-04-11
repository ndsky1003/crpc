package codec

import "github.com/ndsky1003/crpc/header"

type Request struct {
}

type Response struct {
}

// 解码器
type Codec interface {
	Write(*header.Header, any) error
	ReadHeader() (*header.Header, error)
	ReadBody(any) error
	Close() error
}
