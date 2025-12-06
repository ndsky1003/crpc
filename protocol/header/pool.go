package header

import "sync"

var pool = sync.Pool{New: func() interface{} {
	return &Header{}
}}

func Get() *Header {
	h := pool.Get().(*Header)
	return h
}

func release(h *Header) {
	pool.Put(h)
}
