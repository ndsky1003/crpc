package coder

import "github.com/vmihailenco/msgpack/v5"

type msg_pack struct {
}

func new_msg_pack() *msg_pack {
	return new(msg_pack)
}

func (this *msg_pack) Marshal(v any) ([]byte, error) {
	return msgpack.Marshal(v)
}

func (this *msg_pack) Unmarshal(data []byte, v any) error {
	return msgpack.Unmarshal(data, v)
}
