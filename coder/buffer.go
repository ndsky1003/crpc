package coder

import (
	"bytes"
	"sync"
)

var pool = &sync.Pool{
	New: func() any {
		return &bytes.Buffer{}
	},
}

func get_buffer() *bytes.Buffer {
	return pool.Get().(*bytes.Buffer)
}
func release_buffer(buf *bytes.Buffer) {
	if buf == nil {
		return
	}
	buf.Reset()
	pool.Put(buf)
}
