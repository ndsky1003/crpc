package broadcastresult

import (
	"sync"

	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/crpc/v3/protocol/header/headercode"
)

type Result struct {
	RawBody         []byte       // 只包含body
	Res             any          // 已经解码的对象（来自本地优化）,可以是返回值，也可以是错误，通过code判断
	ResCoderT       coder.T      // 编码类型
	Code            headercode.T // 是否成功
	IsEOS           bool         // 是否是结束标志
	FromLocal       bool         // 标记是否来自本地调用 ,支持空返回值，所以必须用一个独立的字段来判断
	ReleaseCallback func()       //池化,释放的时候回调
}

func (r *Result) clear() {
	if r == nil {
		return
	}
	if r.ReleaseCallback != nil {
		r.ReleaseCallback()
	}
	r.RawBody = nil
	r.Res = nil
	r.ResCoderT = coder.Raw
	r.Code = headercode.OK
	r.IsEOS = false
	r.FromLocal = false
	r.ReleaseCallback = nil
}

var pool = sync.Pool{
	New: func() any {
		return &Result{}
	},
}

func Get() *Result {
	return pool.Get().(*Result)
}

func Put(r *Result) {
	if r == nil {
		return
	}
	r.clear()
	pool.Put(r)
}
