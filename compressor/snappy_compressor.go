package compressor

import (
	"bytes"
	"io/ioutil"

	"github.com/golang/snappy"
)

type snappy_compressor struct {
}

func NewSnappyCompressor() *snappy_compressor {
	return new(snappy_compressor)
}

func (_ *snappy_compressor) Zip(data []byte) (ret []byte, err error) {
	var buf bytes.Buffer
	w := snappy.NewBufferedWriter(&buf)
	defer func() {
		w.Close()
	}()
	if _, err = w.Write(data); err != nil {
		return
	}
	if err = w.Flush(); err != nil {
		return
	}
	ret = buf.Bytes()
	return
}

func (_ *snappy_compressor) Unzip(data []byte) ([]byte, error) {
	r := snappy.NewReader(bytes.NewBuffer(data))
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return data, nil
}
