package server

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ndsky1003/crpc/v3/buffer/netpool"
	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/crpc/v3/protocol"
	"github.com/ndsky1003/crpc/v3/protocol/errors"
	"github.com/ndsky1003/crpc/v3/protocol/header"
	"github.com/ndsky1003/crpc/v3/protocol/header/headercode"
	"github.com/ndsky1003/crpc/v3/protocol/header/headerflags"
	"github.com/ndsky1003/crpc/v3/protocol/header/headertype"
	"github.com/ndsky1003/net/v2/conn"
	"github.com/ndsky1003/net/v2/server"
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
	// s.connCache.Store(sess.ID(), sess)
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
	if h.Flags.IsHandshake() {
		defer netpool.Release(data)
		defer h.Release()
		if err := s.handleVerify(sess, body); err != nil {
			return s.replyVerify(sess, h, err)
		} else {
			return s.replyVerify(sess, h, nil)
		}
	}

	if h.Type.IsReq() {
		if h.Deadline > 0 {
			now := uint64(time.Now().UnixMicro())
			if now >= h.Deadline { //超时
				netpool.Release(data)
				defer h.Release()
				if h.Type == headertype.Req {
					if h.Flags.IsBroadcast() {
						h.Flags.With(headerflags.EOS)
					}
					return s.replyError(sess, h, errors.New(errors.ServerDeadlineExceeded, "server-side timeout deadline exceeded"))
				} else {
					slog.Warn("收到的消息已经超时直接丢弃", "header", h, "data", data)
					return nil
				}
			}
		}
	}

	task := func() {
		defer netpool.Release(data)
		defer h.Release()
		// 1. 获取 Context
		handlers := make(HandlersChain, 0, len(s.handlers)+1)
		if len(s.handlers) > 0 {
			handlers = append(handlers, s.handlers...)
		}
		ctx := &Context{
			Sess:     sess,
			Header:   h, //一个池化对象，放到了一个非池化的身上,现在的环境下，生命周期是一样的，所以不存在问题
			Data:     data,
			index:    -1,
			handlers: handlers,
			// Keys map 不需要预分配，用到再分配，省内存
		}

		finalHandler := func(c *Context) {
			// 调用原有的 route 逻辑
			err := s.route(c.Sess, c.Header, c.Data, meta, body)
			c.SetError(err)
		}

		ctx.handlers = append(ctx.handlers, finalHandler)

		// 5. 执行
		ctx.Next()

		switch h.Type {
		case headertype.Req:
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
		case headertype.Send:
			if err := ctx.Err(); err != nil {
				slog.Error("async send request process", "err", err)
			}
		case headertype.Res:
			if err := ctx.Err(); err != nil {
				slog.Error("res process", "err", err)
			}
		default:
			slog.Warn("unknow type", "type", h.Type)

		}
	}
	if err = s.workPool.Submit(task); err != nil {
		h.Release()
		netpool.Release(data)
		if err == ants.ErrPoolOverload {
			return errors.New(errors.ServerInternal, "server busy")
		}
		return errors.New(errors.ServerInternal, err.Error())
	}
	return nil
}

func (s *server_mgr) route(sess server.Session, h *header.Header, data, meta, body []byte) error {
	if h.Type.IsReq() {
		if h.Flags.IsDebug() {
			slog.Debug("handleReq", "header", h, "trace_id", h.TraceID)
		}
		return s.handleReq(sess, h, data)
	} else {
		if h.Flags.IsDebug() {
			slog.Debug("handleRes", "header", h, "trace_id", h.TraceID)
		}
		return s.handleRes(sess, h, data, meta, body)
	}
}

func (s *server_mgr) handleReq(sess server.Session, h *header.Header, data []byte) error {
	val, ok := s.services.Load(h.ToService)
	if !ok {
		if h.Flags.IsBroadcast() {
			h.Flags.With(headerflags.EOS)
		}
		slog.Warn("not found for request", "service", h.ToService)
		return errors.Newf(errors.ServerServiceNotFound, "service %s not found", h.ToService)
	}
	group := val.(*ServiceGroup)
	timeout := *s.opt.SendTimeout
	if deadline := h.Deadline; deadline != 0 {
		if t := time.Until(time.UnixMicro(int64(deadline))); t <= 0 {
			if h.Flags.IsBroadcast() {
				h.Flags.With(headerflags.EOS)
			}
			return errors.New(errors.ServerDeadlineExceeded, "request deadline exceeded")
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
				if excludeSelf && t.Session.ID() == sid {
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
		count := int32(len(realTargets))
		copy_h := h.Clone()
		callBack := func() {
			copy_h.Flags.With(headerflags.EOS)
			if replyErr := s.replyError(sess, copy_h, errors.New(errors.ServerDeadlineExceeded, "广播等待接收消息超时")); replyErr != nil {
				slog.Error("replyError", "err", replyErr)
			}
		}
		if copy_h.Type == headertype.Req {
			s.broadcastCounter.setBroadcastCount(sid, seq, count, timeout, callBack)
		}

		for _, t := range realTargets {
			target := t
			handleFailure := func(err error) {
				if copy_h.Type == headertype.Send {
					slog.Error("broad cast send", "err", err)
					return
				}
				if remain := s.broadcastCounter.decreaseBroadcastCount(sid, seq); remain <= 0 {
					copy_h.Flags.With(headerflags.EOS)
				}
				if replyErr := s.replyError(sess, copy_h, err); replyErr != nil {
					slog.Error("replyError", "err", replyErr)
				}
			}
			task := func() {
				if err := target.Send(context.Background(), data, server.Options().WithConn(func(o *conn.Option) {
					o.SetWriteTimeout(timeout)
				})); err != nil {
					handleFailure(err)
				}
			}
			if err := s.workPool.Submit(task); err != nil {
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
	var err error
	if key := h.HashKey; key != "" {
		target, err = group.SelectByKey(key)
	} else {
		target, err = group.Select()
	}
	if err != nil {
		slog.Error(group.Name, "err", err)
		return errors.New(errors.ServerInternal, "no available service instance")
	}
	if target == nil {
		return errors.New(errors.ServerDeadlineExceeded, "no available service instance")
	}
	return s.forward(target.Session, data, timeout)
}

func (s *server_mgr) handleRes(_ server.Session, h *header.Header, data, meta, body []byte) error {
	timeout := *s.opt.SendTimeout
	tosid := h.UUID
	target, ok := s.connCache.Load(tosid)
	if !ok {
		//有可能重连上来,所以不清理
		return fmt.Errorf("target session %s not found for response", tosid)
	}
	targetSess := target.(server.Session)
	var need_pack bool
	if h.Flags.IsBroadcast() {
		if remain := s.broadcastCounter.decreaseBroadcastCount(tosid, h.Seq); remain <= 0 {
			h.Flags.Add(headerflags.EOS)
			need_pack = true
		}
		//NOTE:
		// 注意：如果重启了 Server 或者超时清理了 Map，
		// 可能会导致 EOS 丢失，Client 会依赖超时机制兜底。
	}
	if need_pack {
		packet, err := protocol.Pack(h, meta, body)
		if err != nil {
			return err
		}
		return s.forwards(targetSess, packet, timeout)
	}
	return s.forward(targetSess, data, timeout)
}

func (s *server_mgr) forward(sess server.Session, data []byte, timeout time.Duration) error {
	return sess.Send(context.Background(), data, server.Options().WithConn(func(o *conn.Option) {
		o.SetWriteTimeout(timeout)
	}))
}

func (s *server_mgr) forwards(sess server.Session, data [][]byte, timeout time.Duration) error {
	return sess.Sends(context.Background(), data, server.Options().WithConn(func(o *conn.Option) {
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
