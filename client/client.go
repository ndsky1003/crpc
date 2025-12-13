package client

import (
	"context"
	"fmt"
	"log"
	"reflect"
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
	handlers   HandlersChain
}

func Dial(ctx context.Context, name string, addr string, opts ...*Option) (c *Client, err error) {
	return New(ctx, name, addr, opts...)
}

func New(ctx context.Context, name string, addr string, opts ...*Option) (c *Client, err error) {
	opt := Options().
		SetWeight(ut.GetEnvInt("CRPC_WEIGHT", 10)).
		SetBroadcastChanCap(ut.GetEnvInt("CRPC_BROADCAST_CAP", 64)).
		SetSecret(ut.GetEnv("CRPC_SECRET", "8620506fd4781174ec05fcacf816a12e")).
		SetVerifyJwtExpire(ut.GetEnvDuration("CRPC_JWT_EXPIRE", 5*time.Second)).
		SetDebug(ut.GetEnvBool("CRPC_DEBUG", false)).
		SetTimeout(10 * time.Second).
		SetMetaCoderT(coder.JSON).
		SetReqCoderT(coder.JSON).
		SetResCoderT(coder.JSON).
		SetCompressT(compressor.Raw).
		Merge(opts...)

	if name == "" {
		return nil, errors.New(errors.ClientInvalidArgs, "service name is required")
	}

	if addr == "" {
		return nil, errors.New(errors.ClientInvalidArgs, "address is required")
	}

	if opt.Secret == nil {
		return nil, errors.New(errors.ClientInvalidArgs, "secret is required")
	}

	c = &Client{
		Name: name,
		opt:  &opt,
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

func (c *Client) _go(ctx *Context, ht headertype.T) (call *Call) {
	call = NewCall()
	defer func() {
		if call.Error != nil {
			call.ctx = nil //回退ctx的控制权，因为上级executeChain，还要使用，再上级释放mCtx
		}
	}()
	ctx.Call = call
	call.ctx = ctx
	if ctx == nil {
		call.Error = errors.New(errors.ClientInvalidArgs, "context is required")
		call.done()
		return
	}
	service := ctx.Service
	method := ctx.Method
	if service == "" {
		call.Error = errors.New(errors.ClientInvalidArgs, "service name is required")
		call.done()
		return
	}
	module, method, err := ut.ParseModuleFunc(method)
	if err != nil {
		call.Error = errors.New(errors.ClientInternal, err.Error())
		call.done()
		return
	}
	opt := c.opt.Merge(ctx.Opts...)

	traceID := ""
	if tid := opt.TraceID; tid != nil {
		traceID = *tid
		ctx.Ctx = trace.WithTraceID(ctx.Ctx, traceID)
	}
	seq := atomic.AddUint64(&c.seq, 1)
	h := header.Get()
	h.SetType(ht).
		SetMetaCoderT(*opt.MetaCoderT).
		SetReqCoderT(*opt.ReqCoderT).
		SetResCoderT(*opt.ResCoderT).
		SetCompressT(*opt.CompressT).
		SetToService(service).
		SetModule(module).
		SetMethod(method).
		SetTraceID(traceID).
		SetSeq(seq)

	if s := opt.HashKey; s != nil {
		h.SetHashKey(*s)
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
	} else {
		h.Deadline = 0
	}

	call.cleanup = func() {
		if cancelFn != nil {
			cancelFn()
		}
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
		subCtx, subCancel := context.WithCancel(context.Background())
		call.subCtx = subCtx
		call.subCancel = subCancel

		// 2. 启动独立的消费协程
		// 将 ctx 传入，以便在请求超时/取消时退出循环
		go c.processBroadcastLoop(subCtx, call)
	}

	if *opt.Debug {
		h.Flags.With(headerflags.Debug)
	}

	// 检查本地是否有该服务
	_, hasLocalModule := c.serviceMap.Load(module)
	isUnicastLocal := (c.Name == service) && hasLocalModule && (ht == headertype.Req) && !h.Flags.IsBroadcast()
	var bodyBytes []byte
	var metaBytes []byte

	// 准备本地调用需要的对象
	var bodyObj any = ctx.Args
	var metaObj any = nil
	if opt.Meta != nil {
		metaObj = opt.Meta
	}

	needNetwork := !isUnicastLocal || h.Flags.IsBroadcast()

	if needNetwork {
		var err error
		bodyBytes, err = coder.Marshal(h.ReqCoderT, bodyObj)
		if err != nil {
			h.Release()
			call.Error = errors.New(errors.ClientInternal, err.Error())
			call.done()
			return
		}

		if opt.Meta != nil {
			if b, err := coder.Marshal(h.MetaCoderT, opt.Meta); err != nil {
				h.Release()
				call.Error = errors.New(errors.ClientInternal, err.Error())
				call.done()
				return
			} else {
				metaBytes = b
			}
		}
	}
	call.seq = seq
	call.Reply = ctx.Reply
	metaT := *opt.MetaCoderT
	reqT := *opt.ReqCoderT
	resT := *opt.ResCoderT

	// 闭包：执行本地逻辑
	runLocal := func(tmpCtx context.Context) {
		// 调用本地服务
		// 注意：invoke_local_func 内部还是使用了反射调用生成的 HandleMsg，
		// 如果追求极致性能，HandleMsg 应该接受 any 类型的 args，目前架构下为了兼容仍传 bytes。
		// 但返回值 res 是 interface{}，不需要反序列化。
		res, err := c.invoke_local_func(tmpCtx, module, method, metaT, reqT, metaObj, bodyObj)

		if h.Flags.IsBroadcast() {
			// 广播模式：构造结果推入 channel
			resObj := &broadcastResult{
				// rawBody: nil, // 本地调用不需要 rawBody
				res:       res, // 直接存结果对象,注意这里可能是任意值,指针或者值,指针的辅佐用
				resCoderT: resT,
				code:      headercode.OK,
				IsEOS:     false, // 本地肯定不是 EOS，EOS 由 Server 发
			}
			if err != nil {
				resObj.code = headercode.Failed
				// 如果出错，这里简单处理，将错误包装。
				// 为了兼容 broadcastResult 结构，这里可能需要序列化错误，
				// 或者在 processBroadcastLoop 里处理 decodedBody 为 error 的情况。
				// 现阶段简单做法：序列化 Error 放入 rawBody，模拟远程错误回包
				var rpcErr *errors.Error
				if e, ok := err.(*errors.Error); ok {
					rpcErr = e
				} else {
					rpcErr = errors.New(errors.RemoteInternal, err.Error())
				}
				if b, e := coder.Marshal(coder.Msgp, rpcErr); e == nil {
					resObj.rawBody = b
				}
				resObj.res = nil // 出错了就不传对象了
			}

			select {
			case call.broadcastCh <- resObj:
			case <-call.subCtx.Done():
			}
		} else {
			// 单播模式：直接填充 Reply
			if err != nil {
				call.Error = err
			} else if call.Reply != nil && res != nil {
				// 利用反射直接赋值，跳过 Unmarshal
				// 假设 call.Reply 是指针，res 是值或指针
				destVal := reflect.ValueOf(call.Reply)
				if destVal.Kind() == reflect.Pointer && !destVal.IsNil() {
					srcVal := reflect.ValueOf(res)
					// 处理指针解引用
					if srcVal.Kind() == reflect.Pointer {
						srcVal = srcVal.Elem()
					}
					// 确保目标也是解引用后的值
					destElem := destVal.Elem()
					if destElem.CanSet() {
						destElem.Set(srcVal)
					} else {
						call.Error = errors.New(errors.ClientInternal, "cannot set reply value")
					}
				}
			}
			call.done()
		}
	}

	// 场景 1: 纯本地单播 -> 只跑本地，不发包
	if isUnicastLocal {
		h.Release() // 不需要发包了，释放头
		go runLocal(ctx.Ctx)
		return call
	}

	// 场景 2: 广播且本地也有 -> 跑本地 + 发包(排除自己)
	if h.Flags.IsBroadcast() && hasLocalModule {
		h.Flags.Add(headerflags.ExcludeSender) // [关键] 标记告诉 Server 不要发给我
		go runLocal(ctx.Ctx)
		// 继续往下走，发送网络包给其他人
	}

	packet, err := protocol.Pack(h, metaBytes, bodyBytes)
	h.Release()
	if err != nil {
		call.Error = errors.New(errors.ClientInternal, err.Error())
		call.done()
		return
	}

	if ht != headertype.Send {
		c.pending.Store(seq, call)
	}

	if err := c.client.Sends(ctx.Ctx, packet, &opt.Option); err != nil {
		if ht != headertype.Send {
			c.pending.Delete(seq)
		}
		call.Error = errors.New(errors.ClientInternal, err.Error())
		call.done()
		return
	}

	if ht != headertype.Send {
		stop := context.AfterFunc(ctx.Ctx, func() {
			// 原子性抢占：如果能删掉，说明是超时导致的结束
			if _, loaded := c.pending.LoadAndDelete(seq); loaded {
				call.Error = ctx.Ctx.Err()
				if call.Error == nil {
					call.Error = context.DeadlineExceeded
				}
				call.done()
			}
		})

		// ------------------------------------------------------------------
		// 当请求正常返回时 (handleRes -> call.done)，主动停止 Timer 并释放 Context 资源
		call.cleanup = func() {
			stop() // 停止 AfterFunc 的监听，省资源
			if cancelFn != nil {
				cancelFn() // 立即释放 WithDeadline 创建的 Timer
			}
		}
	}
	return call
}

// 广播消费循环
func (c *Client) dispatchBroadcast(call *Call, res *broadcastResult) bool {
	if call.BroadcastResNewFunc == nil || call.BroadcastResCallBack == nil {
		return false
	}

	var reply any
	var resErr error
	if res.res != nil { // 本地调用优化，直接有对象
		reply = res.res
	} else if res.code.IsOK() {
		reply = call.BroadcastResNewFunc()
		if err := coder.Unmarshal(res.resCoderT, res.rawBody, reply); err != nil {
			tmpErr := errors.New(errors.ClientInternal, "unmarshal error: "+err.Error())
			if call.ctx != nil {
				call.ctx.invokeHooks(nil, tmpErr)
			}
			call.BroadcastResCallBack(nil, tmpErr, res.IsEOS)
			return false
		}
	} else {
		tmpErr := &errors.Error{}
		if err := coder.Unmarshal(coder.Msgp, res.rawBody, tmpErr); err != nil {
			tmpErr := errors.New(errors.ClientInternal, "unmarshal error: "+err.Error())
			if call.ctx != nil {
				call.ctx.invokeHooks(nil, tmpErr)
			}
			call.BroadcastResCallBack(nil, tmpErr, res.IsEOS)
			return false
		}
		if tmpErr.Code != errors.None {
			resErr = tmpErr
		}
	}

	if call.ctx != nil {
		call.ctx.invokeHooks(reply, resErr)
	}
	// 执行用户回调
	cont := call.BroadcastResCallBack(reply, resErr, res.IsEOS)
	// 如果是 EOS，框架层强制停止，不管用户返回什么
	if res.IsEOS {
		return false
	}
	return cont
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
					// 处理残留消息,也就是最后一条消息无法确定select选中上面，还是下面
					if !c.dispatchBroadcast(call, res) {
						return
					}
				default:
					// 通道空了，跳出排空循环，去报超时
					goto TIMEOUT_EXIT
				}
			}
		TIMEOUT_EXIT:
			if call.normalStop.Load() {
				if f := call.BroadcastResCallBack; f != nil {
					f(nil, nil, true)
				}
				return
			}
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
			if !c.dispatchBroadcast(call, res) {
				return
			}
		}
	}
}

func (c *Client) Use(middleware ...HandlerFunc) {
	c.handlers = append(c.handlers, middleware...)
}

// finalMiddleware 构造最后一环：调用底层的 _go 进行网络发送
func (c *Client) finalMiddleware(callType headertype.T) HandlerFunc {
	return func(ctx *Context) {
		// 使用 ctx.Ctx (可能被中间件修改过)
		c._go(ctx, callType)

		// 双向绑定：让 Context 和 Call 互相引用
		// 1. Context 持有 Call，以便中间件后续可以访问 Call 的信息
		// ctx.Call = call

		// 2. Call 持有 Context，以便 Call 结束时触发回调并释放 Context
		// call.ctx = ctx
		// : 确保绑定完成后，再启动广播处理循环
		// 这样 processBroadcastLoop 内部就能安全使用 call.ctx 了
		// if call.broadcastCh != nil {
		// 	go c.processBroadcastLoop(call.subCtx, call)
		// }
	}
}

// executeChain 统一执行流
func (c *Client) executeChain(ctx context.Context, callType headertype.T, serviceName, method string, args, reply any, opts ...*Option) *Call {
	// 1. 从 Pool 获取 Context

	mCtx := &Context{
		Ctx:     ctx,
		Service: serviceName,
		Method:  method,
		Args:    args,
		Reply:   reply,
		Opts:    opts,
		index:   -1,
		// 预分配切片 (可选优化)
		handlers: make(HandlersChain, 0, len(c.handlers)+1),
		hooks:    make([]responseHook, 0, 4),
	}

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
		if err := mCtx.Err(); err != nil {
			if _, ok := err.(*errors.Error); ok {
				dummyCall.Error = err
			} else {
				dummyCall.Error = errors.New(errors.ClientInternal, err.Error())
			}
		} else {
			// 如果用户 Abort 了但没设置 Err，给一个默认错误
			dummyCall.Error = errors.New(errors.ClientCanceled, "request aborted by middleware")
		}
		dummyCall.ctx = mCtx
		mCtx.Call = dummyCall
		dummyCall.done() // 立即结束
		return dummyCall
	}

	// 情况 B: _go 内部报错 (Critical Fix)
	// Call 对象存在，但已有 Error，说明 _go 内部已经调用过 done() 了。
	// 此时 mCtx 还没来得及被 done() 释放，必须在这里手动释放！
	if mCtx.Call.Error != nil {
		mCtx.invokeHooks(nil, mCtx.Call.Error) // 触发 hooks (如监控耗时)
		mCtx.Call.ctx = nil                    // 断开引用，防止野指针
		return mCtx.Call
	}
	// 5. 返回 Call 对象
	// 注意：Context 的释放权现在移交给了 Call (在 call.done() 中释放)
	return mCtx.Call
}
