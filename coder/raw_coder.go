package coder

import "errors"

type raw_coder struct {
}

func new_raw_coder() *raw_coder {
	return &raw_coder{}
}

func (this *raw_coder) Marshal(v any) ([]byte, error) {
	if data, ok := v.([]byte); ok {
		d := make([]byte, len(data))
		copy(d, data)
		return d, nil
	} else {
		return nil, errors.New("CoderRawBodyMustData Marshal")
	}
}

func (this *raw_coder) Unmarshal(data []byte, v any) error {
	if d, ok := v.(*[]byte); ok {
		*d = make([]byte, len(data))
		copy(*d, data)
		return nil
	} else {
		return errors.New("CoderRawBodyMustData Unmarshal")
	}
}
