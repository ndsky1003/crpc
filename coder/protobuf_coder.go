package coder

import (
	"errors"

	"google.golang.org/protobuf/proto"
)

type protobuf_pack struct {
}

var NotImplementProtoMessageError = errors.New("param must implement proto.Message")

func new_protobuf_pack() *protobuf_pack {
	return new(protobuf_pack)
}

func (this *protobuf_pack) Marshal(v any) ([]byte, error) {
	if v == nil {
		return []byte{}, nil
	}
	if body, ok := v.(proto.Message); ok {
		return proto.Marshal(body)
	} else {
		return nil, NotImplementProtoMessageError
	}
}

func (this *protobuf_pack) Unmarshal(data []byte, v any) error {
	if v == nil {
		return nil
	}
	if body, ok := v.(proto.Message); ok {
		return proto.Unmarshal(data, body)
	} else {
		return NotImplementProtoMessageError
	}
}
