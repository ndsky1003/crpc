// 目前这个性能最好,不在乎其内部结构是否缺少
package coder

import (
	"reflect"
	"sync"
	"testing"
)

func Test_msgp_coder_Marshal(t *testing.T) {
	type fields struct {
		pool *sync.Pool
	}
	type args struct {
		v any
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []byte
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			this := &msgp_coder{
				pool: tt.fields.pool,
			}
			got, err := this.Marshal(tt.args.v)
			if (err != nil) != tt.wantErr {
				t.Errorf("msgp_coder.Marshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("msgp_coder.Marshal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_msgp_coder_Unmarshal(t *testing.T) {
	type fields struct {
		pool *sync.Pool
	}
	type args struct {
		data []byte
		v    any
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			this := &msgp_coder{
				pool: tt.fields.pool,
			}
			if err := this.Unmarshal(tt.args.data, tt.args.v); (err != nil) != tt.wantErr {
				t.Errorf("msgp_coder.Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Benchmark_msgp_coder_Marshal(b *testing.B) {
	type args struct {
		v any
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "person",
			args: args{
				v: "cc",
			},
		},
	}
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				// _, err := new_msgp_coder().Marshal(tt.args.v)
				// b.Error(err)
			}
		})
	}
}
