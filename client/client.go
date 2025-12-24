package client

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ndsky1003/crpc/v3/buffer/netpool"
	"github.com/ndsky1003/crpc/v3/client/broadcastresult"
	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/crpc/v3/comm/ut"
	"github.com/ndsky1003/crpc/v3/compressor"
	"github.com/ndsky1003/crpc/v3/protocol/errors"
	"github.com/ndsky1003/crpc/v3/protocol/header/headertype"
	"github.com/ndsky1003/net/v2/client"
	"github.com/ndsky1003/net/v2/conn"
)

type Client struct {
	UUID       uuid.UUID
	Name       string
	client     *client.Client
	opt        *Option
	seq        uint64
	pending    sync.Map // seq -> *Call
	serviceMap sync.Map // map[string]*service (本地服务注册)
	handlers   HandlersChain
}

func Dial(ctx context.Context, name string, addr string, opts ...*Option) (c *Client, err error) {
	return New(ctx, name, addr, opts...)
}

func New(ctx context.Context, name string, addr string, opts ...*Option) (c *Client, err error) {
	opt := Options().
		SetWeight(ut.GetEnvInt("CRPC_WEIGHT", 10)).
		SetBroadcastChanCap(ut.GetEnvInt("CRPC_BROADCAST_CAP", 64)).
		SetSecret(ut.GetEnv("CRPC_SECRET", "")).
		SetVerifyJwtExpire(ut.GetEnvDuration("CRPC_JWT_EXPIRE", 5*time.Second)).
		SetDebug(ut.GetEnvBool("CRPC_DEBUG", false)).
		SetTimeout(10 * time.Second).
		SetMetaCoderT(coder.JSON).
		SetReqCoderT(coder.JSON).
		SetResCoderT(coder.JSON).
		SetCompressT(compressor.Raw).
		WithConn(func(o *client.Option) {
			o.WithConn(func(oo *conn.Option) {
				oo.GenBufFn = func() []byte {
					return netpool.Get()
				}
			})
		}).Merge(opts...)

	if name == "" {
		return nil, errors.New(errors.ClientInvalidArgs, "service name is required")
	}

	if addr == "" {
		return nil, errors.New(errors.ClientInvalidArgs, "address is required")
	}

	if s := opt.Secret; s == nil || *s == "" {
		return nil, errors.New(errors.ClientInvalidArgs, "secret is required")
	}

	c = &Client{
		Name: name,
		opt:  &opt,
	}

	c.handlers = HandlersChain{
		MwInit(c),
		MwHeader(c),
		MwBroadcast(c),
		MwLocal(c),
		MwCodec(c),
		MwTransport(c),
	}

	nc, err := client.Dial(ctx, c.Name, addr, &opt.Option, client.Options().
		SetHandler(c).
		SetOnDisconnected(c.onDisconnected).
		SetOnConnected(c.onConnected))
	if err != nil {
		return nil, errors.New(errors.ClientInternal, err.Error())
	}
	c.client = nc
	return c, nil
}

// executeChain 执行中间件链
func (c *Client) executeChain(ctx context.Context, callType headertype.T, service, method string, args, reply any, opts ...*Option) *Call {
	opt := c.opt.Merge(opts...)
	mCtx := &Context{
		Ctx:       ctx,
		Service:   service,
		Method:    method,
		CallType:  callType,
		Args:      args,
		Reply:     reply,
		MergedOpt: &opt,
		index:     -1,
		hooks:     make([]responseHook, 0, 4),
	}

	// 这里直接使用 Client 配置好的 Chain
	// 如果需要支持 Option 级别的中间件，可以在这里拼接
	mCtx.handlers = c.handlers

	if len(c.handlers) == 0 {
		mCtx.SetError(errors.New(errors.ClientInternal, "middware is empty"))
	}

	mCtx.Next()

	//Context <=> 尚未绑定上关系的时候
	if mCtx.Call == nil {
		dummyCall := NewCall()
		if mCtx.Err() != nil {
			dummyCall.Error = mCtx.Err()
		} else {
			dummyCall.Error = errors.New(errors.ClientCanceled, "request aborted unexpectly")
		}
		dummyCall.ctx = mCtx
		mCtx.Call = dummyCall
	}

	//兜底逻辑：处理有错误但中间件可能忘记收尾的情况
	if err := mCtx.Err(); err != nil {
		if mCtx.Call.Error == nil {
			mCtx.Call.Error = err
		}
		mCtx.Call.done()
	}

	return mCtx.Call
}

func (c *Client) dispatchBroadcast(call *Call, res *broadcastresult.Result, isEOS bool) bool {
	defer broadcastresult.Put(res)
	if call.BroadcastResNewFunc == nil || call.BroadcastResCallBack == nil {
		return false
	}
	var reply any
	var resErr error
	if res.FromLocal { // 本地调用优化，直接有对象
		if res.Code.IsOK() {
			reply = res.Res
		} else {
			if res.Res != nil { //空返回,这里就是nil
				if err, ok := res.Res.(error); ok {
					if er, ok1 := err.(*errors.Error); ok1 {
						resErr = er
					} else {
						resErr = errors.Newf(errors.ClientCallError, "err:%+v", err)
					}
				} else {
					resErr = errors.Newf(errors.ClientReturnInvalid, "invalid return ,ret:%+v is not error", res)
				}
			}
		}
	} else { //remote
		if res.Code.IsOK() {
			reply = call.BroadcastResNewFunc()
			if err := coder.Unmarshal(res.ResCoderT, res.RawBody, reply); err != nil {
				resErr = errors.New(errors.ClientInternal, "unmarshal error: "+err.Error())
			}
		} else {
			tmpErr := &errors.Error{}
			if err := coder.Unmarshal(res.ResCoderT, res.RawBody, tmpErr); err != nil {
				resErr = errors.New(errors.ClientInternal, "unmarshal error: "+err.Error())
			}
			if tmpErr.Code != errors.None {
				resErr = tmpErr
			}
		}
	}

	if call.ctx != nil {
		call.ctx.invokeHooks(reply, resErr)
	}
	return call.BroadcastResCallBack(reply, resErr, isEOS)
}

func (c *Client) processBroadcastLoop(ctx context.Context, call *Call) {
	// 保证退出时清理 pending (虽然 handleRes 也会清理，但双重保险)
	// 同时也防止用户回调返回 false 后，pending map 中还有残留
	defer func() {
		c.pending.Delete(call.seq)
		call.done()
	}()

	for {
		select {
		case <-ctx.Done():
			// 上下文取消时，尝试排空 channel
			// 防止因为 handleRes 设置了 EOS 并调用了 done，导致 select 随机选中此分支而丢失 EOS
			for {
				select {
				case res, ok := <-call.broadcastCh:
					if !ok {
						return
					}
					isEOS := call.fixStop(res)
					// 处理残留消息,也就是最后一条消息无法确定select选中上面，还是下面
					if !c.dispatchBroadcast(call, res, isEOS) {
						return
					}
					if isEOS {
						return
					}
				default:
					// 通道空了，跳出排空循环，去报超时
					goto TIMEOUT_EXIT
				}
			}
		TIMEOUT_EXIT:
			// 【关键步骤 B】：补发超时通知
			// 既然走到这里，说明还没遇到 EOS 就被掐断了。
			// 我们需要人工合成一个 (err=Timeout, eos=true) 的回调。

			// 优先取 call.Error (由 AfterFunc 设置的准确错误)
			finalErr := call.Error
			if finalErr == nil {
				finalErr = ctx.Err() // 兜底
			}
			// 告诉用户：结束了(EOS=true)，原因是 finalErr
			if f := call.BroadcastResCallBack; f != nil {
				f(nil, finalErr, true)
			}
		case res, ok := <-call.broadcastCh:
			if !ok {
				return
			}
			isEOS := call.fixStop(res)
			if !c.dispatchBroadcast(call, res, isEOS) {
				return
			}
			if isEOS {
				return
			}
		}
	}
}
