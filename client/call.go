package client

import (
	"context"
	"sync/atomic"

	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/crpc/v3/protocol/header/headercode"
)

type broadcastResult struct {
	rawBody   []byte       // 原始数据
	res       any          // 已经解码的对象（来自本地优化）
	resCoderT coder.T      // 编码类型
	code      headercode.T // 是否成功
	IsEOS     bool         // 是否是结束标志
}

type Call struct {
	seq   uint64
	Reply any
	Error error

	// middleware 上下文
	ctx *Context

	//broadcast 相关字段 start
	BroadcastResNewFunc  func() any                              // 用于广播调用时创建返回值对象
	BroadcastResCallBack func(ret any, err error, eos bool) bool // 返回true表示继续广播,返回false表示停止广播
	broadcastCh          chan *broadcastResult
	subCtx               context.Context
	subCancel            context.CancelFunc
	//broadcast 相关字段 end

	finished atomic.Bool
	Done     chan *Call
}

func NewCall() *Call {
	return &Call{
		Done: make(chan *Call, 1),
	}
}

func (this *Call) done() {
	if !this.finished.CompareAndSwap(false, true) {
		return
	}

	// 这会让 processBroadcastLoop 安全退出，而不需要关闭 channel
	if this.subCancel != nil {
		this.subCancel()
	}

	if this.ctx != nil {
		this.ctx.invokeHooks(this.Reply, this.Error)
		this.ctx.releaseContext()
		this.ctx = nil
	}

	select {
	case this.Done <- this:
	default:
	}
}
