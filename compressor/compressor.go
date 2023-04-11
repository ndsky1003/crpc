package compressor

type Compressor interface {
	Zip([]byte) ([]byte, error)
	Unzip([]byte) ([]byte, error)
}
type CompressType uint16

const (
	Raw CompressType = iota
	Snappy
)

var Compressors = map[CompressType]Compressor{
	Raw:    NewRawCompressor(),
	Snappy: NewSnappyCompressor(),
}
