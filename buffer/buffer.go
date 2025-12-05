package buffer

import (
	"bytes"
	"sync"
)

type Buffer struct {
	bytes.Buffer
}

var pool = &sync.Pool{
	New: func() any {
		return &Buffer{}
	},
}

func Get() *Buffer {
	return pool.Get().(*Buffer)
}

func (this *Buffer) Release() {
	if this == nil {
		return
	}
	this.Reset()
	pool.Put(this)
}
