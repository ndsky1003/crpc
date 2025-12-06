package coder

import (
	"bytes"
	"errors"
	"reflect"
	"sync"

	"github.com/tinylib/msgp/msgp"
	"github.com/vmihailenco/msgpack/v5"
)

type msgp_coder struct {
}

func new_msgp_coder() *msgp_coder {
	return &msgp_coder{}
}

var pool = sync.Pool{
	New: func() any {
		return msgp.NewWriter(nil)
	},
}

func (this *msgp_coder) Marshal(v any) ([]byte, error) {
	if v == nil {
		return []byte{0xC0}, nil
	}

	// 1. 优先尝试 Marshaler (追加模式)
	if value, ok := v.(msgp.Marshaler); ok {
		data, err := value.MarshalMsg(nil)
		if err != nil {
			return nil, err
		}
		return data, nil
	}

	var buf bytes.Buffer
	// 2. 只有当不支持 Marshaler 时，才降级使用 Encodable (流式)
	if value, ok := v.(msgp.Encodable); ok {
		w := pool.Get().(*msgp.Writer)
		w.Reset(&buf) // 重定向输出到 buf
		err := value.EncodeMsg(w)
		if err == nil {
			err = w.Flush()
		}
		pool.Put(w)
		if err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}

	// 3. 最后降级到反射 (msgpack)
	enc := msgpack.GetEncoder()
	defer msgpack.PutEncoder(enc)
	enc.Reset(&buf)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (this *msgp_coder) Unmarshal(data []byte, v any) error {
	if v == nil {
		return nil
	}

	// 1. 优先尝试 Unmarshaler (直接解析)
	// 没有 io.Reader 包装，没有 buffer copy，速度最快
	if value, ok := v.(msgp.Unmarshaler); ok {
		_, err := value.UnmarshalMsg(data)
		return err
	}

	// 2. 处理 nil 情况
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

	// 3. 降级到 Decodable (流式)
	if value, ok := v.(msgp.Decodable); ok {
		// NewReader 会分配内存
		return value.DecodeMsg(msgp.NewReader(bytes.NewReader(data)))
	}

	// 4. 反射兜底
	dec := msgpack.GetDecoder()
	defer msgpack.PutDecoder(dec)
	dec.Reset(bytes.NewReader(data))
	return dec.Decode(v)
}
