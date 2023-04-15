package coder

import (
	"errors"

	"github.com/ndsky1003/crpc/dto"
)

type file_pack struct {
}

func new_file_pack() *file_pack {
	return new(file_pack)
}

func (this *file_pack) Marshal(v any) ([]byte, error) {
	switch b := v.(type) {
	case *dto.FileBody:
		return b.Marshal(), nil
	}
	return nil, errors.New("v must type  dto.FileBody")
}

func (this *file_pack) Unmarshal(data []byte, v any) error {
	switch b := v.(type) {
	case *dto.FileBody:
		return b.Unmarshal(data)
	}
	return errors.New("v must type  *dto.FileBody")
}
