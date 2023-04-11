package serializer

// 持久序列化 -> adapter 适配器
type Serializer interface {
	Serialize([]byte, []byte) error //header,body
	Deserialize() ([]byte, []byte, error)
}
