package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"

	"github.com/ndsky1003/crpc/v3/protocol/header"
)

// Pack 打包消息: HeaderLen(2) + Header + Meta + Body
func Pack(h *header.Header, meta []byte, body []byte) ([]byte, error) {
	h.MetaLen = uint32(len(meta))
	h.BodyLen = uint32(len(body))

	headBuf := new(bytes.Buffer)
	binary.Write(headBuf, binary.BigEndian, h.Seq)
	binary.Write(headBuf, binary.BigEndian, h.Type)
	writeString(headBuf, h.ServiceName)
	writeString(headBuf, h.Method)
	writeString(headBuf, h.TargetSid)
	writeString(headBuf, h.Error)
	binary.Write(headBuf, binary.BigEndian, h.MetaLen)
	binary.Write(headBuf, binary.BigEndian, h.BodyLen)

	headBytes := headBuf.Bytes()
	if len(headBytes) > 65535 {
		return nil, errors.New("header too large")
	}

	totalLen := 2 + len(headBytes) + len(meta) + len(body)
	buf := make([]byte, totalLen)

	binary.BigEndian.PutUint16(buf[0:2], uint16(len(headBytes)))
	copy(buf[2:], headBytes)

	offset := 2 + len(headBytes)
	copy(buf[offset:], meta)

	offset += len(meta)
	copy(buf[offset:], body)

	return buf, nil
}

// Unpack 解包
func Unpack(data []byte) (*header.Header, []byte, []byte, error) {
	if len(data) < 2 {
		return nil, nil, nil, errors.New("packet too short")
	}

	headLen := binary.BigEndian.Uint16(data[0:2])
	if len(data) < int(2+headLen) {
		return nil, nil, nil, errors.New("header incomplete")
	}

	h := &header.Header{}
	r := bytes.NewReader(data[2 : 2+headLen])
	binary.Read(r, binary.BigEndian, &h.Seq)
	binary.Read(r, binary.BigEndian, &h.Type)
	h.ServiceName = readString(r)
	h.Method = readString(r)
	h.TargetSid = readString(r)
	h.Error = readString(r)
	binary.Read(r, binary.BigEndian, &h.MetaLen)
	binary.Read(r, binary.BigEndian, &h.BodyLen)

	offset := int(2 + headLen)
	if len(data) < offset+int(h.MetaLen)+int(h.BodyLen) {
		return nil, nil, nil, errors.New("body incomplete")
	}

	meta := data[offset : offset+int(h.MetaLen)]
	offset += int(h.MetaLen)
	body := data[offset : offset+int(h.BodyLen)]

	return h, meta, body, nil
}

func writeString(w io.Writer, s string) {
	l := uint16(len(s))
	binary.Write(w, binary.BigEndian, l)
	io.WriteString(w, s)
}

func readString(r io.Reader) string {
	var l uint16
	binary.Read(r, binary.BigEndian, &l)
	buf := make([]byte, l)
	io.ReadFull(r, buf)
	return string(buf)
}
