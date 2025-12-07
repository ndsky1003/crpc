package header

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/crpc/v3/compressor"
	"github.com/ndsky1003/crpc/v3/protocol/header/headerstatus"
	"github.com/ndsky1003/crpc/v3/protocol/header/headertype"
)

// | Version |Type | Status |  CoderT |  CoderT |  CoderT |CompressType|    FromService    | ToService       |      Module       |   Method	       |  Seq     | MataLen  | RequestLen | Checksum |
// +---------+----------+---------+---------+---------+------------+-------------------+-----------------+-------------------+-----------------+----------+----------+------------+----------+
// |  uint32 |uint8|  uint8 | uint8  | uint8  | uint8  |   uint8   |  uvarint (1byte)+ string   | uvarint+ string | uvarint + string  | uvarint +string |  uvarint |  uvarint  |   uvarint  |  uint32  |
type Header struct {
	Version         uint32         //4
	Type            headertype.T   //1
	Status          headerstatus.T //1
	MetaCoderT      coder.T        //1
	ReqCoderT       coder.T        //1
	ResCoderT       coder.T        //1
	CompressT       compressor.T   //1
	FromServiceUUID string         //1 长度超过127就报错 ,实际上100就试上线
	ToService       string         //1 同上
	Module          string         //1 同上
	Method          string         //1 同上
	Seq             uint64         //10
	MetaLen         uint64         //10
	BodyLen         uint64         //10
	// Checksum    uint32       //4 //tcp 层面已经有checksum了，这里可以不需要
}

const (
	// MaxHeaderSize = 4 + 1 + 1 + 1 + 1 + 1 + 1 + 1 + 1 + 1 + 1 + 10 + 10 + 10  (10 refer to binary.MaxVarintLen64)
	MaxHeaderSize = 44
	//防止链接异常，传入的第一个数字过大，导致耗尽系统资源，已经遇到过该问题，所以修复
	FrozeMaxHeaderSize = MaxHeaderSize + 100 + 100 + 100 + 100 //固定最大header长度,超过这个长度就属于异常数据
)

func (this *Header) SetVersion(v uint32) *Header {
	this.Version = v
	return this
}

func (this *Header) SetType(t headertype.T) *Header {
	this.Type = t
	return this
}

func (this *Header) SetStatus(t headerstatus.T) *Header {
	this.Status = t
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
	this.FromServiceUUID = s
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

func (this *Header) SetMataLen(s uint64) *Header {
	this.MetaLen = s
	return this
}

func (this *Header) SetBodyLen(s uint64) *Header {
	this.BodyLen = s
	return this
}

// func (this *Header) SetChecksum(s uint32) *Header {
// 	this.Checksum = s
// 	return this
// }

const (
	uint_32_size = 4
	// uint_16_size = 2
	uint_8_size = 2
)

// Marshal will encode request header into a byte slice
func (r *Header) Marshal() ([]byte, error) {
	length := MaxHeaderSize + len(r.FromServiceUUID) + len(r.ToService) + len(r.Module) + len(r.Method)
	if length > FrozeMaxHeaderSize {
		return nil, fmt.Errorf("heaer size:%v, > FrozeMaxHeaderSize:%v", length, FrozeMaxHeaderSize)
	}
	idx := 0
	header := make([]byte, length)

	binary.LittleEndian.PutUint32(header[idx:], r.Version)
	idx += uint_32_size

	header[idx+1] = uint8(r.Type)
	idx += uint_8_size

	header[idx+1] = uint8(r.Status)
	idx += uint_8_size

	header[idx+1] = uint8(r.MetaCoderT)
	idx += uint_8_size

	header[idx+1] = uint8(r.ReqCoderT)
	idx += uint_8_size

	header[idx+1] = uint8(r.ResCoderT)
	idx += uint_8_size

	header[idx+1] = uint8(r.CompressT)
	idx += uint_8_size

	idx += binary_write_string(header[idx:], r.FromServiceUUID)

	idx += binary_write_string(header[idx:], r.ToService)

	idx += binary_write_string(header[idx:], r.Module)

	idx += binary_write_string(header[idx:], r.Method)

	idx += binary.PutUvarint(header[idx:], r.Seq)

	idx += binary.PutUvarint(header[idx:], r.MetaLen)

	idx += binary.PutUvarint(header[idx:], r.BodyLen)

	// binary.LittleEndian.PutUint32(header[idx:], r.Checksum)
	// idx += uint_32_size

	return header[:idx], nil
}

// Unmarshal will decode request header into a byte slice
func (r *Header) Unmarshal(data []byte) (err error) {
	if len(data) == 0 {
		return errors.New("empty header data")
	}

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("unmarshal header panic: %v", r)
		}
	}()
	idx, size := 0, 0
	r.Version = binary.LittleEndian.Uint32(data[idx:])
	idx += uint_32_size

	r.Type = headertype.T(data[idx+1])
	idx += uint_8_size

	r.Status = headerstatus.T(data[idx+1])
	idx += uint_8_size

	r.MetaCoderT = coder.T(data[idx+1])
	idx += uint_8_size

	r.ReqCoderT = coder.T(data[idx+1])
	idx += uint_8_size

	r.ResCoderT = coder.T(data[idx+1])
	idx += uint_8_size

	r.CompressT = compressor.T(data[idx+1])
	idx += uint_8_size

	r.FromServiceUUID, size = binary_read_string(data[idx:])
	idx += size

	r.ToService, size = binary_read_string(data[idx:])
	idx += size

	r.Module, size = binary_read_string(data[idx:])
	idx += size

	r.Method, size = binary_read_string(data[idx:])
	idx += size

	r.Seq, size = binary.Uvarint(data[idx:])
	idx += size

	r.MetaLen, size = binary.Uvarint(data[idx:])
	idx += size

	r.BodyLen, size = binary.Uvarint(data[idx:])
	idx += size

	// r.Checksum = binary.LittleEndian.Uint32(data[idx:])
	return
}

func (h *Header) Release() {
	if h == nil {
		return
	}
	h.reset()
	release(h)
}

func (r *Header) reset() {
	r.Version = 0
	r.Type = 0
	r.Status = 0
	r.MetaCoderT = 0
	r.ReqCoderT = 0
	r.ResCoderT = 0
	r.CompressT = 0
	r.FromServiceUUID = ""
	r.ToService = ""
	r.Module = ""
	r.Method = ""
	r.Seq = 0
	r.MetaLen = 0
	r.BodyLen = 0
	// r.Checksum = 0
}

func (h *Header) GetMarshalType() coder.T {
	if h.Type.IsReq() {
		return h.ReqCoderT
	} else {
		return h.ResCoderT
	}
}

func binary_read_string(data []byte) (string, int) {
	idx := 0
	length, size := binary.Uvarint(data)
	idx += size
	str := string(data[idx : idx+int(length)])
	idx += len(str)
	return str, idx
}

func binary_write_string(data []byte, str string) int {
	idx := 0
	idx += binary.PutUvarint(data, uint64(len(str)))
	copy(data[idx:], str)
	idx += len(str)
	return idx
}
