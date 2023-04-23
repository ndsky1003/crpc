package coder

import (
	"github.com/ndsky1003/crpc/comm"
	"google.golang.org/protobuf/proto"
)

type protobuf_pack struct {
}

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
		return nil, comm.NotImplementProtoMessageError
	}
}

func (this *protobuf_pack) Unmarshal(data []byte, v any) error {
	if v == nil {
		return nil
	}
	if body, ok := v.(proto.Message); ok {
		return proto.Unmarshal(data, body)
	} else {
		return comm.NotImplementProtoMessageError
	}
}
