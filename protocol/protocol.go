package protocol

import (
	"encoding/binary"
	"errors"

	"github.com/ndsky1003/crpc/v3/protocol/header"
)

// Pack 打包消息: HeaderLen(2) + Header + Meta + Body
func Pack(h *header.Header, meta []byte, body []byte) ([][]byte, error) {
	h.MetaLen = uint64(len(meta))
	h.BodyLen = uint64(len(body))

	headBytes, err := h.Marshal()
	if err != nil {
		return nil, err
	}

	if len(headBytes) > 65535 {
		return nil, errors.New("header too large")
	}

	first_bytes := make([]byte, 2)
	binary.BigEndian.PutUint16(first_bytes[0:2], uint16(len(headBytes)))
	res := make([][]byte, 4)
	res[0] = first_bytes
	res[1] = headBytes
	res[2] = meta
	res[3] = body
	return res, nil
}

// Unpack 解包
func Unpack(data []byte) (*header.Header, []byte, []byte, error) {
	if len(data) < 2 {
		return nil, nil, nil, errors.New("packet too short")
	}

	headLen := uint64(binary.BigEndian.Uint16(data[0:2]))
	if len(data) < int(2+headLen) {
		return nil, nil, nil, errors.New("header incomplete")
	}
	h := header.Get()
	header_bytes := data[2 : 2+headLen]
	if err := h.Unmarshal(header_bytes); err != nil {
		h.Release()
		return nil, nil, nil, err
	}

	totalLen := int(2 + headLen + h.MetaLen + h.BodyLen)
	if len(data) < totalLen {
		h.Release()
		return nil, nil, nil, errors.New("packet incomplete")
	}

	meta := data[2+headLen : 2+headLen+h.MetaLen]
	body := data[2+headLen+h.MetaLen : totalLen]
	return h, meta, body, nil

}

func PeekHeader(data []byte) (*header.Header, error) {
	if len(data) < 2 {
		return nil, errors.New("packet too short")
	}
	headLen := binary.BigEndian.Uint16(data[0:2])
	if len(data) < int(2+headLen) {
		return nil, errors.New("header incomplete")
	}

	h := header.Get()
	header_bytes := data[2 : 2+headLen]
	if err := h.Unmarshal(header_bytes); err != nil {
		h.Release()
		return nil, err
	}
	return h, nil
}
