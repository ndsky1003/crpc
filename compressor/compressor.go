package compressor

type Compressor interface {
	Zip([]byte) ([]byte, error)
	Unzip([]byte) ([]byte, error)
}
type T uint16

const (
	Raw T = iota
	Snappy
)

var Compressors = map[T]Compressor{
	Raw:    NewRawCompressor(),
	Snappy: NewSnappyCompressor(),
}
