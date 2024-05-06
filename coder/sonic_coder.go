package coder

import (
	"github.com/bytedance/sonic"
)

type sonic_coder struct {
}

func new_sonic_coder() *sonic_coder {
	return new(sonic_coder)
}

func (this *sonic_coder) Marshal(v any) ([]byte, error) {
	return sonic.Marshal(v)
}

func (this *sonic_coder) Unmarshal(data []byte, v any) error {
	return sonic.Unmarshal(data, v)
}
