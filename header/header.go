package header

import (
	"encoding/binary"

	"github.com/ndsky1003/crpc/v2/coder"
	"github.com/ndsky1003/crpc/v2/comm"
	"github.com/ndsky1003/crpc/v2/compressor"
	"github.com/ndsky1003/crpc/v2/header/headertype"
)

const (
	// MaxHeaderSize = 4 + 2 + 2 + 2 + 2 + 2 + 10 + 10 + 10 + 10 +  4 + 10 + 10 + 4 (10 refer to binary.MaxVarintLen64)
	MaxHeaderSize = 82
	//防止链接异常，传入的第一个数字过大，导致耗尽系统资源，已经遇到过该问题，所以修复
	FrozeMaxHeaderSize = MaxHeaderSize + 100 + 100 + 100 + 100 //固定最大header长度,超过这个长度就属于异常数据
)

// Header request header structure looks like:
// +---------+----------+---------+---------+---------+------------+-------------------+-----------------+-------------------+-----------------+----------+----------+------------+----------+
// | Version |HeaderType|  CoderT |  CoderT |  CoderT |CompressType|    FromService    | ToService       |      Module       |   Method	       |  Seq     | MataLen  | RequestLen | Checksum |
// +---------+----------+---------+---------+---------+------------+-------------------+-----------------+-------------------+-----------------+----------+----------+------------+----------+
// |  uint32 | uint16   | uint16  | uint16  | uint16  |   uint16   |  uvarint+ string  | uvarint+ string | uvarint + string  | uvarint +string |  uvarint |  uint32  |   uvarint  |  uint32  |
// +---------+----------+---------+---------+---------+------------+-------------------+-----------------+-------------------+-----------------+----------+----------+------------+----------+
type Header struct {
	Version     uint32
	Type        headertype.T
	MetaCoderT  coder.T //meta coder //body在使用场景的情况下可能使用json,但是meta可能追求性能使用msgp来序列化
	ReqCoderT   coder.T //req coder //类似的文件发送,发送是filebody的序列化,返回值可能是json序列化
	ResCoderT   coder.T //res coder
	CompressT   compressor.T
	FromService string //来源服务器
	ToService   string //目的服务器
	Module      string
	Method      string
	Seq         uint64
	MetaLen     uint32 //meta data长度
	BodyLen     uint64
	Checksum    uint32
}

func (this *Header) SetVersion(v uint32) *Header {
	this.Version = v
	return this
}

func (this *Header) SetType(t headertype.T) *Header {
	this.Type = t
	return this
}

func (this *Header) SetMetaCoderT(t coder.T) *Header {
	this.MetaCoderT = t
	return this
}

func (this *Header) SetReqCoderT(t coder.T) *Header {
	this.ReqCoderT = t
	return this
}

func (this *Header) SetResCoderT(t coder.T) *Header {
	this.ResCoderT = t
	return this
}

func (this *Header) SetCompressT(t compressor.T) *Header {
	this.CompressT = t
	return this
}

func (this *Header) SetFromService(s string) *Header {
	this.FromService = s
	return this
}

func (this *Header) SetToService(s string) *Header {
	this.ToService = s
	return this
}

func (this *Header) SetModule(s string) *Header {
	this.Module = s
	return this
}

func (this *Header) SetMethod(s string) *Header {
	this.Method = s
	return this
}

func (this *Header) SetSeq(s uint64) *Header {
	this.Seq = s
	return this
}

func (this *Header) SetMataLen(s uint32) *Header {
	this.MetaLen = s
	return this
}

func (this *Header) SetBodyLen(s uint64) *Header {
	this.BodyLen = s
	return this
}

func (this *Header) SetChecksum(s uint32) *Header {
	this.Checksum = s
	return this
}

// Marshal will encode request header into a byte slice
func (r *Header) Marshal() []byte {
	idx := 0
	header := make([]byte, MaxHeaderSize+len(r.FromService)+len(r.ToService)+len(r.Module)+len(r.Method))

	binary.LittleEndian.PutUint32(header[idx:], r.Version)
	idx += comm.Uint32Size

	binary.LittleEndian.PutUint16(header[idx:], uint16(r.Type))
	idx += comm.Uint16Size

	binary.LittleEndian.PutUint16(header[idx:], uint16(r.MetaCoderT))
	idx += comm.Uint16Size

	binary.LittleEndian.PutUint16(header[idx:], uint16(r.ReqCoderT))
	idx += comm.Uint16Size

	binary.LittleEndian.PutUint16(header[idx:], uint16(r.ResCoderT))
	idx += comm.Uint16Size

	binary.LittleEndian.PutUint16(header[idx:], uint16(r.CompressT))
	idx += comm.Uint16Size

	idx += comm.BinaryWriteString(header[idx:], r.FromService)

	idx += comm.BinaryWriteString(header[idx:], r.ToService)

	idx += comm.BinaryWriteString(header[idx:], r.Module)

	idx += comm.BinaryWriteString(header[idx:], r.Method)

	idx += binary.PutUvarint(header[idx:], r.Seq)

	binary.LittleEndian.PutUint32(header[idx:], r.MetaLen)
	idx += comm.Uint32Size

	idx += binary.PutUvarint(header[idx:], r.BodyLen)

	binary.LittleEndian.PutUint32(header[idx:], r.Checksum)
	idx += comm.Uint32Size
	return header[:idx]
}

// Unmarshal will decode request header into a byte slice
func (r *Header) Unmarshal(data []byte) (err error) {
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

	r.Type = headertype.T(binary.LittleEndian.Uint16(data[idx:]))
	idx += comm.Uint16Size

	r.MetaCoderT = coder.T(binary.LittleEndian.Uint16(data[idx:]))
	idx += comm.Uint16Size

	r.ReqCoderT = coder.T(binary.LittleEndian.Uint16(data[idx:]))
	idx += comm.Uint16Size

	r.ResCoderT = coder.T(binary.LittleEndian.Uint16(data[idx:]))
	idx += comm.Uint16Size

	r.CompressT = compressor.T(binary.LittleEndian.Uint16(data[idx:]))
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

	r.MetaLen = binary.LittleEndian.Uint32(data[idx:])
	idx += comm.Uint32Size

	r.BodyLen, size = binary.Uvarint(data[idx:])
	idx += size

	r.Checksum = binary.LittleEndian.Uint32(data[idx:])
	return
}

func (r *Header) Release() {
	Release(r)
}
func (r *Header) Reset() {
	r.Version = 0
	r.Type = 0
	r.MetaCoderT = 0
	r.ReqCoderT = 0
	r.ResCoderT = 0
	r.CompressT = 0
	r.FromService = ""
	r.ToService = ""
	r.Module = ""
	r.Method = ""
	r.Seq = 0
	r.MetaLen = 0
	r.BodyLen = 0
	r.Checksum = 0
}

func (h *Header) GetMarshalType() coder.T {
	t := h.ReqCoderT
	if h.Type&headertype.Res != 0 {
		t = h.ResCoderT
	}
	return t
}
