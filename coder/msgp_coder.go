// 目前这个性能最好,不在乎其内部结构是否缺少
// msgp 是msgpack的代码生成实现
// vmihailenco/msgpack 是msgpack的非代码实现,这2个玩意儿是兼容的
package coder

import (
	"bytes"

	"github.com/tinylib/msgp/msgp"
	"github.com/vmihailenco/msgpack/v5"
)

type msgp_coder struct {
}

func new_msgp_coder() *msgp_coder {
	return &msgp_coder{}
}

func (this *msgp_coder) Marshal(v any) ([]byte, error) {
	buf := get_buffer()
	defer release_buffer(buf)
	if value, ok := v.(msgp.Encodable); ok {
		// buf := this.pool.Get().(*bytes.Buffer)
		// defer this.pool.Put(buf)
		// defer buf.Reset()
		if err := msgp.Encode(buf, value); err != nil {
			return nil, err
		}
		data := buf.Bytes()
		return data, nil
	} else {
		enc := msgpack.GetEncoder()
		defer msgpack.PutEncoder(enc)
		enc.Reset(buf)
		if err := enc.Encode(v); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
}

func (this *msgp_coder) Unmarshal(data []byte, v any) error {
	buf := get_buffer()
	defer release_buffer(buf)
	if value, ok := v.(msgp.Decodable); ok {
		// buf := this.pool.Get().(*bytes.Buffer)
		// defer this.pool.Put(buf)
		// defer buf.Reset()
		if _, err := buf.Write(data); err != nil {
			return err
		}
		return msgp.Decode(buf, value)
	} else {
		dec := msgpack.GetDecoder()
		defer msgpack.PutDecoder(dec)
		dec.Reset(bytes.NewReader(data))
		return dec.Decode(v)
	}
}
