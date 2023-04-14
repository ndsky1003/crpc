package serializer

import "github.com/ndsky1003/crpc/header"

// 持久序列化 -> adapter 适配器
type Serializer interface {
	Serialize(*header.Header, []byte) error //header,body
	Deserialize() (*header.Header, []byte, error)
}
