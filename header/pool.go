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
	//pool_res = sync.Pool{New: func() interface{} {
	//return &ResponseHeader{}
	//}}
}
func Get() *Header {
	h := pool.Get().(*Header)
	return h
}

func Release(h *Header) {
	h.Reset()
	pool.Put(h)
}

//func GetResponse() *ResponseHeader {
//h := pool_res.Get().(*ResponseHeader)
//return h
//}

//func ReleaseResponse(h *ResponseHeader) {
//h.Reset()
//pool_res.Put(h)
//}
