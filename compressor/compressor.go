package compressor

import "fmt"

type Compressor interface {
	Zip([]byte) ([]byte, error)
	Unzip([]byte) ([]byte, error)
}
type T uint8

const (
	Raw T = iota
	Snappy
)

var compressors = map[T]Compressor{
	Raw:    new_raw_compressor(),
	Snappy: NewSnappyCompressor(),
}

func Zip(t T, data []byte) ([]byte, error) {
	c, ok := compressors[t]
	if !ok {
		return nil, fmt.Errorf("compressor:%d is not exist", t)
	}
	bodyData, err := c.Zip(data)
	if err != nil {
		return nil, err
	}
	return bodyData, nil
}

func Unzip(t T, data []byte) ([]byte, error) {
	c, ok := compressors[t]
	if !ok {
		return nil, fmt.Errorf("compressor:%d is not exist", t)
	}
	return c.Unzip(data)
}
