package compressor

type raw_compressor struct {
}

func NewRawCompressor() *raw_compressor {
	return new(raw_compressor)
}

func (_ *raw_compressor) Zip(data []byte) (ret []byte, err error) {
	return data, nil
}

func (_ *raw_compressor) Unzip(data []byte) ([]byte, error) {
	return data, nil
}
