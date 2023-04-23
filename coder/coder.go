package coder

// body体的序列化
type Coder interface {
	Marshal(any) ([]byte, error)
	Unmarshal([]byte, any) error
}

type CoderType uint16

const (
	JSON CoderType = iota
	MsgPack
	FilePack
	Protobuf
)

var Coders = map[CoderType]Coder{
	JSON:     new_json_coder(),
	MsgPack:  new_msg_pack(),
	FilePack: new_file_pack(),
	Protobuf: new_protobuf_pack(),
}
