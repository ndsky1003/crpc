package dto

import (
	"encoding/binary"

	"github.com/ndsky1003/crpc/comm"
)

// Marshal will encode request header into a byte slice
type FileBody struct {
	ChunksIndex uint16 // 65525个
	Offset      uint64
	Filename    string //存储路径
	Data        []byte
}

// 10 + 10 + 2
const MaxFileBodySize = 22

func (r *FileBody) Marshal() []byte {
	idx := 0
	body := make([]byte, MaxFileBodySize+len(r.Filename)+len(r.Data))

	binary.LittleEndian.PutUint16(body[idx:], r.ChunksIndex)
	idx += comm.Uint16Size

	idx += binary.PutUvarint(body[idx:], r.Offset)

	idx += comm.BinaryWriteString(body[idx:], r.Filename)

	copy(body[idx:], r.Data)
	idx += len(r.Data)
	return body[:idx]
}

// Unmarshal will decode request header into a byte slice
func (r *FileBody) Unmarshal(data []byte) (err error) {
	if len(data) == 0 {
		return comm.UnmarshalError
	}

	defer func() {
		if r := recover(); r != nil {
			err = comm.UnmarshalError
		}
	}()
	idx, size := 0, 0
	r.ChunksIndex = binary.LittleEndian.Uint16(data[idx:])
	idx += comm.Uint16Size

	offset, size := binary.Uvarint(data[idx:])
	r.Offset = offset
	idx += size

	r.Filename, size = comm.BinaryReadString(data[idx:])
	idx += size

	r.Data = data[idx:]
	return
}
