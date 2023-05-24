package header

import (
	"encoding/binary"
	"sync"

	"github.com/ndsky1003/crpc/coder"
	"github.com/ndsky1003/crpc/comm"
	"github.com/ndsky1003/crpc/compressor"
	"github.com/ndsky1003/crpc/header/headertype"
)

const (
	// MaxHeaderSize = 4 + 2 + 2 + 2 + 10 + 10 + 10 + 10 + 10 + 10 + 4 (10 refer to binary.MaxVarintLen64)
	MaxHeaderSize = 74
	//防止链接异常，传入的第一个数字过大，导致耗尽系统资源，已经遇到过该问题，所以修复
	FrozeMaxHeaderSize = MaxHeaderSize + 100 + 100 + 100 + 100 //固定最大header长度,超过这个长度就属于异常数据
)

// Header request header structure looks like:
// +---------+----------+---------+------------+-------------------+-----------------+-------------------+-----------------+----------+------------+----------+
// | Version |HeaderType|CoderType|CompressType|    FromService    | ToService       |      Module       |  Method	     |    Seq   | RequestLen | Checksum |
// +---------+----------+---------+------------+-------------------+-----------------+-------------------+-----------------+----------+------------+----------+
// |  uint32 | uint16   | uint16  |   uint16   |  uvarint+ string  | uvarint+ string | uvarint + string  | uvarint +string |  uvarint |   uvarint  |  uint32  |
// +---------+----------+---------+------------+-------------------+-----------------+-------------------+-----------------+----------+------------+----------+
type Header struct {
	sync.RWMutex
	Version      uint32
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

func (this *Header) InitVersionType(v uint32, t headertype.Type) {
	this.Version = v
	this.Type = t
}

func (this *Header) InitData(v uint32, t headertype.Type, coderT coder.CoderType, compressT compressor.CompressType, from_service, to_service, module, method string, seq uint64) {
	this.Version = v
	this.Type = t
	this.CoderType = coderT
	this.CompressType = compressT
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

	binary.LittleEndian.PutUint32(header[idx:], r.Version)
	idx += comm.Uint32Size

	binary.LittleEndian.PutUint16(header[idx:], uint16(r.Type))
	idx += comm.Uint16Size

	binary.LittleEndian.PutUint16(header[idx:], uint16(r.CoderType))
	idx += comm.Uint16Size

	binary.LittleEndian.PutUint16(header[idx:], uint16(r.CompressType))
	idx += comm.Uint16Size

	idx += comm.BinaryWriteString(header[idx:], r.FromService)

	idx += comm.BinaryWriteString(header[idx:], r.ToService)

	idx += comm.BinaryWriteString(header[idx:], r.Module)

	idx += comm.BinaryWriteString(header[idx:], r.Method)

	idx += binary.PutUvarint(header[idx:], r.Seq)
	idx += binary.PutUvarint(header[idx:], r.BodyLen)
	binary.LittleEndian.PutUint32(header[idx:], r.Checksum)
	idx += comm.Uint32Size
	return header[:idx]
}

// Unmarshal will decode request header into a byte slice
func (r *Header) Unmarshal(data []byte) (err error) {
	r.Lock()
	defer r.Unlock()
	if len(data) == 0 {
		return comm.UnmarshalError
	}

	defer func() {
		if r := recover(); r != nil {
			err = comm.UnmarshalError
		}
	}()
	idx, size := 0, 0
	r.Version = binary.LittleEndian.Uint32(data[idx:])
	idx += comm.Uint32Size

	r.Type = headertype.Type(binary.LittleEndian.Uint16(data[idx:]))
	idx += comm.Uint16Size

	r.CoderType = coder.CoderType(binary.LittleEndian.Uint16(data[idx:]))
	idx += comm.Uint16Size

	r.CompressType = compressor.CompressType(binary.LittleEndian.Uint16(data[idx:]))
	idx += comm.Uint16Size

	r.FromService, size = comm.BinaryReadString(data[idx:])
	idx += size

	r.ToService, size = comm.BinaryReadString(data[idx:])
	idx += size

	r.Module, size = comm.BinaryReadString(data[idx:])
	idx += size

	r.Method, size = comm.BinaryReadString(data[idx:])
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
	r.Version = 0
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
