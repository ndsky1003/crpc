package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/crpc/v3/protocol"
	"github.com/ndsky1003/crpc/v3/protocol/errors"
	"github.com/ndsky1003/crpc/v3/protocol/header"
	"github.com/ndsky1003/crpc/v3/protocol/header/headerstatus"
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

type Call struct {
	Seq   uint64
	Reply any
	Error error
	Done  chan *Call
}

func New(ctx context.Context, addr string, opts ...*Option) (c *Client, err error) {
	opt := Options().
		SetWeight(10).
		SetVerifyJwtExpire(5 * time.Second).
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
	if res_h.Status.IsOk() {
		this.UUID = resp.UUID
		return nil
	}
	return errors.New(errors.ClientInternal, "verification failed: %s", resp.Message)
}

// HandleMsg 实现 net.Handler 接口
func (c *Client) HandleMsg(data []byte) error {
	h, meta, body, err := protocol.Unpack(data)
	if err != nil {
		return errors.New(errors.ClientInternal, err.Error())
	}
	// 1. 如果是 Reply 类型 (我发出去的请求回来了)
	if h.Type.IsRes() {
		defer h.Release()
		val, ok := c.pending.LoadAndDelete(h.Seq)
		if !ok {
			return nil // 可能已超时或被移除
		}
		call := val.(*Call)
		if h.Status.IsOk() { //reply 接收
			if call.Reply != nil {
				if err := coder.Unmarshal(h.ResCoderT, body, call.Reply); err != nil {
					call.Error = errors.New(errors.ClientInternal, err.Error())
				}
			}
		} else {
			var res_err errors.Error
			if err := coder.Unmarshal(coder.Msgp, body, res_err); err != nil {
				call.Error = errors.New(errors.ClientInternal, err.Error())
			}
		}
		call.Done <- call //NOTE: 这里一定不能阻塞，否则影响读消息
		return nil
	}

	// 2. 如果是 Call/Broadcast 类型 (别人调我)
	if h.Type.IsReq() {
		return c.handleRemoteCall(h, meta, body)
	}

	return nil
}

func (c *Client) sendReply(h *header.Header, res any, err error) {
	defer h.Release()
	req_type := h.Type
	var res_type headertype.T
	switch req_type {
	case headertype.Req:
		res_type = headertype.Res
	case headertype.BroadcastReq:
		res_type = headertype.BroadcastRes
	case headertype.Send:
		res_type = headertype.BroadcastRes
	default:
		log.Println("unknown request type:", req_type)
		return
	}
	h.SetType(res_type)
	if err != nil {
		h.SetStatus(headerstatus.Failed).SetResCoderT(coder.Msgp)
		var rpcErr *protocol.Error
		if e, ok := err.(*protocol.Error); ok {
			rpcErr = e
		} else {
			rpcErr = protocol.NewError(500, err.Error())
		}
		body, err := coder.Marshal(coder.Msgp, rpcErr)
		if err != nil {
			log.Println("marshal error:", err)
		}
		data, err := protocol.Pack(h, nil, body)
		if err != nil {
			log.Println("pack error:", err)
		}
		if sendErr := c.SendPacket(context.Background(), data); sendErr != nil {
			log.Println("send error:", sendErr)
		}
		return
	}
	h.SetStatus(headerstatus.StatusOK)
	body, err := coder.Marshal(h.ResCoderT, res)
	if err != nil {
		log.Println("marshal error:", err)
	}

	data, packErr := protocol.Pack(h, nil, []byte(body))
	if packErr != nil {
		log.Println("pack error:", packErr)
	}

	if sendErr := c.SendPacket(context.Background(), data); sendErr != nil {
		log.Println("send error:", sendErr)
	}
}

func (c *Client) handleRemoteCall(h *header.Header, meta, body []byte) error {
	module, ok := c.serviceMap.Load(h.Module)
	if !ok {
		go c.sendReply(h, nil, errors.New("module not found locally"))
		return nil
	}
	if handler, ok := module.(client_handler); ok {
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			resp, err := handler.HandleMsg(h, meta, body, &wg)
			c.sendReply(h, resp, err)
		}()
		wg.Wait()
		return nil
	} else {
		go c.sendReply(h, nil, errors.New("module does not implement client_handler"))
	}
	return nil
}

func (c *Client) SendPacket(ctx context.Context, data [][]byte) error {
	return c.client.Sends(ctx, data)
}

// --- 统一调用入口 ---
func (c *Client) Call(ctx context.Context, serviceName, method string, args, reply any, opts ...*Option) error {
	// opt := Options().Merge(c.opt).Merge(opts...)
	// 1. 本地调用拦截 (需求 4)
	if serviceName == c.Name {
		return c.invokeLocal(serviceName, method, args, reply)
	}

	// 2. 远程网络调用
	seq := atomic.AddUint64(&c.seq, 1)
	h := &header.Header{
		Seq:       seq,
		Type:      protocol.TypeCall,
		ToService: serviceName,
		Method:    method,
		TargetSid: options.TargetSid,
	}
	if options.Broadcast {
		h.Type = protocol.TypeBroadcast
	}

	bodyBytes, _ := json.Marshal(args)
	packet, _ := protocol.Pack(h, nil, bodyBytes)

	call := &Call{
		Seq:   seq,
		Reply: reply,
		Done:  make(chan *Call, 1),
	}

	if !options.Broadcast {
		c.pending.Store(seq, call)
	}

	if err := c.client.Send(ctx, packet); err != nil {
		c.pending.Delete(seq)
		return err
	}

	if options.Broadcast {
		return nil
	}

	select {
	case <-ctx.Done():
		c.pending.Delete(seq)
		return ctx.Err()
	case call := <-call.Done:
		return call.Error
	}
}

func (c *Client) invokeLocal(serviceName, method string, args, reply any) error {
	val, ok := c.serviceMap.Load(serviceName)
	if !ok {
		return fmt.Errorf("local service %s not found", serviceName)
	}
	svc := val.(*service)
	m, ok := svc.methods[method]
	if !ok {
		return fmt.Errorf("local method %s not found", method)
	}

	// 直接反射调用，不进行序列化
	fn := m.method.Func
	ret := fn.Call([]reflect.Value{
		svc.rcvr,
		reflect.ValueOf(args),
		reflect.ValueOf(reply),
	})

	if errInter := ret[0].Interface(); errInter != nil {
		return errInter.(error)
	}
	return nil
}
