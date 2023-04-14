package header

import "sync"

var (
	pool sync.Pool
	//pool_res sync.Pool
)

func init() {
	pool = sync.Pool{New: func() interface{} {
		return &Header{}
	}}
}
func Get() *Header {
	h := pool.Get().(*Header)
	return h
}

func Release(h *Header) {
	if h == nil {
		return
	}
	h.Reset()
	pool.Put(h)
}
