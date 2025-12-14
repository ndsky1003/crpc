package client

import (
	"context"
	"fmt"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/crpc/v3/comm/trace"
	"github.com/ndsky1003/crpc/v3/comm/ut"
	"github.com/ndsky1003/crpc/v3/protocol"
	"github.com/ndsky1003/crpc/v3/protocol/errors"
	"github.com/ndsky1003/crpc/v3/protocol/header"
	"github.com/ndsky1003/crpc/v3/protocol/header/headercode"
	"github.com/ndsky1003/crpc/v3/protocol/header/headerflags"
	"github.com/ndsky1003/crpc/v3/protocol/header/headertype"
)

// MwInit 初始化中间件
func MwInit(c *Client) HandlerFunc {
	return func(ctx *Context) {
		if ctx.IsAborted() {
			return
		}
		if ctx.Ctx == nil {
			ctx.AbortWithError(errors.New(errors.ClientInvalidArgs, "context is required"))
			return
		}
		if ctx.Service == "" {
			ctx.AbortWithError(errors.New(errors.ClientInvalidArgs, "service name is required"))
			return
		}
		var err error
		ctx.Module, ctx.Func, err = ut.ParseModuleFunc(ctx.Method)
		if err != nil {
			ctx.AbortWithError(errors.New(errors.ClientInternal, err.Error()))
			return
		}

		if tid := ctx.MergedOpt.TraceID; tid != nil {
			ctx.Ctx = trace.WithTraceID(ctx.Ctx, *tid)
		}

		ctx.Seq = atomic.AddUint64(&c.seq, 1)
		call := NewCall()
		call.seq = ctx.Seq
		call.Reply = ctx.Reply
		call.ctx = ctx
		ctx.Call = call
		ctx.Next()
	}
}

// MwHeader 协议头构建中间件
func MwHeader(c *Client) HandlerFunc {
	return func(ctx *Context) {
		if ctx.IsAborted() {
			return
		}
		opt := ctx.MergedOpt
		h := header.Get()
		h.SetType(ctx.CallType).
			SetMetaCoderT(*opt.MetaCoderT).
			SetReqCoderT(*opt.ReqCoderT).
			SetResCoderT(*opt.ResCoderT).
			SetCompressT(*opt.CompressT).
			SetToService(ctx.Service).
			SetModule(ctx.Module).
			SetMethod(ctx.Func).
			SetSeq(ctx.Seq)

		if tid := trace.GetTraceID(ctx.Ctx); tid != "" {
			h.SetTraceID(tid)
		}
		if s := opt.HashKey; s != nil {
			h.SetHashKey(*s)
		}
		if *opt.Debug {
			h.Flags.With(headerflags.Debug)
		}

		var finalDeadline time.Time
		if d, ok := ctx.Ctx.Deadline(); ok {
			finalDeadline = d
		}
		if opt.Timeout != nil && *opt.Timeout > 0 {
			optDeadline := time.Now().Add(*opt.Timeout)
			if finalDeadline.IsZero() || optDeadline.Before(finalDeadline) {
				finalDeadline = optDeadline
			}
		}

		var cancelFn context.CancelFunc
		if !finalDeadline.IsZero() {
			ctx.Ctx, cancelFn = context.WithDeadline(ctx.Ctx, finalDeadline)
			h.Deadline = uint64(finalDeadline.UnixMicro())
		}
		ctx.Call.cleanup = func() {
			if cancelFn != nil {
				cancelFn()
			}
		}
		ctx.Header = h
		ctx.Next()
		// 注意：Header 的 Release 由 MwTransport 或 MwLocal 负责，或者在此处兜底检查（略）
		if ctx.Header != nil {
			h.Release()
			ctx.Header = nil
		}
	}
}

// MwBroadcast 广播处理中间件
func MwBroadcast(c *Client) HandlerFunc {
	return func(ctx *Context) {
		if ctx.IsAborted() {
			return
		}
		opt := ctx.MergedOpt
		if b := opt.Broadcast; b != nil && *b {
			ctx.Header.Flags.Add(headerflags.Broadcast)
			call := ctx.Call
			if opt.BroadcastResNewFunc == nil || opt.BroadcastResCallBack == nil {
				//eg: 这里中断了释放call，会跑到兜底的逻辑里面去
				ctx.AbortWithError(errors.New(errors.ClientInvalidArgs, "BroadcastResNewFunc/CallBack required"))
				return
			}
			call.BroadcastResNewFunc = opt.BroadcastResNewFunc
			call.BroadcastResCallBack = opt.BroadcastResCallBack
			call.broadcastCh = make(chan *broadcastResult, *opt.BroadcastChanCap)
			call.subCtx, call.subCancel = context.WithCancel(context.Background())
			go c.processBroadcastLoop(call.subCtx, call)
		}
		ctx.Next()
	}
}

// MwLocal 本地调用优化中间件
func MwLocal(c *Client) HandlerFunc {
	return func(ctx *Context) {
		if ctx.IsAborted() {
			return
		}
		h := ctx.Header
		isBroadcast := h.Flags.IsBroadcast()
		_, hasLocalModule := c.serviceMap.Load(ctx.Module)
		isUnicastLocal := (c.Name == ctx.Service) && hasLocalModule && (ctx.CallType == headertype.Req) && !h.Flags.IsBroadcast()

		var bodyObj = ctx.Args
		var metaObj any = nil
		if ctx.MergedOpt.Meta != nil {
			metaObj = ctx.MergedOpt.Meta
		}

		runLocal := func() {
			defer func() {
				if r := recover(); r != nil {
					panicErr := fmt.Errorf("local call panic: %v", r)
					if isBroadcast {
						resObj := &broadcastResult{
							code:      headercode.Failed,
							fromLocal: true,
							res:       panicErr,
						}
						select {
						case ctx.Call.broadcastCh <- resObj:
						default:
						}
					} else {
						ctx.Call.Error = panicErr
						ctx.Call.done()
					}
				}
			}()
			res, err := c.invoke_local_func(ctx.Ctx, ctx.Module, ctx.Func, *ctx.MergedOpt.MetaCoderT, *ctx.MergedOpt.ReqCoderT, metaObj, bodyObj)
			if err != nil {
				if e, ok := err.(*errors.Error); ok {
					err = e
				} else {
					err = errors.New(errors.ClientCallError, err.Error())
				}
			}
			call := ctx.Call
			if isBroadcast {
				resObj := &broadcastResult{
					res:       res, //这里可能是res，也可能是err，通过code来判断
					code:      headercode.OK,
					fromLocal: true,
				}
				if err != nil {
					resObj.code = headercode.Failed
					resObj.res = err
				}
				select {
				case call.broadcastCh <- resObj:
				case <-call.subCtx.Done():
				}
			} else {
				if err != nil {
					call.Error = err
				} else if call.Reply != nil && res != nil {
					destVal := reflect.ValueOf(call.Reply)
					if destVal.Kind() == reflect.Pointer && !destVal.IsNil() {
						srcVal := reflect.ValueOf(res)
						if srcVal.Kind() == reflect.Pointer {
							srcVal = srcVal.Elem()
						}
						destElem := destVal.Elem()
						if destElem.CanSet() {
							destElem.Set(srcVal)
						}
					}
				}
				call.done()
			}
		}

		if isUnicastLocal {
			go runLocal() //异步函数里必须手动done了
			ctx.Abort()   //这里没有指定错误，因此不会走到兜底逻辑
			return
		}
		if h.Flags.IsBroadcast() && hasLocalModule {
			h.Flags.Add(headerflags.ExcludeSender)
			go runLocal()
		}
		ctx.Next()
	}
}

// MwCodec 编解码中间件
func MwCodec(c *Client) HandlerFunc {
	return func(ctx *Context) {
		if ctx.IsAborted() {
			return
		}
		var err error
		ctx.BodyBytes, err = coder.Marshal(ctx.Header.ReqCoderT, ctx.Args)
		if err != nil {
			ctx.AbortWithError(errors.New(errors.ClientInternal, err.Error()))
			//交给兜底
			return
		}
		if ctx.MergedOpt.Meta != nil {
			ctx.MetaBytes, err = coder.Marshal(ctx.Header.MetaCoderT, ctx.MergedOpt.Meta)
			if err != nil {
				ctx.AbortWithError(errors.New(errors.ClientInternal, err.Error()))
				//交给兜底
				return
			}
		}
		ctx.Next()
	}
}

// MwTransport 网络传输中间件
func MwTransport(c *Client) HandlerFunc {
	return func(ctx *Context) {
		if ctx.IsAborted() {
			return
		}
		call := ctx.Call
		seq := ctx.Seq
		packet, err := protocol.Pack(ctx.Header, ctx.MetaBytes, ctx.BodyBytes)
		if err != nil {
			ctx.AbortWithError(errors.New(errors.ClientInternal, err.Error()))
			return
		}
		if ctx.CallType != headertype.Send {
			c.pending.Store(seq, call)
		}

		if err := c.client.Sends(ctx.Ctx, packet, &ctx.MergedOpt.Option); err != nil {
			if ctx.CallType != headertype.Send {
				c.pending.Delete(seq)
			}
			ctx.AbortWithError(errors.New(errors.ClientInternal, err.Error()))
			return
		}
		if ctx.CallType != headertype.Send {
			stop := context.AfterFunc(ctx.Ctx, func() {
				if _, loaded := c.pending.LoadAndDelete(seq); loaded {
					call.Error = ctx.Ctx.Err()
					if call.Error == nil {
						call.Error = context.DeadlineExceeded
					}
					call.done()
				}
			})
			oldCleanup := call.cleanup
			call.cleanup = func() {
				stop()
				if oldCleanup != nil {
					oldCleanup()
				}
			}
		}
	}
}
