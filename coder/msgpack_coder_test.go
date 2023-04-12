package coder

import (
	"testing"
)

func Test_msg_pack_Marshal(t *testing.T) {
	pack := new_msg_pack()
	data, err := pack.Marshal(nil)
	t.Error(data, err)
}
