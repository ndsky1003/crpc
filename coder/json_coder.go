package coder

import (
	"encoding/json"
)

type json_coder struct {
}

func new_json_coder() *json_coder {
	return new(json_coder)
}

func (this *json_coder) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (this *json_coder) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
