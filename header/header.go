package header

import (
	"encoding/binary"
	"errors"
	"sync"

	"github.com/ndsky1003/crpc/coder"
	"github.com/ndsky1003/crpc/compressor"
	"github.com/ndsky1003/crpc/header/headertype"
)

const (
	// MaxHeaderSize = 2 + 2 + 2 + 10 + 10 + 10 + 10 + 10 + 10 + 4 (10 refer to binary.MaxVarintLen64)
	MaxHeaderSize = 70

	Uint32Size = 4
	Uint16Size = 2
)

var UnmarshalError = errors.New("error occurred in Unmarshal")

// Header request header structure looks like:
// +----------+---------+------------+-------------------+-----------------+-------------------+-----------------+----------+------------+----------+
// |HeaderType|CoderType|CompressType|    FromService    | ToService       |      Module       |  Method	     |    Seq   | RequestLen | Checksum |
// +----------+---------+------------+-------------------+-----------------+-------------------+-----------------+----------+------------+----------+
// | uint16   | uint16  |   uint16   |  uvarint+ string  | uvarint+ string | uvarint + string  | uvarint +string |  uvarint |   uvarint  |  uint32  |
// +----------+---------+------------+-------------------+-----------------+-------------------+-----------------+----------+------------+----------+
type Header struct {
	sync.RWMutex
	Type         headertype.Type
	CoderType    coder.CoderType
	CompressType compressor.CompressType
	FromService  string //来源服务器
	ToService    string //目的服务器
	Module       string
	Method       string
	Seq          uint64
	BodyLen      uint64
	Checksum     uint32
}

func (this *Header) Init(t headertype.Type, from_service, to_service, module, method string, seq uint64) {
	this.Type = t
	this.FromService = from_service
	this.ToService = to_service
	this.Module = module
	this.Method = method
	this.Seq = seq
}

// Marshal will encode request header into a byte slice
func (r *Header) Marshal() []byte {
	r.RLock()
	defer r.RUnlock()
	idx := 0
	header := make([]byte, MaxHeaderSize+len(r.FromService)+len(r.ToService)+len(r.Module)+len(r.Method))

	binary.LittleEndian.PutUint16(header[idx:], uint16(r.Type))
	idx += Uint16Size

	binary.LittleEndian.PutUint16(header[idx:], uint16(r.CoderType))
	idx += Uint16Size

	binary.LittleEndian.PutUint16(header[idx:], uint16(r.CompressType))
	idx += Uint16Size

	idx += writeString(header[idx:], r.FromService)

	idx += writeString(header[idx:], r.ToService)

	idx += writeString(header[idx:], r.Module)

	idx += writeString(header[idx:], r.Method)

	idx += binary.PutUvarint(header[idx:], r.Seq)
	idx += binary.PutUvarint(header[idx:], r.BodyLen)
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
	r.Type = headertype.Type(binary.LittleEndian.Uint16(data[idx:]))
	idx += Uint16Size

	r.CoderType = coder.CoderType(binary.LittleEndian.Uint16(data[idx:]))
	idx += Uint16Size

	r.CompressType = compressor.CompressType(binary.LittleEndian.Uint16(data[idx:]))
	idx += Uint16Size

	r.FromService, size = readString(data[idx:])
	idx += size

	r.ToService, size = readString(data[idx:])
	idx += size

	r.Module, size = readString(data[idx:])
	idx += size

	r.Method, size = readString(data[idx:])
	idx += size

	r.Seq, size = binary.Uvarint(data[idx:])
	idx += size

	length, size := binary.Uvarint(data[idx:])
	r.BodyLen = length
	idx += size

	r.Checksum = binary.LittleEndian.Uint32(data[idx:])

	return
}

func (r *Header) GetCoderType() coder.CoderType {
	r.RLock()
	defer r.RUnlock()
	return r.CoderType
}

func (r *Header) GetCompressType() compressor.CompressType {
	r.RLock()
	defer r.RUnlock()
	return r.CompressType
}

func (r *Header) Release() {
	Release(r)
}
func (r *Header) Reset() {
	r.Lock()
	defer r.Unlock()
	r.Type = 0
	r.Seq = 0
	r.Checksum = 0
	r.FromService = ""
	r.ToService = ""
	r.Module = ""
	r.Method = ""
	r.CoderType = 0
	r.CompressType = 0
	r.BodyLen = 0
}

// MaxResHeaderSize = 2 + 2 + 2 + 10 + 10 + 10 + 10  + 4 (10 refer to binary.MaxVarintLen64)
// ResponseHeader request header structure looks like:
// +------+-----------+--------------+-----------------+-------+----------------+---------+----------+
// | Type | CoderType | CompressType |    ToService    |  Seq  |      Error     | BodyLen | Checksum |
// +------+-----------+--------------+-----------------+-------+----------------+---------+----------+
// |uint16|   uint16  |   uint16     |  uvarint+string |uvarint| uvarint+string | uvarint  |  uint32 |
// +------+-----------+--------------+-----------------+-------+----------------+----------+---------+
//type ResponseHeader struct {
//sync.RWMutex
//Type         headertype.Type
//CoderType    coder.CoderType
//CompressType compressor.CompressType
//ToService    string //目的服务器
//Error        string
//Seq          uint64
//BodyLen      uint64
//Checksum     uint32
//}

//func (this *ResponseHeader) Reset() {
//this.Lock()
//defer this.Unlock()
//this.Type = 0
//this.CoderType = 0
//this.CompressType = 0
//this.ToService = ""
//this.Seq = 0
//this.Error = ""
//this.BodyLen = 0
//this.Checksum = 0
//}

//// Marshal will encode response header into a byte slice
//func (r *ResponseHeader) Marshal() []byte {
//r.RLock()
//defer r.RUnlock()
//idx := 0
//header := make([]byte, MaxHeaderSize+len(r.Error)+len(r.ToService))

//binary.LittleEndian.PutUint16(header[idx:], uint16(r.Type))
//idx += Uint16Size

//binary.LittleEndian.PutUint16(header[idx:], uint16(r.CoderType))
//idx += Uint16Size

//binary.LittleEndian.PutUint16(header[idx:], uint16(r.CompressType))
//idx += Uint16Size

//idx += writeString(header[idx:], r.ToService)
//idx += binary.PutUvarint(header[idx:], r.Seq)
//idx += writeString(header[idx:], r.Error)
//idx += binary.PutUvarint(header[idx:], uint64(r.BodyLen))
//binary.LittleEndian.PutUint32(header[idx:], r.Checksum)
//idx += Uint32Size
//return header[:idx]
//}

//// Unmarshal will decode response header into a byte slice
//func (r *ResponseHeader) Unmarshal(data []byte) (err error) {
//r.Lock()
//defer r.Unlock()
//if len(data) == 0 {
//return UnmarshalError
//}

//defer func() {
//if r := recover(); r != nil {
//err = UnmarshalError
//}
//}()
//idx, size := 0, 0
//r.Type = headertype.Type(binary.LittleEndian.Uint16(data[idx:]))
//idx += Uint16Size

//r.CoderType = coder.CoderType(binary.LittleEndian.Uint16(data[idx:]))
//idx += Uint16Size

//r.CompressType = compressor.CompressType(binary.LittleEndian.Uint16(data[idx:]))
//idx += Uint16Size

//r.ToService, size = readString(data[idx:])
//idx += size

//r.Seq, size = binary.Uvarint(data[idx:])
//idx += size

//r.Error, size = readString(data[idx:])
//idx += size

//length, size := binary.Uvarint(data[idx:])
//r.BodyLen = length
//idx += size

//r.Checksum = binary.LittleEndian.Uint32(data[idx:])
//return
//}

//// GetCompressType get compress type
//func (r *ResponseHeader) GetCompressType() compressor.CompressType {
//r.RLock()
//defer r.RUnlock()
//return compressor.CompressType(r.CompressType)
//}

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
