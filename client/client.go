package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/ndsky1003/crpc/v3/protocol"
	"github.com/ndsky1003/net/client"
)

type Client struct {
	client     *client.Client
	version    uuid.UUID
	name       string
	weight     int
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

// service 用于本地反射调用
type service struct {
	name    string
	rcvr    reflect.Value
	typ     reflect.Type
	methods map[string]*methodType
}

type methodType struct {
	method    reflect.Method
	ArgType   reflect.Type
	ReplyType reflect.Type
}

func New(ctx context.Context, name, addr string, weight int) (c *Client, err error) {
	var version uuid.UUID
	version, err = uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("failed to generate uuid: %w", err)
	}
	c = &Client{
		version: version,
		name:    name,
		weight:  weight,
	}

	// 使用 net/client 库
	nc, err := client.Dial(ctx, name, addr,
		client.Options().SetHandler(c).SetReconnectInterval(time.Second),
	)
	if err != nil {
		return nil, err
	}

	// 连接成功后自动发送 Verify
	nc.GetOpt().SetOnConnected(func() error {
		return c.sendVerify()
	})

	c.client = nc
	return c, nil
}

// 握手包定义 (需与 Server 端一致)
type VerifyReq struct {
	ServiceName string
	Weight      int
}

func (c *Client) sendVerify() error {
	req := VerifyReq{
		ServiceName: c.name,
		Weight:      c.weight,
	}
	body, _ := json.Marshal(req)
	h := &protocol.CrpcHeader{Type: protocol.TypeVerify}
	packet, _ := protocol.Pack(h, nil, body)
	c.client.Send(context.Background(), packet)
}

// HandleMsg 实现 net.Handler 接口
func (c *Client) HandleMsg(data []byte) error {
	h, _, body, err := protocol.Unpack(data)
	if err != nil {
		return err
	}

	// 1. 如果是 Reply 类型 (我发出去的请求回来了)
	if h.Type == protocol.TypeReply {
		val, ok := c.pending.LoadAndDelete(h.Seq)
		if !ok {
			return nil // 可能已超时或被移除
		}
		call := val.(*Call)

		if h.Error != "" {
			call.Error = errors.New(h.Error)
		} else {
			if call.Reply != nil {
				// 这里为了简单使用了 JSON，你可以换成 v2 的 Coder 接口
				if err := json.Unmarshal(body, call.Reply); err != nil {
					call.Error = err
				}
			}
		}
		call.Done <- call
		return nil
	}

	// 2. 如果是 Call/Broadcast 类型 (别人调我)
	if h.Type == protocol.TypeCall || h.Type == protocol.TypeBroadcast {
		// 路由到本地 invokeLocal
		// 这里的 h.Method 应该是 "Struct.Method" 格式
		// h.ServiceName 应该是 "MyService"
		// 这里的 body 是 args
		go c.handleRemoteCall(h, body)
		return nil
	}

	return nil
}

func (c *Client) handleRemoteCall(h *protocol.CrpcHeader, body []byte) {
	// 查找本地服务
	// 假设 h.Method 传的是 "Func" 名，且我们已通过 RegisterName 注册了服务
	// 这里需要一套简单的协议约定，比如 h.Method = "MethodName"
	// 而 h.ServiceName 用来匹配 c.serviceMap 中的 key

	val, ok := c.serviceMap.Load(h.ServiceName)
	if !ok {
		c.sendReply(h.Seq, "", errors.New("service not found locally"))
		return
	}
	svc := val.(*service)
	mtype, ok := svc.methods[h.Method]
	if !ok {
		c.sendReply(h.Seq, "", errors.New("method not found locally"))
		return
	}

	// 反序列化参数
	argVal := reflect.New(mtype.ArgType.Elem())
	if err := json.Unmarshal(body, argVal.Interface()); err != nil {
		c.sendReply(h.Seq, "", fmt.Errorf("arg unmarshal error: %v", err))
		return
	}

	// 构造返回值
	replyVal := reflect.New(mtype.ReplyType.Elem())

	// 调用
	function := mtype.method.Func
	in := []reflect.Value{svc.rcvr, argVal, replyVal}
	ret := function.Call(in)

	// 检查 error
	if errInter := ret[0].Interface(); errInter != nil {
		c.sendReply(h.Seq, "", errInter.(error))
		return
	}

	// 成功，发送响应 (Broadcast 通常不回包，或者根据业务需求)
	if h.Type == protocol.TypeBroadcast {
		return
	}

	respBody, _ := json.Marshal(replyVal.Interface())
	c.sendReply(h.Seq, string(respBody), nil)
}

func (c *Client) sendReply(seq uint64, bodyStr string, err error) {
	h := &protocol.CrpcHeader{
		Seq:  seq,
		Type: protocol.TypeReply,
	}
	if err != nil {
		h.Error = err.Error()
	}
	// Body 这里的处理需要根据序列化协议统一
	packet, _ := protocol.Pack(h, nil, []byte(bodyStr))
	c.client.Send(context.Background(), packet)
}

// --- 注册机制 (移植自 v2) ---

func (c *Client) RegisterName(name string, rcvr any) error {
	s := new(service)
	s.typ = reflect.TypeOf(rcvr)
	s.rcvr = reflect.ValueOf(rcvr)
	s.name = name
	s.methods = make(map[string]*methodType)

	for m := 0; m < s.typ.NumMethod(); m++ {
		method := s.typ.Method(m)
		mtype := method.Type
		// 假设格式: func (t *T) Method(args *Args, reply *Reply) error
		if mtype.NumIn() != 3 || mtype.NumOut() != 1 {
			continue
		}
		s.methods[method.Name] = &methodType{
			method:    method,
			ArgType:   mtype.In(1),
			ReplyType: mtype.In(2),
		}
	}
	c.serviceMap.Store(name, s)
	return nil
}

// --- 统一调用入口 ---

func (c *Client) Call(ctx context.Context, serviceName, method string, args, reply any, opts ...CallOption) error {
	options := &CallOptions{}
	for _, o := range opts {
		o(options)
	}

	// 1. 本地调用拦截 (需求 4)
	if serviceName == c.name {
		return c.invokeLocal(serviceName, method, args, reply)
	}

	// 2. 远程网络调用
	seq := atomic.AddUint64(&c.seq, 1)
	h := &protocol.CrpcHeader{
		Seq:         seq,
		Type:        protocol.TypeCall,
		ServiceName: serviceName,
		Method:      method,
		TargetSid:   options.TargetSid,
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
