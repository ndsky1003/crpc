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
	Msgp
	MsgPackJSONTag
	Sonic
)

var Coders = map[CoderType]Coder{
	JSON:           new_json_coder(),
	MsgPack:        new_msgpack(),
	MsgPackJSONTag: new_msgpack_with_tag("json"),
	FilePack:       new_file_pack(),
	Protobuf:       new_protobuf_pack(),
	Msgp:           new_msgp_coder(),
	Sonic:          new_sonic_coder(),
}
