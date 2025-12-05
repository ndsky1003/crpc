package coder

import (
	"bytes"
	"errors"
	"reflect"

	"github.com/ndsky1003/crpc/v3/buffer"
	"github.com/tinylib/msgp/msgp"
	"github.com/vmihailenco/msgpack/v5"
)

type msgp_coder struct {
}

func new_msgp_coder() *msgp_coder {
	return &msgp_coder{}
}

func (this *msgp_coder) Marshal(v any) ([]byte, func(), error) {
	if v == nil {
		return []byte{0xC0}, nil, nil
	}
	buf := buffer.Get()
	if value, ok := v.(msgp.Encodable); ok {
		if err := value.EncodeMsg(msgp.NewWriter(buf)); err != nil {
			buf.Release()
			return nil, nil, err
		}
		data := buf.Bytes()
		return data, func() {
			buf.Release()
		}, nil
	}

	enc := msgpack.GetEncoder()
	defer msgpack.PutEncoder(enc)
	enc.Reset(buf)
	if err := enc.Encode(v); err != nil {
		return nil, nil, err
	}
	return buf.Bytes(), func() { buf.Release() }, nil
}

func (this *msgp_coder) Unmarshal(data []byte, v any) error {
	if v == nil {
		return nil
	}
	if msgp.IsNil(data) {
		// 更安全的 nil 处理
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Pointer {
			if rv.IsNil() {
				return nil
			}
			// 将指针目标设置为零值
			rv.Elem().Set(reflect.Zero(rv.Elem().Type()))
			return nil
		}
		return errors.New("cannot set nil to non-pointer")
	}
	if value, ok := v.(msgp.Decodable); ok {
		return value.DecodeMsg(msgp.NewReader(bytes.NewReader(data)))
	} else {
		dec := msgpack.GetDecoder()
		defer msgpack.PutDecoder(dec)
		dec.Reset(bytes.NewReader(data))
		return dec.Decode(v)
	}
}
