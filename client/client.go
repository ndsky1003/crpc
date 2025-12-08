package client

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/ndsky1003/crpc/v3/coder"
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
		SetWeight(10).
		SetVerifyJwtExpire(5 * time.Second).
		SetDebug(false).
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

	nc, err := client.Dial(ctx, *opt.Name, addr, client.Options().
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
	payload := protocol.JwtClaims{
		Data: body,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(*this.opt.VerifyJwtExpire)),
		},
	}
	ss, err := jwt.NewWithClaims(jwt.SigningMethodES256, payload).SignedString(secret)
	if err != nil {
		return errors.New(errors.ClientInternal, err.Error())
	}
	h := header.Get().SetType(headertype.VerifyReq)
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
	// 等待验证响应
	respData, err := c.Read()
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
	if res_h.Code.IsOK() {
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
	switch {
	case h.Type.IsReq():
		var wg sync.WaitGroup
		var err error
		wg.Add(1)
		go func() {
			err = c.handleReq(h, meta, body, &wg)
		}()
		wg.Wait()
		return err
	case h.Type.IsRes():
		return c.handleRes(h, body)
	default:
		h.Release()
		return fmt.Errorf("unknown header type: %d", h.Type)
	}
}

func (c *Client) handleReq(h *header.Header, meta, body []byte, wg *sync.WaitGroup) error {
	defer h.Release()
	res, err := c.invoke_local_func(h.Module, h.Method, h.MetaCoderT, h.ReqCoderT, meta, body, wg)
	return c.sendReply(h, res, err)
}

func (c *Client) invoke_local_func(mod, method string, metaCoderT coder.T, reqCoderT coder.T, meta, body []byte, wg *sync.WaitGroup) (res any, err error) {
	module, ok := c.serviceMap.Load(mod)
	if !ok {
		wg.Done()
		err = errors.New(errors.RemoteInternal, "module not found locally")
		return
	}
	if handler, ok := module.(client_handler); !ok {
		wg.Done()
		err = errors.New(errors.RemoteInternal, "module does not implement client_handler")
		return
	} else {
		res, err = handler.HandleMsg(method, metaCoderT, reqCoderT, meta, body, wg)
		return
	}
}

func (c *Client) handleRes(h *header.Header, body []byte) error {
	defer h.Release()
	seq := h.Seq

	// 1. 修正：使用 Load 而不是 LoadAndDelete
	// 防止广播流中间出现 "真空期" 导致后续包丢失
	val, ok := c.pending.Load(seq)
	if !ok {
		return nil // 确实找不到了（可能已超时被清理）
	}
	call := val.(*call_)

	// 标记该次调用是否应该结束（从 Map 移除 + Close Channel）
	shouldFinish := false

	// 2. 统一处理逻辑，减少重复代码
	// 无论是 OK 还是 Error，如果是广播，流程都很像
	isBroadcast := h.Type == headertype.BroadcastRes

	if h.Code.IsOK() {
		// --- 成功处理 ---
		if isBroadcast {
			// 广播成功
			if call.BroadcaseResNewFunc == nil || call.BroadcaseResCallBack == nil {
				// 配置错误：直接结束，不留后患
				call.Error = errors.New(errors.ClientInternal, "broadcast callbacks missing")
				shouldFinish = true
			} else {
				reply := call.BroadcaseResNewFunc()
				if err := coder.Unmarshal(h.ResCoderT, body, reply); err != nil {
					// 反序列化失败：视为严重错误，终止广播
					call.Error = errors.New(errors.ClientInternal, err.Error())
					shouldFinish = true
				} else {
					// 触发用户回调
					cont := call.BroadcaseResCallBack(reply, nil)
					// 只有当 (用户想继续) 且 (服务端没发EOS) 时，才继续保留
					if !cont || h.Flags.IsEOS() {
						shouldFinish = true
					}
				}
			}
		} else {
			// 普通 RPC 成功
			if call.Reply != nil {
				if err := coder.Unmarshal(h.ResCoderT, body, call.Reply); err != nil {
					call.Error = errors.New(errors.ClientInternal, err.Error())
				}
			}
			shouldFinish = true
		}
	} else {
		// --- 错误处理 ---
		// 解析服务端传回的错误信息
		var resErr errors.Error
		// 3. 修正：传入指针 &resErr
		if err := coder.Unmarshal(coder.Msgp, body, &resErr); err != nil {
			// 如果连错误包都解不开，只好报内部错误
			call.Error = errors.New(errors.ClientInternal, "unmarshal error: "+err.Error())
		} else {
			call.Error = &resErr
		}

		if isBroadcast {
			// 广播出错：通知用户，并询问是否继续（有些错误可能不致命？）
			if call.BroadcaseResCallBack != nil {
				// 传 nil reply, 传 error
				cont := call.BroadcaseResCallBack(nil, call.Error)
				if !cont || h.Flags.IsEOS() {
					shouldFinish = true
				}
			} else {
				shouldFinish = true
			}
		} else {
			// 普通 RPC 出错：直接结束
			shouldFinish = true
		}
	}

	// 4. 统一收尾
	if shouldFinish {
		// 只有在真正结束时，才从 Map 中移除
		c.pending.Delete(seq)
		call.done()
	}

	return nil
}

func (c *Client) sendReply(h *header.Header, res any, err error) error {
	req_type := h.Type
	res_type := headertype.None
	switch req_type {
	case headertype.Req:
		res_type = headertype.Res
	case headertype.BroadcastReq:
		res_type = headertype.BroadcastRes
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

func (c *Client) Broadcast(ctx context.Context, serviceName, method string, args, reply any, opts ...*Option) error {
	call := c._go(ctx, headertype.BroadcastReq, serviceName, method, args, reply, opts...)
	defer call.Release()
	if call.Error != nil {
		return call.Error
	}
	select {
	case <-ctx.Done():
		call.Error = errors.New(errors.ClientInternal, ctx.Err().Error())
		c.pending.Delete(call.seq)
		return ctx.Err()
	case call := <-call.Done:
		return call.Error
	}
}

func (c *Client) Call(ctx context.Context, serviceName, method string, args, reply any, opts ...*Option) error {
	call := c.Go(ctx, serviceName, method, args, reply, opts...)
	defer call.Release()
	if call.Error != nil {
		return call.Error
	}
	select {
	case <-ctx.Done():
		call.Error = errors.New(errors.ClientInternal, ctx.Err().Error())
		c.pending.Delete(call.seq)
		return ctx.Err()
	case call := <-call.Done:
		return call.Error
	}
}

// WARNING: 需要自行释放call
func (c *Client) Go(ctx context.Context, serviceName, method string, args, reply any, opts ...*Option) (call *call_) {
	return c._go(ctx, headertype.Req, serviceName, method, args, reply, opts...)
}

func (c *Client) Send(ctx context.Context, serviceName, method string, args, reply any, opts ...*Option) error {
	call := c._go(ctx, headertype.Send, serviceName, method, args, reply, opts...)
	defer call.Release()
	return call.Error
}

// WARNING: 线程安全
// Call need release manually after use
func (c *Client) _go(ctx context.Context, ht headertype.T, serviceName, method string, args, reply any, opts ...*Option) (call *call_) {
	call = GetCall()
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

	if ht == headertype.BroadcastReq {
		if opt.BroadcastResNewFunc == nil || opt.BroadcastResCallBack == nil {
			call.Error = errors.New(errors.ClientInvalidArgs, "BroadcaseResNewFunc and BroadcaseResCallBack are required for broadcast calls")
			call.done()
			return
		}
		call.BroadcaseResNewFunc = opt.BroadcastResNewFunc
		call.BroadcaseResCallBack = opt.BroadcastResCallBack
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
		SetSeq(seq)
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
			var dummyWg sync.WaitGroup
			dummyWg.Add(1)
			res, err := c.invoke_local_func(module, method, metaT, reqT, metaBytes, bodyBytes, &dummyWg)

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

func (c *Client) parseModuleFunc(raw string) (module, function string, err error) {
	if raw == "" {
		// 建议错误信息更明确
		return "", "", fmt.Errorf("%w: input is empty", errors.ModuleFuncError)
	}

	before, after, found := strings.Cut(raw, ".")

	if !found {
		return "", "", fmt.Errorf("%w: missing dot separator in '%s'", errors.ModuleFuncError, raw)
	}

	if before == "" || after == "" {
		return "", "", fmt.Errorf("%w: invalid format '%s', expect 'module.function'", errors.ModuleFuncError, raw)
	}

	return before, after, nil
}
