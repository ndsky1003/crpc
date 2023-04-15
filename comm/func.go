package comm

import (
	"encoding/binary"
	"os"
	"path/filepath"
)

func BinaryReadString(data []byte) (string, int) {
	idx := 0
	length, size := binary.Uvarint(data)
	idx += size
	str := string(data[idx : idx+int(length)])
	idx += len(str)
	return str, idx
}

func BinaryWriteString(data []byte, str string) int {
	idx := 0
	idx += binary.PutUvarint(data, uint64(len(str)))
	copy(data[idx:], str)
	idx += len(str)
	return idx
}

func GetWriteFile(chunkIndex uint16, filename string) (f *os.File, err error) {
	dir, _ := filepath.Split(filename)
	if dir != "" {
		if err = os.MkdirAll(dir, 0700); err != nil {
			return
		}
	}
	var flag = os.O_CREATE | os.O_APPEND | os.O_WRONLY
	if chunkIndex == 0 {
		flag |= os.O_TRUNC
	}
	f, err = os.OpenFile(filename, flag, 0600)
	return
}
