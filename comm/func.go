package comm

import (
	"encoding/binary"
	"reflect"

	"github.com/ndsky1003/crpc/v2/constant"
	"github.com/samber/lo"
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

func IsStandardErr(err error) bool {
	et := reflect.TypeOf(err)
	return lo.Contains(constant.StandardErrors, et)
}
