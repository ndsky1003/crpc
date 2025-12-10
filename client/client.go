package client

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/crpc/v3/comm/trace"
	"github.com/ndsky1003/crpc/v3/comm/ut"
	"github.com/ndsky1003/crpc/v3/compressor"
	"github.com/ndsky1003/crpc/v3/protocol"
	"github.com/ndsky1003/crpc/v3/protocol/errors"
	"github.com/ndsky1003/crpc/v3/protocol/header"
	"github.com/ndsky1003/crpc/v3/protocol/header/headercode"
	"github.com/ndsky1003/crpc/v3/protocol/header/headerflags"
	"github.com/ndsky1003/crpc/v3/protocol/header/headertype"
	"github.com/ndsky1003/net/client"
	"github.com/ndsky1003/net/conn"
)

type Client struct {
	UUID       uuid.UUID
	Name       string
	client     *client.Client
	opt        *Option
	seq        uint64
	pending    sync.Map // seq -> *Call
	serviceMap sync.Map // map[string]*service (本地服务注册)
}

func New(ctx context.Context, addr string, opts ...*Option) (c *Client, err error) {
	opt := Options().
		SetWeight(ut.GetEnvInt("CRPC_WEIGHT", 10)).
		SetBroadcastChanCap(ut.GetEnvInt("CRPC_BROADCAST_CAP", 64)).
		SetSecret(ut.GetEnv("CRPC_SECRET", "8620506fd4781174ec05fcacf816a12e")).
		SetVerifyJwtExpire(ut.GetEnvDuration("CRPC_JWT_EXPIRE", 5*time.Second)).
		SetDebug(ut.GetEnvBool("CRPC_DEBUG", false)).
		SetMetaCoderT(coder.JSON).
		SetReqCoderT(coder.JSON).
		SetResCoderT(coder.JSON).
		SetCompressT(compressor.Raw).
		Merge(opts...)

	if opt.Name == nil {
		return nil, errors.New(errors.ClientInvalidArgs, "service name is required")
	}

	if addr == "" {
		return nil, errors.New(errors.ClientInvalidArgs, "address is required")
	}

	if opt.Secret == nil {
		return nil, errors.New(errors.ClientInvalidArgs, "secret is required")
	}

	c = &Client{
		Name: *opt.Name,
		opt:  &opt,
	}

	nc, err := client.Dial(ctx, c.Name, addr, client.Options().
		SetHandler(c).
		SetOnConnected(c.onConnected))
	if err != nil {
		return nil, err
	}

	c.client = nc
	return c, nil
}

// onconnect
func (this *Client) onConnected(c *conn.Conn) error {
	secret := *this.opt.Secret
	req := &protocol.VerifyReq{
		UUID:   this.UUID,
		Name:   *this.opt.Name,
		Weight: *this.opt.Weight,
	}
	body, err := coder.Marshal(coder.Msgp, req)
	if err != nil {
		return errors.New(errors.ClientInternal, err.Error())
	}
	time_out := *this.opt.VerifyJwtExpire
	payload := protocol.JwtClaims{
		Data: body,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time_out)),
		},
	}
	ss, err := jwt.NewWithClaims(jwt.SigningMethodHS256, payload).SignedString([]byte(secret))
	if err != nil {
		return errors.New(errors.ClientInternal, err.Error())
	}
	h := header.Get().SetType(headertype.Req)
	h.Flags.With(headerflags.Handshake)
	h.Deadline = uint64(time.Now().Add(5 * time.Second).UnixMicro())
	packets, err := protocol.Pack(h, nil, []byte(ss))
	if err != nil {
		h.Release()
		return errors.New(errors.ClientInternal, err.Error())
	}
	if err := c.Writes(packets); err != nil {
		h.Release()
		return errors.New(errors.ClientInternal, err.Error())
	}
	if err := c.Flush(); err != nil {
		h.Release()
		return errors.New(errors.ClientInternal, err.Error())
	}
	h.Release()

	// 等待验证响应 -----------------------------------------------
	respData, err := c.Read(conn.Options().SetReadTimeout(time_out))
	if err != nil {
		return errors.New(errors.ClientInternal, err.Error())
	}
	res_h, _, resBody, err := protocol.Unpack(respData)
	if err != nil {
		return errors.New(errors.ClientInternal, err.Error())
	}
	defer res_h.Release()

	var claim protocol.JwtClaims
	if _, err := jwt.ParseWithClaims(string(resBody), &claim, func(token *jwt.Token) (any, error) {
		return []byte(secret), nil
	}); err != nil {
		return errors.New(errors.ClientInternal, err.Error())
	}

	var resp protocol.VerifyRes
	if err := coder.Unmarshal(coder.Msgp, claim.Data, &resp); err != nil {
		return errors.New(errors.ClientInternal, err.Error())
	}
	if res_h.Type == headertype.Res && res_h.Flags.IsHandshake() && res_h.Code.IsOK() {
		this.UUID = resp.UUID
		return nil
	}
	return errors.Newf(errors.ClientInternal, "verification failed: %s", resp.Message)
}

// HandleMsg 实现 net.Handler 接口
func (c *Client) HandleMsg(data []byte) error {
	h, meta, body, err := protocol.Unpack(data)
	if err != nil {
		return errors.New(errors.ClientInternal, err.Error())
	}
	ctx := context.Background()
	if h.TraceID != "" {
		ctx = trace.WithTraceID(ctx, h.TraceID)
	}

	bodyCopy := make([]byte, len(body))
	copy(bodyCopy, body)
	switch {
	case h.Type.IsReq():
		metaCopy := make([]byte, len(meta))
		copy(metaCopy, meta)
		go c.handleReq(ctx, h, metaCopy, bodyCopy)
	case h.Type.IsRes():
		if h.Flags.IsBroadcast() {
			if err := c.handleRes(ctx, h, bodyCopy); err != nil {
				log.Println("handleRes :", err)
			}
		} else {
			go c.handleRes(ctx, h, bodyCopy)
		}
	default:
		h.Release()
		return fmt.Errorf("unknown header type: %d", h.Type)
	}
	return nil
}

func (c *Client) handleReq(ctx context.Context, h *header.Header, meta, body []byte) error {
	defer h.Release()
	if h.Deadline > 0 {
		deadlineTime := time.UnixMicro(int64(h.Deadline))
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, deadlineTime)
		defer cancel()
	}
	res, err := c.invoke_local_func(ctx, h.Module, h.Method, h.MetaCoderT, h.ReqCoderT, meta, body)
	return c.sendReply(h, res, err)
}

func (c *Client) invoke_local_func(ctx context.Context, mod, method string, metaCoderT coder.T, reqCoderT coder.T, meta, body []byte) (res any, err error) {
	module, ok := c.serviceMap.Load(mod)
	if !ok {
		err = errors.New(errors.RemoteInternal, "module not found locally")
		return
	}
	if handler, ok := module.(client_handler); !ok {
		err = errors.New(errors.RemoteInternal, "module does not implement client_handler")
		return
	} else {
		res, err = handler.HandleMsg(ctx, method, metaCoderT, reqCoderT, meta, body)
		return
	}
}

func (c *Client) handleRes(_ context.Context, h *header.Header, body []byte) error {
	defer h.Release()
	seq := h.Seq

	// 1. 修正：使用 Load 而不是 LoadAndDelete
	// 防止广播流中间出现 "真空期" 导致后续包丢失
	val, ok := c.pending.Load(seq)
	if !ok {
		return nil // 确实找不到了（可能已超时被清理）
	}
	call := val.(*Call)

	// 2. 统一处理逻辑，减少重复代码
	// 无论是 OK 还是 Error，如果是广播，流程都很像
	isBroadcast := h.Flags.IsBroadcast()

	if isBroadcast {
		d := &broadcastResult{rawBody: body, resCoderT: h.ResCoderT, code: h.Code, IsEOS: h.Flags.IsEOS()}
		select {
		case call.broadcastCh <- d:
		case <-call.ctx.Done():
			// [安全保护] 消费者已死 (可能用户取消了，或者并发的其他 EOS 导致退出了)
			// 此时直接丢弃消息，不阻塞，也不 panic
			return nil
		default:
			//允许丢包,防止客户端阻塞死了
			log.Printf("seq:%v,service:%v:data%+v", h.Seq, h.ToService, d)
		}
		if h.Flags.IsEOS() {
			c.pending.Delete(seq)
			call.done()
		}
	} else {
		defer func() {
			c.pending.Delete(seq)
			call.done()
		}()
		if h.Code.IsOK() {
			if call.Reply != nil {
				if err := coder.Unmarshal(h.ResCoderT, body, call.Reply); err != nil {
					call.Error = errors.New(errors.ClientInternal, err.Error())
				}
			}
		} else {
			var resErr errors.Error
			if err := coder.Unmarshal(coder.Msgp, body, &resErr); err != nil {
				call.Error = errors.New(errors.ClientInternal, "unmarshal error: "+err.Error())
			} else {
				call.Error = &resErr
			}
		}
	}

	return nil
}

func (c *Client) sendReply(h *header.Header, res any, err error) error {
	req_type := h.Type
	res_type := headertype.None
	switch req_type {
	case headertype.Req:
		res_type = headertype.Res
	default:
		return errors.New(errors.ClientInternal, "unknown request type")
	}
	h.SetType(res_type)
	if err != nil {
		h.SetCode(headercode.Failed).SetResCoderT(coder.Msgp)
		var rpcErr *errors.Error
		if e, ok := err.(*errors.Error); ok {
			rpcErr = e
		} else {
			rpcErr = errors.New(errors.RemoteInternal, err.Error())
		}
		body, err := coder.Marshal(coder.Msgp, rpcErr)
		if err != nil {
			return errors.New(errors.ClientInternal, err.Error())
		}
		data, err := protocol.Pack(h, nil, body)
		if err != nil {
			return errors.New(errors.ClientInternal, err.Error())
		}
		if err := c.sendPacket(context.Background(), data); err != nil {
			return errors.New(errors.ClientInternal, err.Error())
		}
		return nil
	}
	h.SetCode(headercode.OK)
	body, err := coder.Marshal(h.ResCoderT, res)
	if err != nil {
		return err
	}

	data, packErr := protocol.Pack(h, nil, []byte(body))
	if packErr != nil {
		return err
	}

	if err := c.sendPacket(context.Background(), data); err != nil {
		return errors.New(errors.ClientInternal, err.Error())
	}
	return nil
}

func (c *Client) sendPacket(ctx context.Context, packet [][]byte, opts ...*Option) error {
	opt := c.opt.Merge(opts...)
	return c.client.Sends(ctx, packet, &opt.Option)
}

func (c *Client) _go(ctx context.Context, ht headertype.T, serviceName, method string, args, reply any, opts ...*Option) (call *Call) {
	call = NewCall()
	if ctx == nil {
		call.Error = errors.New(errors.ClientInvalidArgs, "context is required")
		call.done()
		return
	}
	if serviceName == "" {
		call.Error = errors.New(errors.ClientInvalidArgs, "service name is required")
		call.done()
		return
	}
	module, method, err := c.parseModuleFunc(method)
	if err != nil {
		call.Error = errors.New(errors.ClientInternal, err.Error())
		call.done()
		return
	}
	opt := c.opt.Merge(opts...)

	traceID := ""
	if tid := opt.TraceID; tid != nil {
		traceID = *tid
		ctx = trace.WithTraceID(ctx, traceID)
	}
	seq := atomic.AddUint64(&c.seq, 1)
	h := header.Get()
	h.SetType(ht).
		SetMetaCoderT(*opt.MetaCoderT).
		SetReqCoderT(*opt.ReqCoderT).
		SetResCoderT(*opt.ResCoderT).
		SetCompressT(*opt.CompressT).
		SetToService(serviceName).
		SetModule(module).
		SetMethod(method).
		SetTraceID(traceID).
		SetSeq(seq)

	if s := opt.HashKey; s != nil {
		h.SetHashKey(*s)
	}

	if deadline, ok := ctx.Deadline(); ok {
		h.Deadline = uint64(deadline.UnixMicro())
	} else {
		h.Deadline = 0
	}

	if b := opt.Broadcast; b != nil && *b {
		h.Flags.Add(headerflags.Broadcast)
	}

	if h.Flags.IsBroadcast() {
		if opt.BroadcastResNewFunc == nil || opt.BroadcastResCallBack == nil {
			call.Error = errors.New(errors.ClientInvalidArgs, "BroadcaseResNewFunc and BroadcaseResCallBack are required for broadcast calls")
			call.done()
			return
		}
		call.BroadcastResNewFunc = opt.BroadcastResNewFunc
		call.BroadcastResCallBack = opt.BroadcastResCallBack

		call.broadcastCh = make(chan *broadcastResult, *opt.BroadcastChanCap)
		subCtx, cancel := context.WithCancel(ctx)
		call.ctx = subCtx
		call.cancel = cancel

		// 2. 启动独立的消费协程
		// 将 ctx 传入，以便在请求超时/取消时退出循环
		go c.processBroadcastLoop(subCtx, call)
	}

	isLocalCall := (c.Name == serviceName) && (ht == headertype.Req)
	metaT := *opt.MetaCoderT
	reqT := *opt.ReqCoderT
	resT := *opt.ResCoderT

	if *opt.Debug {
		h.Flags.With(headerflags.Debug)
	}

	bodyBytes, err := coder.Marshal(h.ReqCoderT, args)
	if err != nil {
		h.Release()
		call.Error = errors.New(errors.ClientInternal, err.Error())
		call.done()
		return
	}

	var metaBytes []byte
	if opt.Meta != nil {
		if meta_bytes, err := coder.Marshal(h.MetaCoderT, opt.Meta); err != nil {
			h.Release()
			call.Error = errors.New(errors.ClientInternal, err.Error())
			call.done()
			return
		} else {
			metaBytes = meta_bytes
		}
	}

	packet, err := protocol.Pack(h, metaBytes, bodyBytes)
	h.Release()
	if err != nil {
		call.Error = errors.New(errors.ClientInternal, err.Error())
		call.done()
		return
	}

	call.seq = seq
	call.Reply = reply

	if isLocalCall {

		go func() {
			res, err := c.invoke_local_func(ctx, module, method, metaT, reqT, metaBytes, bodyBytes)
			if err != nil {
				call.Error = err
			} else if call.Reply != nil {
				data, err := coder.Marshal(resT, res)
				if err != nil {
					call.Error = errors.New(errors.ClientInternal, err.Error())
				} else {
					if err := coder.Unmarshal(resT, data, call.Reply); err != nil {
						call.Error = errors.New(errors.ClientInternal, err.Error())
					}
				}
			}
			call.done()
		}()

		return call
	}

	if err := c.client.Sends(ctx, packet, &opt.Option); err != nil {
		call.Error = errors.New(errors.ClientInternal, err.Error())
		call.done()
		return
	}
	if ht != headertype.Send {
		c.pending.Store(seq, call)
	}
	return call
}

// [新增] 广播消费循环
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
			// 上下文取消/超时，停止处理
			return
		case res, ok := <-call.broadcastCh:
			if !ok {
				// Channel 被 handleRes 关闭 (EOS)，说明流结束
				return
			}
			if call.BroadcastResNewFunc == nil || call.BroadcastResCallBack == nil {
				// 安全保护，理论上不应该发生
				return
			}
			var reply any
			var resErr error
			if res.code.IsOK() {
				reply = call.BroadcastResNewFunc()
				if err := coder.Unmarshal(res.resCoderT, res.rawBody, reply); err != nil {
					call.BroadcastResCallBack(nil, errors.New(errors.ClientInternal, "unmarshal error: "+err.Error()), res.IsEOS)
					return
				}
			} else {
				resErr = &errors.Error{}
				if err := coder.Unmarshal(coder.Msgp, res.rawBody, resErr); err != nil {
					// 无法解析错误信息，构造一个通用错误
					call.BroadcastResCallBack(nil, errors.New(errors.ClientInternal, "unmarshal error: "+err.Error()), res.IsEOS)
					return
				}
			}

			cont := call.BroadcastResCallBack(reply, resErr, res.IsEOS)
			if !cont {
				// 用户决定停止接收
				return
			}
		}
	}
}

func (c *Client) parseModuleFunc(raw string) (module, function string, err error) {
	if raw == "" {
		// 建议错误信息更明确
		return "", "", fmt.Errorf("%w: input is empty", errors.ModuleFuncError)
	}

	// before, after, found := strings.Cut(raw, ".")
	idx := strings.LastIndex(raw, ".")
	if idx == -1 {
		return "", "", fmt.Errorf("%w: missing dot separator in '%s'", errors.ModuleFuncError, raw)
	}
	module = raw[:idx]
	function = raw[idx+1:]
	return module, function, nil
}
