package client

import (
	"sync"
	"sync/atomic"
)

type call_ struct {
	seq   uint64
	Reply any
	Error error

	BroadcaseResNewFunc  func() any            // 用于广播调用时创建返回值对象
	BroadcaseResCallBack func(any, error) bool // 返回true表示继续广播,返回false表示停止广播

	finished atomic.Bool
	Done     chan *call_
}

func GetCall() *call_ {
	c := pool_Call.Get().(*call_)
	// 只有第一次创建时才 make，之后复用！
	if c.Done == nil {
		c.Done = make(chan *call_, 1)
	}
	return c
}

func (this *call_) done() {
	if !this.finished.CompareAndSwap(false, true) {
		return
	}

	// 非阻塞发送：防止 double done 导致阻塞，或者没人读导致的泄露
	select {
	case this.Done <- this:
		// 发送成功，不要 close！保留 Channel 给下次复用
	default:
		// 正常情况不应该走到 default，除非 buffer 满了（说明没人读）
	}
}

func (c *call_) Release() {
	// 1. 清理业务字段
	c.seq = 0
	c.Reply = nil
	c.Error = nil
	c.BroadcaseResNewFunc = nil // 必须清空，防止内存泄漏 (闭包可能引用大对象)
	c.BroadcaseResCallBack = nil
	c.finished.Store(false)

	// 2. 核心：排空 Channel，确保归还给 Pool 时它是空的
	select {
	case <-c.Done:
		// 里的旧数据扔掉
	default:
		// 空的，没事
	}

	// 3. 归还
	pool_Call.Put(c)
}

var pool_Call = sync.Pool{
	New: func() any {
		return &call_{} // 这里不 make chan，推迟到 GetCall 的 if check
	},
}
