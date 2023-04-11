package header

import "sync"

var (
	pool sync.Pool
)

func init() {
	pool = sync.Pool{New: func() interface{} {
		return &Header{}
	}}
}
func GetRequestHeader() *Header {
	h := pool.Get().(*Header)
	h.Type = header_type_req
	return h
}
func GetResponseHeader() *Header {
	h := pool.Get().(*Header)
	h.Type = header_type_res
	return h
}

func ReleaseHeader(h *Header) {
	h.ResetHeader()
	pool.Put(h)
}
