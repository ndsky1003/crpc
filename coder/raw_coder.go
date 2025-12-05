package coder

import "errors"

type raw_coder struct {
}

func new_raw_coder() *raw_coder {
	return &raw_coder{}
}

func (this *raw_coder) Marshal(v any) ([]byte, func(), error) {
	if data, ok := v.([]byte); ok {
		return data, nil, nil
	} else {
		return nil, nil, errors.New("CoderRawBodyMustData Marshal")
	}
}

// WARN: data 与v 所以使用的时候要注意内存共享问题
func (this *raw_coder) Unmarshal(data []byte, v any) error {
	if d, ok := v.(*[]byte); ok {
		// *d = make([]byte, len(data))
		// copy(*d, data)
		*d = data
		return nil
	} else {
		return errors.New("CoderRawBodyMustData Unmarshal")
	}
}
