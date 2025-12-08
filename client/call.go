package client

import (
	"context"
	"sync/atomic"
)

type broadcastResult struct {
	data any
	err  error
}

type Call struct {
	seq   uint64
	Reply any
	Error error

	BroadcaseResNewFunc  func() any            // 用于广播调用时创建返回值对象
	BroadcaseResCallBack func(any, error) bool // 返回true表示继续广播,返回false表示停止广播

	// [新增] 广播专用缓冲通道
	// 网络层 -> 写入 -> 业务协程读取 -> 执行回调
	broadcastCh chan broadcastResult

	// [新增] 用于控制广播消费协程退出的 Context
	ctx    context.Context
	cancel context.CancelFunc

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

	// [新增] 核心修改：触发 CancelFunc
	// 这会让 processBroadcastLoop 安全退出，而不需要关闭 channel
	if this.cancel != nil {
		this.cancel()
	}

	// 非阻塞发送：防止 double done 导致阻塞，或者没人读导致的泄露
	select {
	case this.Done <- this:
		// 发送成功，不要 close！保留 Channel 给下次复用
	default:
		// 正常情况不应该走到 default，除非 buffer 满了（说明没人读）
	}
}
