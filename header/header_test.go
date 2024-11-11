package header

import (
	"testing"
	"time"
)

func TestReqHeader_Marshal(t *testing.T) {
	h := &Header{
		Version:     uint32(time.Now().Unix()),
		Type:        10,
		MetaCoderT:  1,
		ReqCoderT:   2,
		ResCoderT:   3,
		CompressT:   4,
		FromService: "gateway",
		ToService:   "db",
		Module:      "rpc",
		Method:      "ChangePwd",
		Seq:         22,
		MetaLen:     102,
		BodyLen:     100,
		Checksum:    12834,
	}

	t.Logf("header:%+v", h)
	data := h.Marshal()
	t.Log(len(data), "data:", data)
	h1 := Get()
	h1.Unmarshal(data)
	t.Logf("%+v", h1)
}
