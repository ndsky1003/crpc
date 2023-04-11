package header

import (
	"testing"

	"github.com/ndsky1003/crpc/coder"
	"github.com/ndsky1003/crpc/compressor"
)

func TestHeader_Marshal(t *testing.T) {
	h := &Header{
		Type:         header_type_res,
		CoderType:    coder.JSON,
		CompressType: compressor.Raw,
		Service:      "db",
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

func TestHeader_Unmarshal(t *testing.T) {
	type args struct {
		data []byte
	}
	tests := []struct {
		name    string
		init    func(t *testing.T) *Header
		inspect func(r *Header, t *testing.T) //inspects receiver after test run

		args func(t *testing.T) args

		wantErr    bool
		inspectErr func(err error, t *testing.T) //use for more precise error evaluation after test
	}{
		//TODO: Add test cases
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tArgs := tt.args(t)

			receiver := tt.init(t)
			err := receiver.Unmarshal(tArgs.data)

			if tt.inspect != nil {
				tt.inspect(receiver, t)
			}

			if (err != nil) != tt.wantErr {
				t.Fatalf("Header.Unmarshal error = %v, wantErr: %t", err, tt.wantErr)
			}

			if tt.inspectErr != nil {
				tt.inspectErr(err, t)
			}
		})
	}
}
