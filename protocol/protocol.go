package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/ndsky1003/crpc/v3/protocol/header"
)

// MagicNumber 用于协议识别 (ASCII 'C' 'R')
// 防止非法连接导致内存分配异常
const MagicNumber uint16 = 0x4352
const MaxPacketSize = 100 * 1024 * 1024

var (
	ErrPacketTooShort = errors.New("packet too short")
	ErrMagicMismatch  = errors.New("magic number mismatch")
	ErrPacketTooLarge = errors.New("header too large")
	ErrIncomplete     = errors.New("packet incomplete")
)

// Pack 打包消息: Magic(2) + HeaderLen(2) + Header + Meta + Body
func Pack(h *header.Header, meta []byte, body []byte) ([][]byte, error) {
	h.MetaLen = uint64(len(meta))
	h.BodyLen = uint64(len(body))

	headBytes, err := h.Marshal()
	if err != nil {
		return nil, err
	}

	if len(headBytes) > 65535 {
		return nil, ErrPacketTooLarge
	}

	// 申请 4 个字节：2字节魔数 + 2字节头长度
	first_bytes := make([]byte, 4)

	// 1. 写入魔数
	binary.BigEndian.PutUint16(first_bytes[0:2], MagicNumber)
	// 2. 写入 Header 长度
	binary.BigEndian.PutUint16(first_bytes[2:4], uint16(len(headBytes)))

	res := make([][]byte, 4)
	res[0] = first_bytes
	res[1] = headBytes
	res[2] = meta
	res[3] = body
	return res, nil
}

// Unpack 解包
func Unpack(data []byte) (*header.Header, []byte, []byte, error) {
	lengh := len(data)
	if lengh == 0 {
		return nil, nil, nil, errors.New("data length is zero")
	}
	// 最小长度变为 4 (Magic + Len)
	if lengh < 4 {
		return nil, nil, nil, ErrPacketTooShort
	}

	// 1. 校验魔数
	magic := binary.BigEndian.Uint16(data[0:2])
	if magic != MagicNumber {
		return nil, nil, nil, fmt.Errorf("%w: read %x, expect %x", ErrMagicMismatch, magic, MagicNumber)
	}

	// 2. 读取 Header 长度
	headLen := uint64(binary.BigEndian.Uint16(data[2:4]))

	// 校验数据总长度是否足够容纳 Header
	if lengh < int(4+headLen) {
		return nil, nil, nil, ErrIncomplete
	}

	h := header.Get()
	// Header 数据从第 4 个字节开始
	header_bytes := data[4 : 4+headLen]
	if err := h.Unmarshal(header_bytes); err != nil {
		h.Release()
		return nil, nil, nil, err
	}

	if h.MetaLen > MaxPacketSize || h.BodyLen > MaxPacketSize {
		h.Release()
		return nil, nil, nil, ErrPacketTooLarge
	}

	needed := uint64(4+headLen) + h.MetaLen + h.BodyLen

	if needed < h.MetaLen || needed < h.BodyLen {
		h.Release()
		return nil, nil, nil, ErrPacketTooShort // 或 ErrOverflow
	}

	if needed > MaxPacketSize {
		h.Release()
		return nil, nil, nil, ErrPacketTooLarge
	}

	if uint64(len(data)) < needed {
		h.Release()
		return nil, nil, nil, ErrIncomplete
	}

	metaStart := 4 + headLen
	metaEnd := metaStart + h.MetaLen
	meta := data[metaStart:metaEnd]
	body := data[metaEnd:needed]

	return h, meta, body, nil
}

// PeekHeader 仅查看头部信息（用于路由等），不完整解包
func PeekHeader(data []byte) (*header.Header, error) {
	if len(data) < 4 {
		return nil, ErrPacketTooShort
	}

	// 1. 校验魔数
	magic := binary.BigEndian.Uint16(data[0:2])
	if magic != MagicNumber {
		return nil, fmt.Errorf("%w: read %x", ErrMagicMismatch, magic)
	}

	headLen := binary.BigEndian.Uint16(data[2:4])
	if len(data) < int(4+headLen) {
		return nil, ErrIncomplete
	}

	h := header.Get()
	header_bytes := data[4 : 4+headLen]
	if err := h.Unmarshal(header_bytes); err != nil {
		h.Release()
		return nil, err
	}
	return h, nil
}
