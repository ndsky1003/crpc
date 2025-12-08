package client

import (
	"sync/atomic"
)

type Call struct {
	seq   uint64
	Reply any
	Error error

	BroadcaseResNewFunc  func() any            // 用于广播调用时创建返回值对象
	BroadcaseResCallBack func(any, error) bool // 返回true表示继续广播,返回false表示停止广播

	finished atomic.Bool
	Done     chan *Call
}

func GetCall() *Call {
	return &Call{
		Done: make(chan *Call, 1),
	}
}

func (this *Call) done() {
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
