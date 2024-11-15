package coder

import (
	"reflect"
	"testing"

	"github.com/ndsky1003/crpc/v2/dto"
)

var test_coders = map[T]Coder{
	JSON:           new_json_coder(),
	MsgPack:        new_msgpack(),
	MsgPackJSONTag: new_msgpack_with_tag("json"),
	// FilePack:       new_file_pack(),
	// Protobuf:       new_protobuf_pack(),
	Msgp: new_msgp_coder(),
	// Sonic: new_sonic_coder(),
}

func Test_int(t *testing.T) {
	var a int = 12
	var r int
	var rr int
	type args struct {
		name string
		a    *int
		r    *int
	}
	tests := []args{
		{
			name: "int 2 int",
			a:    &a,
			r:    &r,
		},
		{
			name: "int 2 nil",
			a:    &a,
			r:    nil,
		},
		{
			name: "nil 2 int",
			a:    nil,
			r:    &rr,
		},
		{
			name: "nil 2 nil",
			a:    nil,
			r:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key := range test_coders {
				// t.Logf("code:%+v", tt)
				data, err := Marshal(key, tt.a)
				if err != nil {
					t.Error(key, err)
					return
				}
				var receive any
				if tt.r != nil {
					receive = tt.r
				}
				if err := Unmarshal(key, data, receive); err != nil {
					t.Error(key, err)
					return
				}

				t.Logf("%18s a:%+v r:%+v", key, getValue(tt.a), getValue(receive))

			}
		})
	}
}

func getValue(a any) any {
	rv := reflect.ValueOf(a)
	for {
		if !rv.IsValid() {
			return nil
		}
		if rv.Kind() == reflect.Ptr || rv.Kind() == reflect.Interface {
			rv = rv.Elem()
		} else {
			return rv.Interface()
		}
	}

}

func Test_string(t *testing.T) {
	var a string = "lll"
	var r string
	var rr string
	type args struct {
		name string
		a    *string
		r    *string
	}
	tests := []args{
		{
			name: "string 2 string",
			a:    &a,
			r:    &r,
		},
		{
			name: "string 2 nil",
			a:    &a,
			r:    nil,
		},
		{
			name: "nil 2 string",
			a:    nil,
			r:    &rr,
		},
		{
			name: "nil 2 nil",
			a:    nil,
			r:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key := range test_coders {
				// t.Logf("code:%+v", tt)
				data, err := Marshal(key, tt.a)
				if err != nil {
					t.Error(key, err)
					return
				}
				var receive any
				if tt.r != nil {
					receive = tt.r
				}
				if err := Unmarshal(key, data, receive); err != nil {
					t.Error(key, err)
					return
				}
				t.Logf("%18s a:%+v r:%+v", key, getValue(tt.a), getValue(receive))
			}
		})
	}
}

func Test_bool(t *testing.T) {
	var a bool = true
	var r bool
	var rr bool
	type args struct {
		name string
		a    *bool
		r    *bool
	}
	tests := []args{
		{
			name: "bool 2 bool",
			a:    &a,
			r:    &r,
		},
		{
			name: "bool 2 nil",
			a:    &a,
			r:    nil,
		},
		{
			name: "nil 2 bool",
			a:    nil,
			r:    &rr,
		},
		{
			name: "nil 2 nil",
			a:    nil,
			r:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key := range test_coders {
				// t.Logf("code:%+v", tt)
				data, err := Marshal(key, tt.a)
				if err != nil {
					t.Error(key, err)
					return
				}
				var receive any
				if tt.r != nil {
					receive = tt.r
				}
				if err := Unmarshal(key, data, receive); err != nil {
					t.Error(key, err)
					return
				}
				t.Logf("%18s a:%+v r:%+v", key, getValue(tt.a), getValue(receive))
			}
		})
	}
}

func Test_struct(t *testing.T) {
	var a = dto.PersonUseTest{
		Hobby: "度数",
	}
	var r dto.PersonUseTest
	var rr dto.PersonUseTest
	type args struct {
		name string
		a    *dto.PersonUseTest
		r    *dto.PersonUseTest
	}
	tests := []args{
		{
			name: "struct 2 struct",
			a:    &a,
			r:    &r,
		},
		{
			name: "struct 2 nil",
			a:    &a,
			r:    nil,
		},
		{
			name: "nil 2 struct",
			a:    nil,
			r:    &rr,
		},
		{
			name: "nil 2 nil",
			a:    nil,
			r:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key := range test_coders {
				// t.Logf("code:%+v", tt)
				data, err := Marshal(key, tt.a)
				if err != nil {
					t.Error(key, err)
					return
				}
				var receive any
				if tt.r != nil {
					receive = tt.r
				}
				if err := Unmarshal(key, data, receive); err != nil {
					t.Error(key, err)
					return
				}
				t.Logf("%18s a:%+v r:%+v", key, getValue(tt.a), getValue(receive))
			}
		})
	}
}
