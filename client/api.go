package client

import (
	"context"

	"github.com/ndsky1003/crpc/v3/protocol/errors"
	"github.com/ndsky1003/crpc/v3/protocol/header/headertype"
)

// finalMiddleware 构造最后一环：调用底层的 _go 进行网络发送
func (c *Client) finalMiddleware(callType headertype.T) HandlerFunc {
	return func(ctx *Context) {
		// 使用 ctx.Ctx (可能被中间件修改过)
		call := c._go(ctx.Ctx, callType, ctx.ServiceName, ctx.Method, ctx.Args, ctx.Reply, ctx.Opts...)

		// 双向绑定：让 Context 和 Call 互相引用
		// 1. Context 持有 Call，以便中间件后续可以访问 Call 的信息
		ctx.Call = call
		ctx.Err = call.Error

		// 2. Call 持有 Context，以便 Call 结束时触发回调并释放 Context
		call.ctx = ctx
	}
}

// executeChain 统一执行流
func (c *Client) executeChain(ctx context.Context, callType headertype.T, serviceName, method string, args, reply any, opts ...*Option) *Call {
	// 1. 从 Pool 获取 Context
	mCtx := obtainContext()

	// 2. 初始化
	mCtx.Ctx = ctx
	mCtx.ServiceName = serviceName
	mCtx.Method = method
	mCtx.Args = args
	mCtx.Reply = reply
	mCtx.Opts = opts

	mCtx.handlers = mCtx.handlers[:0]
	if len(c.handlers) > 0 {
		mCtx.handlers = append(mCtx.handlers, c.handlers...)
	}
	mCtx.handlers = append(mCtx.handlers, c.finalMiddleware(callType))

	mCtx.Next()

	// 如果被 Abort 了，finalMiddleware 没执行，Call 是 nil
	if mCtx.Call == nil {
		// 创建一个"伪造"的 Call 对象，用于承载错误信息
		// 这样调用方拿到 Call 后，访问 Call.Error 能看到拦截原因
		dummyCall := NewCall()
		if mCtx.Err != nil {
			dummyCall.Error = mCtx.Err
		} else {
			// 如果用户 Abort 了但没设置 Err，给一个默认错误
			dummyCall.Error = errors.New(errors.ClientCanceled, "request aborted by middleware")
		}
		dummyCall.done() // 立即结束

		// 这里的 releaseContext 很重要，因为 finalMiddleware 没跑，没绑定到 Call 上
		// 所以必须在这里手动释放！
		mCtx.releaseContext()

		return dummyCall
	}

	// 情况 B: _go 内部报错 (Critical Fix)
	// Call 对象存在，但已有 Error，说明 _go 内部已经调用过 done() 了。
	// 此时 mCtx 还没来得及被 done() 释放，必须在这里手动释放！
	if mCtx.Call.Error != nil {
		mCtx.invokeHooks(mCtx.Call.Error) // 触发 hooks (如监控耗时)
		mCtx.releaseContext()             // 归还 Context 到池子
		mCtx.Call.ctx = nil               // 断开引用，防止野指针
		return mCtx.Call
	}
	// 5. 返回 Call 对象
	// 注意：Context 的释放权现在移交给了 Call (在 call.done() 中释放)
	return mCtx.Call
}

func (c *Client) Call(ctx context.Context, serviceName, method string, args, reply any, opts ...*Option) error {
	call := c.executeChain(ctx, headertype.Req, serviceName, method, args, reply, opts...)
	// call := c.Go(ctx, serviceName, method, args, reply, opts...)
	if call.Error != nil {
		return call.Error
	}
	select {
	case <-ctx.Done():
		call.Error = errors.New(errors.ClientInternal, ctx.Err().Error())
		if _, loaded := c.pending.LoadAndDelete(call.seq); loaded {
			call.Error = errors.New(errors.ClientInternal, ctx.Err().Error())
			call.done()
		}
		return ctx.Err()
	case call := <-call.Done:
		return call.Error
	}
}

func (c *Client) Go(ctx context.Context, serviceName, method string, args, reply any, opts ...*Option) (call *Call) {
	// return c._go(ctx, headertype.Req, serviceName, method, args, reply, opts...)
	return c.executeChain(ctx, headertype.Req, serviceName, method, args, reply, opts...)
}

func (c *Client) Send(ctx context.Context, serviceName, method string, args any, opts ...*Option) error {
	// call := c._go(ctx, headertype.Send, serviceName, method, args, nil, opts...)
	// return call.Error
	call := c.executeChain(ctx, headertype.Send, serviceName, method, args, nil, opts...)

	// Send 模式下，_go 内部通常不会把 Call 放入 pending map
	// 所以我们需要确保 Context 能够被释放。
	// 如果没有错误，call.done() 可能没被调用（取决于 _go 实现，通常 Send 不等回包）
	// 安全起见，手动调用 done() 确保 release
	// 之前不需要done,是因为call不是pool里出来的,done里主要是想像释放Context
	call.done()

	return call.Error
}

func (c *Client) Close() error {
	return c.client.Close()
}
