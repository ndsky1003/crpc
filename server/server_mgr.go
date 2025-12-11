package server

import (
	"context"
	"fmt"
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
	"github.com/ndsky1003/net/logger"
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
}

func (s *server_mgr) Close() error {
	s.once.Do(func() {
		if s.broadcastCounter != nil {
			s.broadcastCounter.Stop()
			s.broadcastCounter = nil
		}
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
	logger.Infof("Session disconnected: %s, err: %v", sid, err)
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
		if err := s.route(sess, h, copy_meta.Bytes()[:meta_l], copy_body.Bytes()[:body_l]); err != nil {
			logger.Errorf("route error: %v", err)
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
		err := s.handleReq(sess, h, meta, body)
		if err != nil {
			s.replyError(sess, h, err)
		}
		return err
	} else {
		return s.handleRes(sess, h, meta, body)
	}
}

func (s *server_mgr) handleReq(sess server.Session, h *header.Header, meta, body []byte) error {
	h.UUID = sess.ID() //返回链路
	val, ok := s.services.Load(h.ToService)
	if !ok {
		logger.Warnf("Service %s not found for request", h.ToService)
		return errors.Newf(errors.ServerServiceNotFound, "service %s not found", h.ToService)
	}
	group := val.(*ServiceGroup)
	timeout := *s.opt.SendTimeout
	if deadline := h.Deadline; deadline != 0 {
		if t := time.Until(time.UnixMicro(int64(deadline))); t <= 0 {
			return errors.New(errors.ServerDeadlineExceeded, "broadcast request deadline exceeded")
		} else {
			timeout = t
		}
	}
	// 2. 处理广播请求
	if h.Flags.IsBroadcast() {
		targets := group.GetAll()
		sid := sess.ID()
		seq := h.Seq
		if len(targets) == 0 {
			if h.Type == headertype.Req { //send 不需要回
				h.Flags.With(headerflags.EOS)
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
		count := int32(len(targets))
		s.broadcastCounter.setBroadcastCount(sid, seq, count, timeout)

		for _, t := range targets {
			target := t
			copy_h := *h
			handleFailure := func(err error) {
				if remain := s.broadcastCounter.decreaseBroadcastCount(sid, seq); remain <= 0 {
					copy_h.Flags.With(headerflags.EOS)
				}
				if replyErr := s.replyError(sess, &copy_h, err); replyErr != nil {
					logger.Error(replyErr)
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
	// [新增] 广播响应的拦截处理
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

	logger.Infof("Service Registered: %s [Sid:%s, Weight:%d]", req.Name, sess.ID(), req.Weight)

	return nil
}

// replyVerify 回复鉴权结果
func (s *server_mgr) replyVerify(sess server.Session, reqH *header.Header, verifyErr error) error {
	resp := &protocol.VerifyRes{
		Message: "OK",
	}
	reqH.Code = headercode.OK

	if verifyErr != nil {
		resp.Message = verifyErr.Error()
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
		logger.Warnf("no need to reply error for send type, sid: %s, method: %s", srcSess.ID(), h.Method)
		return nil
	}
	h.Code = headercode.Failed
	h.ResCoderT = coder.Msgp // 错误信息默认用 Msgp
	body, err := coder.Marshal(h.ResCoderT, rpcErr)
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
