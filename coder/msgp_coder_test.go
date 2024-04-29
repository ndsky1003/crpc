// 目前这个性能最好,不在乎其内部结构是否缺少
package coder

import (
	"reflect"
	"testing"

	"github.com/ndsky1003/crpc/dto"
)

func Test_new_msgp_coder(t *testing.T) {
	tests := []struct {
		name string
		want *msgp_coder
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := new_msgp_coder(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("new_msgp_coder() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_msgp_coder_Unmarshal(t *testing.T) {
	data := []byte{
		133, 162, 73, 100, 19, 163, 83, 101, 120, 195, 165, 72,
		111, 98, 98, 121, 145, 166, 229, 186, 166, 230, 149, 176,
		161, 77, 129, 164, 110, 97, 109, 101, 18, 163, 83, 117,
		98, 132, 162, 73, 100, 204, 191, 163, 83, 117, 98, 192,
		165, 72, 111, 98, 98, 121, 146, 164, 110, 105, 104, 97,
		169, 231, 160, 129, 228, 187, 163, 231, 160, 129, 164, 78,
		97, 109, 101, 164, 84, 111, 109, 50,
	}
	p := new_msgp_coder()
	var obj dto.Item
	if err := p.Unmarshal(data, &obj); err != nil {
		t.Error(err)
	}

	t.Logf("obj:%+v", obj)

}

func Test_msgp_coder_Marshal(t *testing.T) {
	type args struct {
		v any
	}
	var a int
	var s string
	var b bool
	tests := []struct {
		name    string
		this    *msgp_coder
		args    args
		want    any
		wantErr bool
	}{
		{
			name: "*point",
			args: args{
				v: &dto.Item{
					Id:    1,
					Name:  "one",
					Sex:   true,
					Hobby: nil,
					Sub: &dto.Item{
						Id:   1,
						Name: "one",
					},
				},
			},
			want: &dto.Item{},
		},
		{
			name: "struct",
			args: args{
				v: &dto.Item{
					Id:    11,
					Name:  "two",
					Sex:   false,
					Hobby: []string{"kk"},
					Sub: &dto.Item{
						Id:    12,
						Name:  "two",
						Hobby: []string{"d"},
					},
				},
			},
			want: &dto.Item{},
		},
		{
			name: "slice",
			args: args{
				v: []*dto.Item{
					{
						Id:    11,
						Name:  "one",
						Sex:   false,
						Hobby: []string{"a", "b", "c"},
						Sub: &dto.Item{
							Id:    12,
							Name:  "slice one2",
							Hobby: []string{"a1", "b1", "c1"},
						},
					},
					{
						Id:   12,
						Name: "slice two",
					},
				},
			},
			want: &[]*dto.Item{},
		},
		{
			name: "map",
			args: args{
				v: map[int64]*dto.Item{
					11: {
						Id:    11,
						Name:  "one",
						Sex:   false,
						Hobby: []string{"a", "b", "c"},
						Sub: &dto.Item{
							Id:    12,
							Name:  "map one2",
							Hobby: []string{"a1", "b1", "c1"},
						},
					},
					12: {
						Id:   12,
						Name: "map two",
					},
				},
			},
			want: &map[int64]*dto.Item{},
		},
		{
			name: "number",
			args: args{
				v: 1,
			},
			want: &a,
		},
		{
			name: "string",
			args: args{
				v: "ab",
			},
			want: &s,
		},
		{
			name: "bool",
			args: args{
				v: true,
			},
			want: &b,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			this := &msgp_coder{}
			got, err := this.Marshal(tt.args.v)
			if (err != nil) != tt.wantErr {
				t.Errorf("msgp_coder.Marshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			// var obj dto.Item
			if err := this.Unmarshal(got, tt.want); err != nil {
				t.Error(tt.name, err)
			}
			switch v := tt.want.(type) {
			case *int:
				if !reflect.DeepEqual(tt.args.v, *v) {
					t.Errorf("msgp_coder.Marshal() = %v, want %v", got, tt.want)
				}
			case *string:
				if !reflect.DeepEqual(tt.args.v, *v) {
					t.Errorf("msgp_coder.Marshal() = %v, want %v", got, tt.want)
				}
			case *bool:
				if !reflect.DeepEqual(tt.args.v, *v) {
					t.Errorf("msgp_coder.Marshal() = %v, want %v", got, tt.want)
				}
			case *map[int64]*dto.Item:
				if !reflect.DeepEqual(tt.args.v, *v) {
					t.Errorf("msgp_coder.Marshal() = %v, want %v", got, tt.want)
				}
			case *[]*dto.Item:
				if !reflect.DeepEqual(tt.args.v, *v) {
					t.Errorf("msgp_coder.Marshal() = %v, want %v", got, tt.want)
				}
			case *dto.Item:
				if tt.name == "struct" {
					if !reflect.DeepEqual(tt.args.v, tt.want) {
						t.Errorf("msgp_coder.Marshal(1) = %v, want %v", got, tt.want)
					}
				} else {
					if !reflect.DeepEqual(tt.args.v, tt.want) {
						t.Errorf("msgp_coder.Marshal() = %v, want %v", got, tt.want)
					}
				}
			}
		})
	}
}
