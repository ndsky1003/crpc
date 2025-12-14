// 放一些固定的方法
package client

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/crpc/v3/comm/trace"
	"github.com/ndsky1003/crpc/v3/protocol"
	"github.com/ndsky1003/crpc/v3/protocol/errors"
	"github.com/ndsky1003/crpc/v3/protocol/header"
	"github.com/ndsky1003/crpc/v3/protocol/header/headercode"
	"github.com/ndsky1003/crpc/v3/protocol/header/headerflags"
	"github.com/ndsky1003/crpc/v3/protocol/header/headertype"
	"github.com/ndsky1003/net/conn"
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
	log.Printf("Client %s disconnected, cleared all pending calls: %v", c.Name, err)
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
			//轻操作（仅转发），怕乱序（EOS竞态），所以同步
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

func (c *Client) invoke_local_func(ctx context.Context, mod, method string, metaCoderT coder.T, reqCoderT coder.T, meta, body any) (res any, err error) {
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
	isBroadcast := h.Flags.IsBroadcast()

	if isBroadcast {
		val, ok := c.pending.Load(seq)
		if !ok {
			return nil // 确实找不到了（可能已超时被清理）
		}
		call := val.(*Call)
		d := &broadcastResult{rawBody: body, resCoderT: h.ResCoderT, code: h.Code, IsEOS: h.Flags.IsEOS()}
		select {
		case call.broadcastCh <- d:
		case <-call.subCtx.Done():
			// [安全保护] 消费者已死 (可能用户取消了，或者并发的其他 EOS 导致退出了)
			// 此时直接丢弃消息，不阻塞，也不 panic
			return nil
		default:
			//允许丢包,防止客户端阻塞死了
			//TODO: 看看增买家一个丢包的回调呢
			log.Printf("seq:%v,service:%v:data%+v", h.Seq, h.ToService, d)
		}
		if h.Flags.IsEOS() {
			call.normalStop.Store(true) // 标记为正常结束,可能响应过快最后一个包被丢弃了，但是还是当成正常结束
			c.pending.Delete(seq)
			call.done()
		}
	} else {
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
			if err := coder.Unmarshal(coder.Msgp, body, &resErr); err != nil {
				call.Error = errors.New(errors.ClientInternal, "unmarshal error: "+err.Error())
			} else {
				resErr.WithTraceID(h.TraceID)
				call.Error = &resErr
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
