package coder

import (
	"fmt"
)

type Coder interface {
	Marshal(any) ([]byte, error)
	Unmarshal([]byte, any) error
}

type T uint8

const (
	Raw T = iota
	JSON
	Msgp
)

func (t T) String() string {
	switch t {
	case Raw:
		return "Raw"
	case JSON:
		return "JSON"
	case Msgp:
		return "Msgp"
	default:
		return "unknown"
	}
}

func RegisterCoder(t T, coder Coder) {
	coders[t] = coder
}

var coders = map[T]Coder{
	Raw:  new_raw_coder(),
	JSON: new_json_coder(),
	Msgp: new_msgp_coder(),
}

// NOTE: 这里放弃了zero-copy设计，因为涉及了有sendChan,这种架构下zero-copy不现实,所以返回的data，必须是新分配的内存
// 极致修复,就是使用RingBuffer的架构,就可以做到zero-copy
// 参考: gnet, evio, nbio
func Marshal(t T, v any) (data []byte, err error) {
	coder, ok := coders[t]
	if !ok {
		err = fmt.Errorf("coder:%v is not exist", t)
		return
	}
	data, err = coder.Marshal(v)
	if err != nil {
		err = fmt.Errorf("coder:%v marshal err:%w", t, err)
	}
	return
}

func Unmarshal(t T, data []byte, v any) error {
	if v == nil {
		return nil
	}
	coder, ok := coders[t]
	if !ok {
		return fmt.Errorf("coder:%d is not exist", t)
	}
	if err := coder.Unmarshal(data, v); err != nil {
		return fmt.Errorf("coder:%v unmarshal err:%v", t, err)
	}
	return nil
}
