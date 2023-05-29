package crpc

import (
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/ndsky1003/crpc/coder"
	"github.com/ndsky1003/crpc/compressor"
	"github.com/ndsky1003/crpc/options"
)

// number,string,bool,slice, map *point
type robot_service struct {
}

func (*robot_service) CallInt(a int, r *int) error {
	*r = a * 10
	return nil
}

func (*robot_service) CallString(a string, r *string) error {
	*r = fmt.Sprintf("hhhh:%s", a)
	return nil
}

func (*robot_service) CallBool(a bool, r *bool) error {
	*r = !a
	return nil
}
func (*robot_service) CallSlice(a []*test_obj_data, r *string) error {
	fmt.Printf("a:%+v\n", a)
	*r = a[0].Name
	return nil
}
func (*robot_service) CallMap(a map[int]string, r *string) error {
	for _, v := range a {
		*r = v
		return nil
	}
	return nil
}

func (*robot_service) CallPointer(a *string, r *string) error {
	*r = *a + " is a pointer"
	return nil
}

type test_obj_data struct {
	Name string
	Age  int
}

func (this *test_obj_data) String() string {
	return fmt.Sprintf("%+v", *this)
}

func TestMain(m *testing.M) {
	go NewServer().Listen(":8081")
	go func() {
		Dial("client", "127.0.0.1:8081", options.Client().SetIsStopHeart(true).SetCoderType(coder.JSON)).RegisterName("rpc", new(robot_service))
		select {}
	}()
	time.Sleep(2e9)
	code := m.Run()
	os.Exit(code)
}

func Test_Call(t *testing.T) {
	client1 := Dial("client1", "127.0.0.1:8081", options.Client().SetIsStopHeart(true).SetCoderType(coder.MsgPack).SetCompressorType(compressor.Raw))
	time.Sleep(1e9)
	type args struct {
		name   string
		server string
		method string
		a      any
		r      any
	}
	var a int8 = 120
	var _ float64 = 120 //手动修改，因为不同额coder处理数据的默认类型不一致，但是最终结果是一致的
	tests := []args{
		{
			name:   "1",
			server: "client",
			method: "rpc.CallInt",
			a:      12,
			r:      a, //json无整数
		},
		{
			name:   "2",
			server: "client",
			method: "rpc.CallString",
			a:      "runll",
			r:      "hhhh:runll",
		},
		{
			name:   "3",
			server: "client",
			method: "rpc.CallBool",
			a:      true,
			r:      false,
		},
		{
			name:   "4",
			server: "client",
			method: "rpc.CallSlice",
			a:      []*test_obj_data{{Name: "ppxia", Age: 28}},
			r:      "ppxia",
		},
		{
			name:   "5",
			server: "client",
			method: "rpc.CallMap",
			a:      map[int]string{1: "one"},
			r:      "one",
		},
		{
			name:   "6",
			server: "client",
			method: "rpc.CallPointer",
			a:      "dd",
			r:      "dd is a pointer",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ret any
			if err := client1.Call(tt.server, tt.method, tt.a, &ret); err != nil {
				t.Error(err)
			} else if tt.r != ret {
				t.Logf("v:%+v", reflect.TypeOf(ret))
				t.Errorf("return value:%v,expect value:%v", ret, tt.r)
			}
		})
	}
}

func Benchmark_Call(b *testing.B) {
	client2 := Dial("client2", "127.0.0.1:8081", options.Client().SetIsStopHeart(true).SetCoderType(coder.MsgPack).SetCompressorType(compressor.Raw))
	time.Sleep(1e9)
	type args struct {
		name   string
		server string
		method string
		a      any
		r      any
	}
	var a int8 = 120
	tests := []args{
		{
			name:   "1",
			server: "client",
			method: "rpc.CallInt",
			a:      12,
			r:      a,
		},
		{
			name:   "2",
			server: "client",
			method: "rpc.CallString",
			a:      "runll",
			r:      "hhhh:runll",
		},
		{
			name:   "3",
			server: "client",
			method: "rpc.CallBool",
			a:      true,
			r:      false,
		},
		{
			name:   "4",
			server: "client",
			method: "rpc.CallSlice",
			a:      []*test_obj_data{{Name: "ppxia", Age: 28}},
			r:      "ppxia",
		},
		{
			name:   "5",
			server: "client",
			method: "rpc.CallMap",
			a:      map[int]string{1: "one"},
			r:      "one",
		},
		{
			name:   "6",
			server: "client",
			method: "rpc.CallPointer",
			a:      "dd",
			r:      "dd is a pointer",
		},
	}
	b.ResetTimer()
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			var ret any
			if err := client2.Call(tt.server, tt.method, tt.a, &ret); err != nil {
				b.Error(err)
			} else if tt.r != ret {
				b.Errorf("return value:%v,expect value:%v", ret, tt.r)
			}
		})
	}
}
