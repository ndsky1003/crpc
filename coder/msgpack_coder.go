package coder

import (
	"bytes"

	"github.com/vmihailenco/msgpack/v5"
)

type msg_pack struct {
	Tag string
}

func new_msgpack() *msg_pack {
	return new(msg_pack)
}
func new_msgpack_with_tag(tag string) *msg_pack {
	return &msg_pack{
		Tag: tag,
	}
}

func (this *msg_pack) Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := msgpack.GetEncoder()
	defer msgpack.PutEncoder(enc)
	enc.Reset(&buf)
	enc.SetCustomStructTag(this.Tag)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (this *msg_pack) Unmarshal(data []byte, v any) error {
	dec := msgpack.GetDecoder()
	defer msgpack.PutDecoder(dec)
	dec.Reset(bytes.NewReader(data))
	dec.SetCustomStructTag(this.Tag)
	return dec.Decode(v)
}
