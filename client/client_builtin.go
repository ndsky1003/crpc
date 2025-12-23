// 放一些固定的方法
package client

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/ndsky1003/crpc/v3/buffer/netpool"
	"github.com/ndsky1003/crpc/v3/client/broadcastresult"
	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/crpc/v3/protocol"
	"github.com/ndsky1003/crpc/v3/protocol/errors"
	"github.com/ndsky1003/crpc/v3/protocol/header"
	"github.com/ndsky1003/crpc/v3/protocol/header/headercode"
	"github.com/ndsky1003/crpc/v3/protocol/header/headerflags"
	"github.com/ndsky1003/crpc/v3/protocol/header/headertype"
	"github.com/ndsky1003/net/v2/conn"
)

func (c *Client) onDisconnected(err error) {
	closeErr := errors.New(errors.ClientCanceled, fmt.Sprintf("connection reset: %v", err))
	c.pending.Range(func(key, value any) bool {
		seq := key.(uint64)
		call := value.(*Call)
		call.Error = closeErr
		call.done()
		c.pending.Delete(seq)
		return true // 继续遍历下一个
	})
	slog.Info("onDisconnected, cleared all pending calls", "name", c.Name, "err", err)
}

// onconnect
func (this *Client) onConnected(c *conn.Conn) error {
	secret := *this.opt.Secret
	req := &protocol.VerifyReq{
		UUID:   this.UUID,
		Name:   this.Name,
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
	data, err := c.Read()
	if err != nil {
		return errors.New(errors.ClientInternal, err.Error())
	}
	defer netpool.Release(data)
	res_h, _, resBody, err := protocol.Unpack(data)
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

func (c *Client) HandleMsg(data []byte) error {

	h, meta, body, err := protocol.Unpack(data)
	if err != nil {
		return errors.New(errors.ClientInternal, err.Error())
	}
	ctx := context.Background()
	if h.TraceID != "" && c.opt.WithTraceID != nil {
		ctx = c.opt.WithTraceID(ctx, h.TraceID)
	}
	//返回的广播合并成单线程
	if h.Type.IsRes() && h.Flags.IsBroadcast() {
		defer h.Release()
		if err := c.handleRes(ctx, h, body, data); err != nil {
			slog.Error("handleRes", "err", err)
		}
		return nil
	}

	switch {
	case h.Type.IsReq():
		go func() {
			defer netpool.Release(data)
			defer h.Release()
			if err := c.handleReq(ctx, h, meta, body); err != nil {
				slog.Error("handleReq", "err", err)
			}
		}()
	case h.Type.IsRes():
		go func() {
			if err := c.handleRes(ctx, h, body, data); err != nil {
				slog.Error("handleRes", "err", err)
			}
		}()
	default:
		slog.Error("unknown header type", "type", h.Type)
	}
	return nil
}

func (c *Client) handleReq(ctx context.Context, h *header.Header, meta, body []byte) error {
	if h.Flags.IsDebug() {
		slog.DebugContext(ctx, "handleReq", "header", h)
	}
	if h.Deadline > 0 {
		deadlineTime := time.UnixMicro(int64(h.Deadline))
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, deadlineTime)
		defer cancel()
	}
	// 网络请求，meta 和 body 都是 []byte
	res, err := c.invoke_local_func(ctx, h.Module, h.Method, h.MetaCoderT, h.ReqCoderT, meta, body, true)
	return c.sendReply(h, res, err)
}

func (c *Client) invoke_local_func(ctx context.Context, mod, method string, metaCoderT coder.T, reqCoderT coder.T, meta, body any, fromNetwork bool) (res any, err error) {
	module, ok := c.serviceMap.Load(mod)
	if !ok {
		err = errors.New(errors.RemoteInternal, "module not found locally")
		return
	}
	if handler, ok := module.(client_handler); !ok {
		err = errors.New(errors.RemoteInternal, "module does not implement client_handler")
		return
	} else {
		res, err = handler.HandleMsg(ctx, method, metaCoderT, reqCoderT, meta, body, fromNetwork)
		return
	}
}

func (c *Client) handleRes(ctx context.Context, h *header.Header, body []byte, data []byte) error {
	seq := h.Seq
	isBroadcast := h.Flags.IsBroadcast()
	if h.Flags.IsDebug() {
		slog.DebugContext(ctx, "handleRes", "header", h)
	}
	if isBroadcast {
		//保证只收到一个EOS
		var val any
		var ok bool
		if h.Flags.IsEOS() { //server那边2个goroutine，一个超时，一个正常返回的goroutine
			val, ok = c.pending.LoadAndDelete(seq)
			if !ok {
				if h.Flags.IsDebug() {
					slog.DebugContext(ctx, "ddd")
				}
				netpool.Release(data)
				return nil // 确实找不到了（可能已超时被清理）
			}
		} else {
			val, ok = c.pending.Load(seq)
			if !ok {
				if h.Flags.IsDebug() {
					slog.DebugContext(ctx, "ddd")
				}
				netpool.Release(data)
				return nil // 确实找不到了（可能已超时被清理）
			}
		}
		call := val.(*Call)
		d := broadcastresult.Get()
		d.RawBody = body
		d.ResCoderT = h.ResCoderT
		d.Code = h.Code
		d.IsEOS = h.Flags.IsEOS()
		d.ReleaseCallback = func() {
			netpool.Release(data)
		}
		if h.Flags.IsDebug() {
			slog.DebugContext(ctx, "broadcast receive", "data", d)
		}
		call.trySendBroadcastResult(d)
	} else {
		defer netpool.Release(data)
		val, ok := c.pending.LoadAndDelete(seq)
		if !ok {
			return nil // 确实找不到了（可能已超时被清理）
		}
		call := val.(*Call)
		if h.Code.IsOK() {
			if call.Reply != nil {
				if err := coder.Unmarshal(h.ResCoderT, body, call.Reply); err != nil {
					call.Error = errors.New(errors.ClientInternal, err.Error())
				}
			}
			call.done()
		} else {
			var resErr errors.Error
			if err := coder.Unmarshal(h.ResCoderT, body, &resErr); err != nil {
				call.Error = errors.New(errors.ClientInternal, "unmarshal error: "+err.Error())
			} else {
				if resErr.Code != errors.None {
					if h.TraceID != "" {
						resErr.WithTraceID(h.TraceID)
					}
					call.Error = &resErr
				}
			}
			call.done()
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
	case headertype.Send: // 显式处理
		return nil
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
