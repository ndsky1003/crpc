// 目前这个性能最好,不在乎其内部结构是否缺少
// msgp 是msgpack的代码生成实现
// vmihailenco/msgpack 是msgpack的非代码实现,这2个玩意儿是大体兼容的,在一些特殊处理下还是有区别:var a []string = nil,就无法混用
// 大体上,之前遇到的不兼容的地方,都是编码使用msgpack,解码又用的是msgp
package coder

import (
	"bytes"

	"github.com/ndsky1003/buffer"
	"github.com/tinylib/msgp/msgp"
	"github.com/vmihailenco/msgpack/v5"
)

type msgp_coder struct {
}

func new_msgp_coder() *msgp_coder {
	return &msgp_coder{}
}

func (this *msgp_coder) Marshal(v any) ([]byte, error) {
	if v == nil {
		return []byte{192}, nil
	}
	buf := buffer.Get()
	defer buf.Release()
	if value, ok := v.(msgp.Encodable); ok {
		if err := msgp.Encode(buf, value); err != nil {
			return nil, err
		}
		data := buf.Bytes()
		return data, nil
	}

	enc := msgpack.GetEncoder()
	defer msgpack.PutEncoder(enc)
	enc.Reset(buf)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (this *msgp_coder) Unmarshal(data []byte, v any) error {
	if v == nil {
		return nil
	}
	if len(data) == 1 && data[0] == 192 {
		dec := msgpack.GetDecoder()
		defer msgpack.PutDecoder(dec)
		dec.Reset(bytes.NewReader(data))
		return dec.Decode(v)
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

//支持下面的所有数据类型,msgp,以及0值,nil等
// type Obj struct {
// 	Name            string
// 	TimeValue       time.Time
// 	Data            []byte
// 	Age             int
// 	Height          float64
// 	IsStudent       bool
// 	Hobbies         []string
// 	Attributes      map[string]interface{}
// 	FavoriteNumbers [5]int
// 	School          *School
// 	Grades          []Grade
// 	OptionalData    interface{}
// 	UintValue       uint
// 	Int8Value       int8
// 	Int16Value      int16
// 	Int32Value      int32
// 	Int64Value      int64
// 	Uint8Value      uint8
// 	Uint16Value     uint16
// 	Uint32Value     uint32
// 	Uint64Value     uint64
// 	ByteValue       byte
// 	RuneValue       rune
// }
