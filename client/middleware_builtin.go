package client

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/ndsky1003/crpc/v3/client/broadcastresult"
	"github.com/ndsky1003/crpc/v3/coder"
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
			SetUUID(c.UUID). //避免server再次打包
			SetModule(ctx.Module).
			SetMethod(ctx.Func).
			SetSeq(ctx.Seq)

		if f := ctx.MergedOpt.GenTraceID; f != nil {
			if tid := f(ctx.Ctx); tid != "" {
				h.SetTraceID(tid)
			}
		}
		if s := opt.HashKey; s != nil {
			h.SetHashKey(*s)
		}
		if *opt.Debug {
			h.Flags.With(headerflags.Debug)
			slog.DebugContext(ctx.Ctx, "MwHeader")
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
			call.broadcastCh = make(chan *broadcastresult.Result, *opt.BroadcastChanCap)
			call.subCtx, call.subCancel = context.WithCancel(context.Background())
			if ctx.Header.Flags.IsDebug() {
				slog.DebugContext(ctx.Ctx, "MwBroadcast", "header", ctx.Header)
			}
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

		bodyObj := ctx.Args
		metaObj := ctx.MergedOpt.Meta

		runLocal := func() {
			defer func() {
				if r := recover(); r != nil {
					panicErr := fmt.Errorf("local call panic: %v", r)
					if isBroadcast {
						resObj := broadcastresult.Get()
						resObj.Code = headercode.Failed
						resObj.FromLocal = true
						resObj.Res = panicErr
						ctx.Call.trySendBroadcastResult(resObj)
					} else {
						ctx.Call.Error = panicErr
						ctx.Call.done()
					}
				}
			}()
			// 本地调用，metaObj 和 bodyObj 都是原始对象
			res, err := c.invoke_local_func(ctx.Ctx, ctx.Module, ctx.Func, *ctx.MergedOpt.MetaCoderT, *ctx.MergedOpt.ReqCoderT, metaObj, bodyObj, false)
			if err != nil {
				if e, ok := err.(*errors.Error); ok {
					err = e
				} else {
					err = errors.New(errors.ClientCallError, err.Error())
				}
			}
			call := ctx.Call
			if isBroadcast {
				resObj := broadcastresult.Get()
				resObj.Res = res //这里可能是res，也可能是err，通过code来判断
				resObj.Code = headercode.OK
				resObj.FromLocal = true
				if err != nil {
					resObj.Code = headercode.Failed
					resObj.Res = err
				}
				call.trySendBroadcastResult(resObj)
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
							// 检查类型兼容性，避免panic
							if !srcVal.Type().AssignableTo(destElem.Type()) {
								if srcVal.Type().ConvertibleTo(destElem.Type()) {
									destElem.Set(srcVal.Convert(destElem.Type()))
								} else {
									call.Error = fmt.Errorf("response type mismatch: cannot assign %v to %v", srcVal.Type(), destElem.Type())
								}
							} else {
								destElem.Set(srcVal)
							}
						}
					}
				}
				call.done()
			}
		}

		if isUnicastLocal {
			if ctx.Header.Flags.IsDebug() {
				slog.DebugContext(ctx.Ctx, "MwLocal runLocal", "header", ctx.Header)
			}
			runLocal()  // 同步执行，避免并发问题,如果异步，并发问题发生在调用后对err的赋值，前面兜底那，又在读err。读写同时发生了
			ctx.Abort() // 这里没有指定错误，因此不会走到兜底逻辑
			return
		}
		if h.Flags.IsBroadcast() {
			if hasLocalModule {
				h.Flags.Add(headerflags.ExcludeSender)
				if ctx.Header.Flags.IsDebug() {
					slog.DebugContext(ctx.Ctx, "MwLocal broadcast", "header", ctx.Header)
				}
				go runLocal() //异步的目的是希望本地与远程同时发生
			} else {
				ctx.Call.localStop.Store(true) //无本地调用默认成功
			}
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
		if ctx.Header.Flags.IsDebug() {
			slog.DebugContext(ctx.Ctx, "MwCodec", "header", ctx.Header)
		}
		var err error
		if ctx.Header.Flags.IsDebug() {
			slog.DebugContext(ctx.Ctx, "body Marshal", "reqcodert", ctx.Header.ReqCoderT)
		}
		ctx.BodyBytes, err = coder.Marshal(ctx.Header.ReqCoderT, ctx.Args)
		if err != nil {
			ctx.AbortWithError(errors.New(errors.ClientInternal, err.Error()))
			//交给兜底
			return
		}
		if ctx.MergedOpt.Meta != nil {
			if ctx.Header.Flags.IsDebug() {
				slog.DebugContext(ctx.Ctx, "meta Marshal", "meta coder t", ctx.Header.MetaCoderT)
			}
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
		if ctx.Header.Flags.IsDebug() {
			slog.DebugContext(ctx.Ctx, "MwTransport", "header", ctx.Header)
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
			//保证取消的时候直接删掉call，即使现在网络上回来，也已经过时了
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

		if err := c.client.Sends(ctx.Ctx, packet, &ctx.MergedOpt.Option); err != nil {
			if ctx.CallType != headertype.Send {
				c.pending.Delete(seq)
			}
			ctx.AbortWithError(errors.New(errors.ClientInternal, err.Error()))
			return
		}
	}
}
