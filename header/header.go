package header

import (
	"encoding/binary"
	"errors"
	"sync"

	"github.com/ndsky1003/crpc/compressor"
)

const (
	// MaxHeaderSize = 2 + 2 + 10 + 10 + 10 + 10 + 10 + 4 (10 refer to binary.MaxVarintLen64)
	MaxHeaderSize = 58

	Uint32Size = 4
	Uint16Size = 2
)

type header_type = uint16

const (
	header_type_req header_type = 1
	header_type_res
)

var UnmarshalError = errors.New("error occurred in Unmarshal")

// Header request header structure looks like:
// +------------+--------------+-----------------+-------------------+-----------------+----------+------------+----------+
// | HeaderType | CompressType |  Service	     |      Module       |  Method	       |    Seq   | RequestLen | Checksum |
// +------------+--------------+-----------------+-------------------+-----------------+----------+------------+----------+
// |   uint16   |    uint16    | uvarint+ string | uvarint + string  | uvarint +string |  uvarint |   uvarint  |  uint32  |
// +------------+--------------+-----------------+-------------------+-----------------+----------+------------+----------+
type Header struct {
	sync.RWMutex
	Type         header_type
	CompressType compressor.CompressType
	Service      string
	Module       string
	Method       string
	Seq          uint64
	RequestLen   uint32
	Checksum     uint32
}

// Marshal will encode request header into a byte slice
func (r *Header) Marshal() []byte {
	r.RLock()
	defer r.RUnlock()
	idx := 0
	header := make([]byte, MaxHeaderSize+len(r.Service)+len(r.Module)+len(r.Method))

	binary.LittleEndian.PutUint16(header[idx:], uint16(r.Type))
	idx += Uint16Size

	binary.LittleEndian.PutUint16(header[idx:], uint16(r.CompressType))
	idx += Uint16Size

	idx += writeString(header[idx:], r.Service)

	idx += writeString(header[idx:], r.Module)

	idx += writeString(header[idx:], r.Method)

	idx += binary.PutUvarint(header[idx:], r.Seq)
	idx += binary.PutUvarint(header[idx:], uint64(r.RequestLen))
	binary.LittleEndian.PutUint32(header[idx:], r.Checksum)
	idx += Uint32Size
	return header[:idx]
}

// Unmarshal will decode request header into a byte slice
func (r *Header) Unmarshal(data []byte) (err error) {
	r.Lock()
	defer r.Unlock()
	if len(data) == 0 {
		return UnmarshalError
	}

	defer func() {
		if r := recover(); r != nil {
			err = UnmarshalError
		}
	}()
	idx, size := 0, 0
	r.Type = binary.LittleEndian.Uint16(data[idx:])
	idx += Uint16Size

	r.CompressType = compressor.CompressType(binary.LittleEndian.Uint16(data[idx:]))
	idx += Uint16Size

	r.Service, size = readString(data[idx:])
	idx += size

	r.Module, size = readString(data[idx:])
	idx += size

	r.Method, size = readString(data[idx:])
	idx += size

	r.Seq, size = binary.Uvarint(data[idx:])
	idx += size

	length, size := binary.Uvarint(data[idx:])
	r.RequestLen = uint32(length)
	idx += size

	r.Checksum = binary.LittleEndian.Uint32(data[idx:])

	return
}

func (r *Header) GetCompressType() compressor.CompressType {
	r.RLock()
	defer r.RUnlock()
	return compressor.CompressType(r.CompressType)
}

func (r *Header) Reset() {
	r.Lock()
	defer r.Unlock()
	r.Type = 0
	r.Seq = 0
	r.Checksum = 0
	r.Service = ""
	r.Module = ""
	r.Method = ""
	r.CompressType = 0
	r.RequestLen = 0
}

func readString(data []byte) (string, int) {
	idx := 0
	length, size := binary.Uvarint(data)
	idx += size
	str := string(data[idx : idx+int(length)])
	idx += len(str)
	return str, idx
}

func writeString(data []byte, str string) int {
	idx := 0
	idx += binary.PutUvarint(data, uint64(len(str)))
	copy(data[idx:], str)
	idx += len(str)
	return idx
}
