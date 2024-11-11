package coder

import (
	"errors"
)

type raw_coder struct {
}

func new_raw_coder() *raw_coder {
	return new(raw_coder)
}

func (this *raw_coder) Marshal(v any) ([]byte, error) {
	if data, ok := v.([]byte); ok {
		return data, nil
	} else {
		return nil, errors.New("CoderRawBodyMustData")
	}
}

func (this *raw_coder) Unmarshal(data []byte, v any) error {
	return nil
}
