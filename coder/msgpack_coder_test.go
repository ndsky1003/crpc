package coder

import (
	"encoding/json"
	"testing"

	"github.com/ndsky1003/crpc/dto"
)

func Test_msg_pack_Marshal(t *testing.T) {
	// pack := new_msg_pack()
	// m := map[int]string{
	// 	1: "one",
	// 	2: "two",
	// }
	// data, err := pack.Marshal(m)
	// t.Error(data, err)
	// data := `{
	// "1": "one"
	// }`
	// var a map[int]string
	// pack.Unmarshal([]byte(data), &a)
	// t.Log(a)

	// type Item struct {
	// 	Foo string `json:"foo"`
	// }
	//
	// var buf bytes.Buffer
	// enc := msgpack.NewEncoder(&buf)
	// enc.SetCustomStructTag("json")
	//
	// // Produces `{"foo": "bar"}`.
	// enc.Encode(&Item{Foo: "bar"})
	// t.Log(string(buf.Bytes()))

	// m := map[string]any{}
	var m = dto.Item{
		Id:    1,
		Name:  "one",
		Sex:   true,
		Hobby: nil,
		Sub: &dto.Item{
			Id:    11,
			Name:  "two",
			Hobby: []string{"a", "b"},
		},
	}
	// var m any
	// msgpack.GetDecoder().SetCustomStructTag("json")
	// data := []byte{
	// 	132, 163, 65, 103, 101, 34, 163, 83, 101, 120, 195,
	// 	165, 72, 111, 98, 98, 121, 145, 166, 229, 186, 166,
	// 	230, 149, 176, 163, 83, 117, 98, 132, 162, 73, 100,
	// 	204, 191, 163, 83, 117, 98, 192, 165, 72, 111, 98,
	// 	98, 121, 146, 164, 110, 105, 104, 97, 169, 231, 160,
	// 	129, 228, 187, 163, 231, 160, 129, 164, 78, 97, 109,
	// 	101, 164, 84, 111, 109, 50,
	// }

	d := new_msgp_coder()
	data, err := d.Marshal(&m)
	if err != nil {
		t.Error(err)
	}
	t.Log(data)
	// var obj dto.Item
	// if err := d.Unmarshal(data, &obj); err != nil {
	// 	t.Error(err)
	// }
	// t.Logf("obj:%+v", obj)
}

// func Benchmark_msg_pack_Marshal_map(b *testing.B) {
// 	pack := new_msgp_coder() // new_msgpack_with_tag("json")
// 	m := map[int]string{
// 		1: "one",
// 		2: "two",
// 	}
// 	for i := 0; i < b.N; i++ {
// 		if _, err := pack.Marshal(m); err != nil {
// 			b.Error(err)
// 		}
// 	}
// }
//
// func Benchmark_msg_pack_Unmarshal_map(b *testing.B) {
// 	pack := new_msgp_coder() //new_msgpack_with_tag("json")
// 	m := map[int]string{
// 		1: "one",
// 		2: "two",
// 	}
// 	data, _ := pack.Marshal(m)
// 	for i := 0; i < b.N; i++ {
// 		var a map[int]string
// 		if err := pack.Unmarshal(data, &a); err != nil {
// 			b.Error(err)
// 		}
// 	}
// }
//
// func Benchmark_json_Marshal_map(b *testing.B) {
// 	m := map[int]string{
// 		1: "one",
// 		2: "two",
// 	}
// 	for i := 0; i < b.N; i++ {
// 		_, err := json.Marshal(m)
// 		if err != nil {
// 			b.Error(err)
// 		}
// 	}
// }
//
// func Benchmark_json_Unmarshal_map(b *testing.B) {
// 	m := map[int]string{
// 		1: "one",
// 		2: "two",
// 	}
// 	data, _ := json.Marshal(m)
// 	for i := 0; i < b.N; i++ {
// 		var a map[int]string
// 		err := json.Unmarshal(data, &a)
// 		if err != nil {
// 			b.Error(err)
// 		}
// 	}
// }

func Benchmark_msg_pack_Marshal_struct(b *testing.B) {
	pack := new_msgp_coder() //new_msgpack_with_tag("json")
	m := &dto.Item{
		Id:    1,
		Name:  "one",
		Sex:   true,
		Hobby: nil,
		Sub: &dto.Item{
			Id:   1,
			Name: "one",
		},
	}
	for i := 0; i < b.N; i++ {
		if _, err := pack.Marshal(m); err != nil {
			b.Error(err)
		}
	}
}

func Benchmark_msg_pack_Unmarshal_struct(b *testing.B) {
	pack := new_msgp_coder() //new_msgpack_with_tag("json")
	m := &dto.Item{
		Id:    1,
		Name:  "one",
		Sex:   true,
		Hobby: nil,
		Sub: &dto.Item{
			Id:   1,
			Name: "one",
		},
	}
	data, _ := pack.Marshal(m)
	for i := 0; i < b.N; i++ {
		var a dto.Item
		if err := pack.Unmarshal(data, &a); err != nil {
			b.Error(err)
		}
	}
}

func Benchmark_json_Marshal_struct(b *testing.B) {
	m := dto.Item{
		Id:    1,
		Name:  "one",
		Sex:   true,
		Hobby: nil,
		Sub: &dto.Item{
			Id:   1,
			Name: "one",
		},
	}
	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(m)
		if err != nil {
			b.Error(err)
		}
	}
}

func Benchmark_json_Unmarshal_struct(b *testing.B) {
	m := dto.Item{
		Id:    1,
		Name:  "one",
		Sex:   true,
		Hobby: nil,
		Sub: &dto.Item{
			Id:   1,
			Name: "one",
		},
	}
	data, _ := json.Marshal(m)
	for i := 0; i < b.N; i++ {
		var a dto.Item
		err := json.Unmarshal(data, &a)
		if err != nil {
			b.Error(err)
		}
	}
}
