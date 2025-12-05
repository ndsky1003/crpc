package coder

import (
	"fmt"
)

type Coder interface {
	//NOTE: the release_func is used to release resources after using the marshaled data,
	// 有可能是nil，因为data底层共用,如果copy了,再拿出来就会浪费性能
	Marshal(any) ([]byte, func(), error)
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
	case JSON:
		return "JSON"
	case Msgp:
		return "Msgp"
	default:
		return "Raw"
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

func Marshal(t T, v any) (data []byte, release_func func(), err error) {
	coder, ok := coders[t]
	if !ok {
		err = fmt.Errorf("coder:%v is not exist", t)
		return
	}
	data, release_func, err = coder.Marshal(v)
	if release_func == nil {
		release_func = func() {}
	}
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
		return fmt.Errorf("coder unmarshal err:%v", err)
	}
	return nil
}
