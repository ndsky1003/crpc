package header

import (
	"encoding/binary"
	"errors"

	"github.com/google/uuid"
	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/crpc/v3/compressor"
	"github.com/ndsky1003/crpc/v3/protocol/header/headercode"
	"github.com/ndsky1003/crpc/v3/protocol/header/headerflags"
	"github.com/ndsky1003/crpc/v3/protocol/header/headertype"
)

// +------+-------+------+------+------+------+------+------------------+-----------------------+...
// | Type | Flags | Code | Meta | Req  | Res  | Comp | UUID (Optional)  | Strings & Varints ... |
// |  1B  |   1B  |  1B  |  1B  |  1B  |  1B  |  1B  | 16B (if flag set)| (Length + Bytes) ...  |
// +------+-------+------+------+------+------+------+------------------+-----------------------+
// ^ idx=0                                           ^ idx=7            ^ idx=23 (if has UUID)
type Header struct {
	Type       headertype.T  //1 决定去向
	Flags      headerflags.T //1 决定行为
	Code       headercode.T  //1 决定状态,怎么处理这个结果
	MetaCoderT coder.T       //1
	ReqCoderT  coder.T       //1
	ResCoderT  coder.T       //1
	CompressT  compressor.T  //1
	UUID       uuid.UUID     //16 可能不能存在 可选字段，用于跟踪请求，可以不使用 ,这个直接放在最后,才有可能可选,否则固定消费16个字节
	ToService  string        //10 同上
	Module     string        //10 同上
	Method     string        //10 同上
	TraceID    string        //10 同上
	// [新增] 截止时间 (Unix Micro)
	// 0 表示无限制
	// 	v := now.UnixMicro()
	// t := time.UnixMicro(v)
	Deadline uint64
	Seq      uint64 //10
	MetaLen  uint64 //10
	BodyLen  uint64 //10
}

const (
	// 固定部分的长度: 7个uint8
	BaseFixedSize = 7
	MaxStringLen  = 4 * 1024 // 限制单个字符串最大长度，防止恶意攻击
)

func (this *Header) SetType(t headertype.T) *Header {
	this.Type = t
	return this
}

func (this *Header) SetFlags(t headerflags.T) *Header {
	this.Flags = t
	return this
}

func (this *Header) SetCode(t headercode.T) *Header {
	this.Code = t
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

func (this *Header) SetTraceID(s string) *Header {
	this.TraceID = s
	return this
}

func (this *Header) SetSeq(s uint64) *Header {
	this.Seq = s
	return this
}

func (this *Header) SetDeadline(s uint64) *Header {
	this.Deadline = s
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

func (this *Header) SetUUID(u uuid.UUID) *Header {
	this.UUID = u
	return this
}

const (
	uint_32_size = 4
	uint_8_size  = 1
)

// Marshal will encode request header into a byte slice
func (r *Header) Marshal() ([]byte, error) {
	size := BaseFixedSize

	hasUUID := r.UUID != uuid.Nil
	tmpFlags := r.Flags
	if hasUUID {
		size += 16
		tmpFlags.Add(headerflags.UUID)
	} else {
		tmpFlags.Remove(headerflags.UUID)
	}
	r.Flags = tmpFlags

	size += varintStrSize(r.ToService)
	size += varintStrSize(r.Module)
	size += varintStrSize(r.Method)
	size += varintStrSize(r.TraceID)

	size += uvarintSize(r.Deadline)
	size += uvarintSize(r.Seq)
	size += uvarintSize(r.MetaLen)
	size += uvarintSize(r.BodyLen)

	header := make([]byte, size)

	idx := 0
	header[idx] = uint8(r.Type)
	idx += uint_8_size

	header[idx] = uint8(r.Flags)
	idx += uint_8_size

	header[idx] = uint8(r.Code)
	idx += uint_8_size

	header[idx] = uint8(r.MetaCoderT)
	idx += uint_8_size

	header[idx] = uint8(r.ReqCoderT)
	idx += uint_8_size

	header[idx] = uint8(r.ResCoderT)
	idx += uint_8_size

	header[idx] = uint8(r.CompressT)
	idx += uint_8_size

	if hasUUID {
		copy(header[idx:], r.UUID[:])
		idx += 16
	}

	idx += writeString(header[idx:], r.ToService)

	idx += writeString(header[idx:], r.Module)

	idx += writeString(header[idx:], r.Method)

	idx += writeString(header[idx:], r.TraceID)

	idx += binary.PutUvarint(header[idx:], r.Deadline)

	idx += binary.PutUvarint(header[idx:], r.Seq)

	idx += binary.PutUvarint(header[idx:], r.MetaLen)

	idx += binary.PutUvarint(header[idx:], r.BodyLen)

	return header[:idx], nil
}

// Unmarshal will decode request header into a byte slice
func (r *Header) Unmarshal(data []byte) (err error) {
	if len(data) < BaseFixedSize {
		return errors.New("header too short")
	}

	idx, size := 0, 0

	r.Type = headertype.T(data[idx])
	idx += uint_8_size

	r.Flags = headerflags.T(data[idx])
	idx += uint_8_size

	r.Code = headercode.T(data[idx])
	idx += uint_8_size

	r.MetaCoderT = coder.T(data[idx])
	idx += uint_8_size

	r.ReqCoderT = coder.T(data[idx])
	idx += uint_8_size

	r.ResCoderT = coder.T(data[idx])
	idx += uint_8_size

	r.CompressT = compressor.T(data[idx])
	idx += uint_8_size

	if r.Flags.HasUUID() {
		if len(data[idx:]) < 16 {
			return errors.New("uuid flag set but data insufficient")
		}
		copy(r.UUID[:], data[idx:idx+16])
		idx += 16
	} else {
		r.UUID = uuid.Nil
	}

	r.ToService, size, err = readString(data[idx:])
	idx += size
	if err != nil {
		return err
	}

	r.Module, size, err = readString(data[idx:])
	idx += size
	if err != nil {
		return err
	}

	r.Method, size, err = readString(data[idx:])
	idx += size
	if err != nil {
		return err
	}

	r.TraceID, size, err = readString(data[idx:])
	idx += size
	if err != nil {
		return err
	}

	r.Deadline, size = binary.Uvarint(data[idx:])
	idx += size

	r.Seq, size = binary.Uvarint(data[idx:])
	idx += size

	r.MetaLen, size = binary.Uvarint(data[idx:])
	idx += size

	r.BodyLen, size = binary.Uvarint(data[idx:])
	idx += size

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
	r.Type = 0
	r.Flags = 0
	r.Code = 0
	r.MetaCoderT = 0
	r.ReqCoderT = 0
	r.ResCoderT = 0
	r.CompressT = 0
	r.UUID = uuid.Nil
	r.ToService = ""
	r.Module = ""
	r.Method = ""
	r.TraceID = ""
	r.Deadline = 0
	r.Seq = 0
	r.MetaLen = 0
	r.BodyLen = 0
}

func (h *Header) GetMarshalType() coder.T {
	if h.Type.IsReq() {
		return h.ReqCoderT
	} else {
		return h.ResCoderT
	}
}

func uvarintSize(x uint64) int {
	// binary.PutUvarint 的逻辑
	i := 0
	for x >= 0x80 {
		x >>= 7
		i++
	}
	return i + 1
}

func varintStrSize(s string) int {
	return uvarintSize(uint64(len(s))) + len(s)
}

func readString(b []byte) (string, int, error) {
	l, n := binary.Uvarint(b)
	if n <= 0 {
		err := errors.New("invalid varint len") // defer 会捕获
		return "", 0, err
	}
	if uint64(len(b[n:])) < l {
		err := errors.New("buffer too short for string") // defer 会捕获
		return "", 0, err
	}
	if l > MaxStringLen {
		err := errors.New("string too long") // defer 会捕获
		return "", 0, err
	}
	return string(b[n : n+int(l)]), n + int(l), nil
}

func writeString(b []byte, s string) int {
	n := binary.PutUvarint(b, uint64(len(s)))
	copy(b[n:], s)
	return n + len(s)
}
