package server

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/ndsky1003/crpc/v3/buffer"
	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/crpc/v3/protocol"
	"github.com/ndsky1003/crpc/v3/protocol/errors"
	"github.com/ndsky1003/crpc/v3/protocol/header"
	"github.com/ndsky1003/crpc/v3/protocol/header/headercode"
	"github.com/ndsky1003/crpc/v3/protocol/header/headerflags"
	"github.com/ndsky1003/crpc/v3/protocol/header/headertype"
	"github.com/ndsky1003/net/conn"
	"github.com/ndsky1003/net/server"
	"github.com/panjf2000/ants/v2"
)

type server_mgr struct {
	opt              *Option
	services         sync.Map // map[string]*ServiceGroup (服务名 -> 服务组)
	connCache        sync.Map // map[uuid.UUID]*conn.Conn (临时存储连接)
	once             sync.Once
	broadcastCounter *broadcastCounterAll //tcpid -> seq -> *broadcastCounterItem (广播请求计数器)
	workPool         *ants.Pool

	sidGroupIndex sync.Map

	handlers HandlersChain
}

func (s *server_mgr) Use(middleware ...HandlerFunc) {
	s.handlers = append(s.handlers, middleware...)
}

func (s *server_mgr) Close() error {
	s.once.Do(func() {
		if s.broadcastCounter != nil {
			s.broadcastCounter.Stop()
			s.broadcastCounter = nil
		}
		s.sidGroupIndex.Clear()
		s.handlers = s.handlers[:0]
		if s.workPool != nil {
			s.workPool.Release()
		}
	})
	return nil
}

// --- net.service_manager 接口实现 ---

func (s *server_mgr) OnConnect(sess server.Session) error {
	s.connCache.Store(sess.ID(), sess)
	return nil
}

func (s *server_mgr) OnDisconnect(sess server.Session, err error) error {

	sid := sess.ID()
	s.connCache.Delete(sid)

	if groupNameVal, ok := s.sidGroupIndex.LoadAndDelete(sid); ok {
		groupName := groupNameVal.(string)
		if gVal, ok := s.services.Load(groupName); ok {
			gVal.(*ServiceGroup).Remove(sid.String())
		}
	}

	//不清理,有可能连接上来,因为重连上来id不会变
	// broadcastCounter
	slog.Error("Session disconnected", "sid", sid, "err", err)
	return nil
}

func (s *server_mgr) OnMessage(sess server.Session, data []byte) error {
	h, meta, body, err := protocol.Unpack(data)
	if err != nil {
		return fmt.Errorf("unpack error: %v", err)
	}
	if h.Type.IsReq() {
		if h.Deadline > 0 {
			now := uint64(time.Now().UnixMicro())
			if now >= h.Deadline {
				// 可选：回包告知 Client 已超时（虽然 Client 可能已经不等了，但为了协议完整性建议回）
				if h.Type == headertype.Req && h.Flags.IsBroadcast() { //send 不需要回
					h.Flags.With(headerflags.EOS)
				}
				defer h.Release()
				return s.replyError(sess, h, errors.New(errors.ServerDeadlineExceeded, "server-side timeout deadline exceeded"))
			}
		}
		if h.Flags.IsHandshake() {
			defer h.Release()
			if err := s.handleVerify(sess, body); err != nil {
				return s.replyVerify(sess, h, err)
			} else {
				return s.replyVerify(sess, h, nil)
			}
		}
	}

	copy_meta := buffer.Get()
	meta_l, err := copy_meta.Write(meta)
	if err != nil {
		h.Release()
		copy_meta.Release()
		return err
	}
	copy_body := buffer.Get()
	body_l, err := copy_body.Write(body)
	if err != nil {
		h.Release()
		copy_meta.Release()
		copy_body.Release()
		return err
	}
	task := func() {
		defer h.Release()
		defer copy_meta.Release()
		defer copy_body.Release()
		// 1. 获取 Context
		handlers := make(HandlersChain, 0, len(s.handlers)+1)
		if len(s.handlers) > 0 {
			handlers = append(handlers, s.handlers...)
		}
		ctx := &Context{
			Sess:      sess,
			Header:    h, //一个池化对象，放到了一个非池化的身上,现在的环境下，生命周期是一样的，所以不存在问题
			MetaBytes: copy_meta.Bytes()[:meta_l],
			BodyBytes: copy_body.Bytes()[:body_l],
			index:     -1,
			handlers:  handlers,
			// Keys map 不需要预分配，用到再分配，省内存
		}

		// 2. 初始化
		ctx.Sess = sess
		ctx.Header = h
		ctx.MetaBytes = copy_meta.Bytes()[:meta_l]
		ctx.BodyBytes = copy_body.Bytes()[:body_l]

		// 3. 构造最后一环 Handler
		finalHandler := func(c *Context) {
			// 调用原有的 route 逻辑
			err := s.route(c.Sess, c.Header, c.MetaBytes, c.BodyBytes)
			c.SetError(err)
		}

		ctx.handlers = append(ctx.handlers, finalHandler)

		// 5. 执行
		ctx.Next()

		// 6. 错误日志 (可选)
		if h.Type == headertype.Req {
			if err := ctx.Err(); err != nil {
				// 优先返回具体的错误信息 (token invalid, rate limit 等)
				// 不管是否 Abort，只要有 Err，就以 Err 为主
				slog.Warn("request process", "err", err)
				s.replyError(sess, h, err)
			} else if ctx.IsAborted() {
				// 处理 "Abort 但无 Err" 的死角
				// 必须回包，防止客户端超时
				s.replyError(sess, h, errors.New(errors.ServerStandardError, "request aborted by middleware"))
			}
		} else {
			// Send 类型 (OneWay)：仅记录日志，不回包
			if err := ctx.Err(); err != nil {
				slog.Warn("async send request process", "err", err)
			}
		}
	}
	if err = s.workPool.Submit(task); err != nil {
		h.Release()
		copy_meta.Release()
		copy_body.Release()
		if err == ants.ErrPoolOverload {
			return errors.New(errors.ServerInternal, "server busy")
		}
		return errors.New(errors.ServerInternal, err.Error())
	}
	return nil
}

func (s *server_mgr) route(sess server.Session, h *header.Header, meta, body []byte) error {
	if h.Type.IsReq() {
		return s.handleReq(sess, h, meta, body)
	} else {
		return s.handleRes(sess, h, meta, body)
	}
}

func (s *server_mgr) handleReq(sess server.Session, h *header.Header, meta, body []byte) error {
	h.UUID = sess.ID() //返回链路
	val, ok := s.services.Load(h.ToService)
	if !ok {
		if h.Type == headertype.Req && h.Flags.IsBroadcast() { //send 不需要回
			h.Flags.With(headerflags.EOS)
		}
		slog.Warn("not found for request", "service", h.ToService)
		return errors.Newf(errors.ServerServiceNotFound, "service %s not found", h.ToService)
	}
	group := val.(*ServiceGroup)
	timeout := *s.opt.SendTimeout
	if deadline := h.Deadline; deadline != 0 {
		if t := time.Until(time.UnixMicro(int64(deadline))); t <= 0 {
			if h.Type == headertype.Req && h.Flags.IsBroadcast() { //send 不需要回
				h.Flags.With(headerflags.EOS)
			}
			return errors.New(errors.ServerDeadlineExceeded, "broadcast request deadline exceeded")
		} else {
			timeout = t
		}
	}
	// 2. 处理广播请求
	if h.Flags.IsBroadcast() {
		allTargets := group.GetAll()
		sid := sess.ID()
		seq := h.Seq
		var realTargets []*Session
		excludeSelf := h.Flags.IsExcludeSender()

		if len(allTargets) > 0 {
			// 预分配切片以提高性能，最大长度为 allTargets
			realTargets = make([]*Session, 0, len(allTargets))
			for _, t := range allTargets {
				if excludeSelf && t.ID() == sid {
					continue
				}
				realTargets = append(realTargets, t)
			}
		}

		if len(realTargets) == 0 {
			if h.Type == headertype.Req { //send 不需要回
				h.Flags.With(headerflags.EOS)
			}
			// 如果是因为排除自己导致没有目标，应该返回 OK 而不是 Error，否则客户端会收到报错
			// 如果本来就没服务，才报 Unavailable
			if len(allTargets) > 0 && excludeSelf {
				// 这是一个特殊的成功：没有其他接收者，但本地已经处理了（Client端知道）
				// 或者我们回一个空的 EOS
				return errors.New(errors.None, "无可广播的对象")
			}
			return errors.New(errors.ServerServiceUnavailable, "无可广播的对象")
		}
		packet, err := protocol.Pack(h, meta, body)
		if err != nil {
			if h.Type == headertype.Req { //send 不需要回
				h.Flags.With(headerflags.EOS)
			}
			return err
		}
		count := int32(len(realTargets))
		callBack := func() {
			//TODO:这里应该补发一个EOS的回包
		}
		s.broadcastCounter.setBroadcastCount(sid, seq, count, timeout, callBack)

		for _, t := range realTargets {
			target := t
			copy_h := *h
			handleFailure := func(err error) {
				if remain := s.broadcastCounter.decreaseBroadcastCount(sid, seq); remain <= 0 {
					copy_h.Flags.With(headerflags.EOS)
				}
				if replyErr := s.replyError(sess, &copy_h, err); replyErr != nil {
					slog.Error("replyError", "err", replyErr)
				}
			}
			task := func() {
				if err := target.Sends(context.Background(), packet, server.Options().WithConn(func(o *conn.Option) {
					o.SetWriteTimeout(timeout)
				})); err != nil {
					handleFailure(err)
				}
			}
			if err = s.workPool.Submit(task); err != nil {
				if err == ants.ErrPoolOverload {
					err = errors.New(errors.ServerInternal, "server busy")
				} else {
					err = errors.New(errors.ServerInternal, err.Error())
				}
				handleFailure(err)
			}
		}
		return nil
	}

	var target *Session
	if key := h.HashKey; key != "" {
		target = group.SelectByKey(key)
	} else {
		target = group.Select()
	}

	if target == nil {
		return errors.New(errors.ServerDeadlineExceeded, "no available service instance")
	}
	return s.forward(target, h, meta, body, timeout)
}

func (s *server_mgr) handleRes(_ server.Session, h *header.Header, meta, body []byte) error {
	tosid := h.UUID
	target, ok := s.connCache.Load(tosid)
	if !ok {
		//有可能重连上来,所以不清理
		return fmt.Errorf("target session %s not found for response", tosid)
	}
	targetSess := target.(server.Session)
	if h.Flags.IsBroadcast() {
		if remain := s.broadcastCounter.decreaseBroadcastCount(tosid, h.Seq); remain <= 0 {
			h.Flags.Add(headerflags.EOS)
		}
		//NOTE:
		// 注意：如果重启了 Server 或者超时清理了 Map，
		// 可能会导致 EOS 丢失，Client 会依赖超时机制兜底。
	}
	return s.forward(targetSess, h, meta, body, *s.opt.SendTimeout)
}

// handleVerify 处理服务注册鉴权
func (s *server_mgr) handleVerify(sess server.Session, body []byte) error {
	secret := *s.opt.Secret

	var claim protocol.JwtClaims
	token, err := jwt.ParseWithClaims(string(body), &claim, func(token *jwt.Token) (any, error) {
		return []byte(secret), nil
	})

	if err != nil || !token.Valid {
		return errors.Newf(errors.ServerInternal, "jwt verify failed: %v", err)
	}

	var req protocol.VerifyReq
	if err := coder.Unmarshal(coder.Msgp, claim.Data, &req); err != nil {
		return errors.Newf(errors.ServerInternal, "unmarshal verify req failed: %v", err)
	}
	if req.UUID != uuid.Nil {
		sess.SetID(req.UUID)
	}

	if oldGroupVal, loaded := s.sidGroupIndex.Load(sess.ID()); loaded {
		oldGroupName := oldGroupVal.(string)
		if oldGroupName != req.Name { // 如果名字变了，说明切换了身份
			if gVal, ok := s.services.Load(oldGroupName); ok {
				gVal.(*ServiceGroup).Remove(sess.ID().String())
			}
		}
	}
	// 获取或创建 ServiceGroup
	val, _ := s.services.LoadOrStore(req.Name, NewServiceGroup(req.Name, *s.opt.GroupReplicas))
	group := val.(*ServiceGroup)

	group.Add(&Session{
		Name:    req.Name,
		Weight:  req.Weight,
		Session: sess,
	})

	s.sidGroupIndex.Store(sess.ID(), req.Name)

	slog.Info("Service Registered", "Name", req.Name, "sid", sess.ID(), "weight", req.Weight)

	return nil
}

// replyVerify 回复鉴权结果
func (s *server_mgr) replyVerify(sess server.Session, reqH *header.Header, verifyErr error) error {
	resp := &protocol.VerifyRes{
		Message: "OK",
	}
	reqH.Code = headercode.OK

	if verifyErr != nil {
		slog.Error("Authentication failed", "err", verifyErr, "sid", sess.ID())
		// 返回通用错误信息，避免泄露JWT验证的敏感信息
		resp.Message = "Authentication failed"
		reqH.Code = headercode.Failed
	} else {
		resp.UUID = sess.ID()
	}
	body, err := coder.Marshal(coder.Msgp, resp)
	if err != nil {
		return err
	}
	payload := protocol.JwtClaims{
		Data: body,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)), // 短期有效
		},
	}
	tokenString, err := jwt.NewWithClaims(jwt.SigningMethodHS256, payload).SignedString([]byte(*s.opt.Secret))
	if err != nil {
		return err
	}
	reqH.Type = headertype.Res
	packet, err := protocol.Pack(reqH, nil, []byte(tokenString))
	if err != nil {
		return err
	}
	return sess.Sends(context.Background(), packet)
}

func (s *server_mgr) forward(sess server.Session, h *header.Header, meta, body []byte, timeout time.Duration) error {
	// 直接透传，协议头保持不变
	packet, err := protocol.Pack(h, meta, body)
	if err != nil {
		return err
	}
	return sess.Sends(context.Background(), packet, server.Options().WithConn(func(o *conn.Option) {
		o.SetWriteTimeout(timeout)
	}))
}

func (s *server_mgr) replyError(srcSess server.Session, h *header.Header, rpcErr error) error {
	if h.Type == headertype.Req {
		h.Type = headertype.Res
	} else { //send
		slog.Warn("no need to reply error for send type", "sid", srcSess.ID(), "method", h.Method)
		return nil
	}
	h.Code = headercode.Failed
	h.ResCoderT = coder.Msgp // 错误信息默认用 Msgp
	var finalErr *errors.Error
	if e, ok := rpcErr.(*errors.Error); ok {
		finalErr = e
	} else {
		finalErr = errors.New(errors.ServerInternal, rpcErr.Error())
	}
	body, err := coder.Marshal(h.ResCoderT, finalErr)
	if err != nil {
		return err
	}
	// 3. 发送
	packet, err := protocol.Pack(h, nil, body)
	if err != nil {
		return err
	}
	return srcSess.Sends(context.Background(), packet)

}
