package client

import (
	"context"
	"log/slog"
	"sync/atomic"

	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/crpc/v3/protocol/errors"
	"github.com/ndsky1003/crpc/v3/protocol/header/headercode"
)

type broadcastResult struct {
	rawBody   []byte       // 原始数据
	res       any          // 已经解码的对象（来自本地优化）,可以是返回值，也可以是错误，通过code判断
	resCoderT coder.T      // 编码类型
	code      headercode.T // 是否成功
	IsEOS     bool         // 是否是结束标志
	fromLocal bool         // 标记是否来自本地调用 ,支持空返回值，所以必须用一个独立的字段来判断
}

type Call struct {
	seq   uint64
	Reply any
	Error error

	// middleware 上下文
	ctx *Context

	//broadcast 相关字段 start
	BroadcastResNewFunc func() any // 用于广播调用时创建返回值对象
	// 返回true表示继续广播,返回false表示停止广播,EOS只是表示不再有数据,比如超时了，err也存在，eos为true
	//EOS 可能服务器会收到2条
	// 1.超时
	// 2.正常探测到最后一个返回值 ,这2个不在一个goroutine
	//但是client本地会过滤掉后达到的那条。最终结果有可能远端执行了所有，但是收到超时的EOS
	BroadcastResCallBack func(ret any, err error, eos bool) bool
	broadcastCh          chan *broadcastResult
	subCtx               context.Context
	subCancel            context.CancelFunc
	normalStop           atomic.Bool //表示是否收到过EOS
	localStop            atomic.Bool //表示是否收到过EOS
	//broadcast 相关字段 end

	cleanup func()

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

	if this.cleanup != nil {
		this.cleanup()
	}

	// 这会让 processBroadcastLoop 安全退出，而不需要关闭 channel
	if this.subCancel != nil {
		this.subCancel()
	}

	if this.ctx != nil {
		this.ctx.invokeHooks(this.Reply, this.Error)
		this.ctx = nil
	}

	select {
	case this.Done <- this:
	default:
	}
}

// 不是一定是单线程执行，理论上有2个，一个是消费线程，一个是send满的那个goroutine
// NOTE: 概率小的可怜
func (c *Call) fixStop(res *broadcastResult) (isEOS bool) {
	if res.fromLocal {
		c.localStop.Store(true)
	}
	if res.IsEOS {
		c.normalStop.Store(true)
	}
	isEOS = c.localStop.Load() && c.normalStop.Load()
	return
}

// trySendBroadcastResult 尝试发送广播结果，如果通道满则丢弃，防止阻塞
func (c *Call) trySendBroadcastResult(res *broadcastResult) {
	select {
	case c.broadcastCh <- res:
	case <-c.subCtx.Done():
		err := c.subCtx.Err()
		slog.Error("subCtx Cancel", "err", err, "res", res)
	default:
		c.fixStop(res)
		err := errors.New(errors.ClientInternal, "广播通道满，丢弃消息")
		slog.Error("broadcastCh exhaust", "err", err, "res", res)
	}
}
