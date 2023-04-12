package header

import (
	"testing"

	"github.com/ndsky1003/crpc/coder"
	"github.com/ndsky1003/crpc/compressor"
	"github.com/ndsky1003/crpc/header/headertype"
)

func TestReqHeader_Marshal(t *testing.T) {
	h := &Header{
		Type:         headertype.Ping,
		CoderType:    coder.JSON,
		CompressType: compressor.Raw,
		FromService:  "gateway",
		ToService:    "db",
		Module:       "rpc",
		Method:       "ChangePwd",
		Seq:          1,
		BodyLen:      100,
		Checksum:     12834,
	}
	data := h.Marshal()
	t.Log(data)
	t.Error(1)
	h1 := Get()
	h1.Unmarshal(data)
	t.Logf("%+v", h1)
}

//func TestResHeader_Marshal(t *testing.T) {
//h := &ResponseHeader{
//Type:         headertype.Res,
//CoderType:    coder.MsgPack,
//CompressType: compressor.Snappy,
//ToService:    "gateway",
//Error:        "error",
//Seq:          1,
//BodyLen:      100,
//Checksum:     12834,
//}
//data := h.Marshal()
//t.Log(data)
//t.Error(1)
//h1 := GetResponse()
//h1.Unmarshal(data)
//t.Logf("%+v", h1)
//}
