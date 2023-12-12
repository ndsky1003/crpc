// 目前这个性能最好,不在乎其内部结构是否缺少
package coder

import (
	"bytes"
	"errors"
	"sync"

	"github.com/tinylib/msgp/msgp"
)

type msgp_coder struct {
	pool *sync.Pool
}

func new_msgp_coder() *msgp_coder {
	return &msgp_coder{
		pool: &sync.Pool{
			New: func() any {
				return &bytes.Buffer{}
			},
		},
	}
}

func (this *msgp_coder) Marshal(v any) ([]byte, error) {
	if value, ok := v.(msgp.Encodable); ok {
		buf := this.pool.Get().(*bytes.Buffer)
		defer this.pool.Put(buf)
		defer buf.Reset()
		if err := msgp.Encode(buf, value); err != nil {
			return nil, err
		}
		data := buf.Bytes()
		return data, nil
	} else {
		return nil, errors.New("not msgp.Encodable")
	}
}

func (this *msgp_coder) Unmarshal(data []byte, v any) error {
	if value, ok := v.(msgp.Decodable); ok {
		buf := this.pool.Get().(*bytes.Buffer)
		defer this.pool.Put(buf)
		defer buf.Reset()
		buf.Write(data)
		return msgp.Decode(buf, value)
	}
	return nil
}
