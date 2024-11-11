package coder

import (
	"fmt"
)

type Coder interface {
	Marshal(any) ([]byte, error)
	Unmarshal([]byte, any) error
}

type T uint16

const (
	Raw T = iota
	JSON
	MsgPack
	FilePack
	Protobuf
	Msgp
	MsgPackJSONTag
	Sonic
)

var coders = map[T]Coder{
	JSON:           new_json_coder(),
	MsgPack:        new_msgpack(),
	MsgPackJSONTag: new_msgpack_with_tag("json"),
	FilePack:       new_file_pack(),
	Protobuf:       new_protobuf_pack(),
	Msgp:           new_msgp_coder(),
	Sonic:          new_sonic_coder(),
}

func Marshal(t T, v any) (data []byte, err error) {
	coder, ok := coders[t]
	if !ok {
		err = fmt.Errorf("coder:%d is not exist", t)
		return
	}
	data, err = coder.Marshal(v)
	if err != nil {
		err = fmt.Errorf("coder:%d marshal err:%v", t, err)
	}
	return
}

func Unmarshal(t T, data []byte, v any) error {
	coder, ok := coders[t]
	if !ok {
		return fmt.Errorf("coder:%d is not exist", t)
	}
	if err := coder.Unmarshal(data, v); err != nil {
		return fmt.Errorf("coder unmarshal err:%v", err)
	}
	return nil
}
