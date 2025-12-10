package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
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
)

type server_mgr struct {
	opt              *Option
	services         sync.Map // map[string]*ServiceGroup (服务名 -> 服务组)
	connCache        sync.Map // map[uuid.UUID]*conn.Conn (临时存储连接)
	once             sync.Once
	broadcastCounter *broadcastCounterAll //tcpid -> seq -> *broadcastCounterItem (广播请求计数器)
}

func (s *server_mgr) Close() error {
	s.once.Do(func() {
		if s.broadcastCounter != nil {
			s.broadcastCounter.Stop()
			s.broadcastCounter = nil
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

	// 遍历所有服务组，移除该连接
	s.services.Range(func(key, value any) bool {
		group := value.(*ServiceGroup)
		group.Remove(sid.String())
		return true
	})

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
	defer h.Release()
	if h.Type.IsReq() {
		if h.Deadline > 0 {
			now := uint64(time.Now().UnixMicro())
			if now >= h.Deadline {
				// 可选：回包告知 Client 已超时（虽然 Client 可能已经不等了，但为了协议完整性建议回）
				s.replyError(sess, h, errors.New(errors.ServerDeadlineExceeded, "server-side timeout deadline exceeded"))
				return nil
			}
		}
		if h.Flags.IsHandshake() {
			return s.handleVerify(sess, h, body)
		}
	}

	go func() {
		copy_h := *h
		copy_meta := make([]byte, len(meta))
		copy(copy_meta, meta)
		copy_body := make([]byte, len(body))
		copy(copy_body, body)
		if err := s.route(sess, &copy_h, copy_meta, copy_body); err != nil {
			logger.Errorf("route error: %v", err)
		}
	}()
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
	val, ok := s.services.Load(h.ToService)
	if !ok {
		return s.replyError(sess, h, errors.Newf(errors.ServerServiceNotFound, "service %s not found", h.ToService))
	}
	group := val.(*ServiceGroup)
	timeout := *s.opt.SendTimeout
	if deadline := h.Deadline; deadline != 0 {
		if t := time.Until(time.UnixMicro(int64(deadline))); t <= 0 {
			return s.replyError(sess, h, errors.New(errors.ServerDeadlineExceeded, "broadcast request deadline exceeded"))
		} else {
			timeout = t
		}
	}
	// 2. 处理广播请求
	if h.Flags.IsBroadcast() {
		targets := group.GetAll()
		if len(targets) == 0 {
			// 广播如果没人在线，通常不需要报错，或者报 warning
			return nil
		}
		count := int32(len(targets))
		s.setBroadcastCount(sess.ID(), h.Seq, count, timeout)

		packet, err := protocol.Pack(h, meta, body)
		if err != nil {
			return err
		}
		for _, target := range targets {
			go target.Sends(context.Background(), packet, server.Options().WithConn(func(o *conn.Option) {
				o.SetWriteTimeout(timeout)
			}))
		}
		return nil
	}

	// 3. 普通请求负载均衡
	// Header 中没有 ShardingKey 或 TargetSid 字段，因此只能进行默认负载均衡
	target := group.Select()
	if target == nil {
		return s.replyError(sess, h, errors.New(errors.ServerDeadlineExceeded, "no available service instance"))
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
		if remain := s.decreaseBroadcastCount(tosid, h.Seq); remain <= 0 {
			h.Flags.Add(headerflags.EOS)
		}
		// 注意：如果重启了 Server 或者超时清理了 Map，
		// 可能会导致 EOS 丢失，Client 会依赖超时机制兜底。
	}
	return s.forward(targetSess, h, meta, body, *s.opt.SendTimeout)
}

// handleVerify 处理服务注册鉴权
func (s *server_mgr) handleVerify(sess server.Session, reqH *header.Header, body []byte) error {
	secret := *s.opt.Secret

	var claim protocol.JwtClaims
	token, err := jwt.ParseWithClaims(string(body), &claim, func(token *jwt.Token) (any, error) {
		return []byte(secret), nil
	})

	if err != nil || !token.Valid {
		return s.replyVerify(sess, reqH, nil, errors.Newf(errors.ServerInternal, "jwt verify failed: %v", err))
	}

	var req protocol.VerifyReq
	if err := coder.Unmarshal(coder.Msgp, claim.Data, &req); err != nil {
		return s.replyVerify(sess, reqH, nil, errors.Newf(errors.ServerInternal, "unmarshal verify req failed: %v", err))
	}

	// 3. 注册服务
	// 获取或创建 ServiceGroup
	val, _ := s.services.LoadOrStore(req.Name, NewServiceGroup(req.Name, *s.opt.GroupReplicas))
	group := val.(*ServiceGroup)

	group.Add(&Session{
		Name:    req.Name,
		Weight:  req.Weight,
		Session: sess,
	})

	logger.Infof("Service Registered: %s [Sid:%s, Weight:%d]", req.Name, sess.ID(), req.Weight)

	// 4. 回复鉴权成功
	return s.replyVerify(sess, reqH, &req, nil)
}

// replyVerify 回复鉴权结果
func (s *server_mgr) replyVerify(sess server.Session, reqH *header.Header, req *protocol.VerifyReq, verifyErr error) error {
	resp := &protocol.VerifyRes{
		Message: "OK",
	}
	reqH.Code = headercode.OK

	if verifyErr != nil {
		resp.Message = verifyErr.Error()
		reqH.Code = headercode.Failed
	} else {
		if req.UUID != uuid.Nil {
			// 设置 Session ID 为客户端指定的 UUID ,这里是重连上来的
			sess.SetID(req.UUID)
		}
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

func (s *server_mgr) replyError(srcSess server.Session, h *header.Header, rpcErr *errors.Error) error {
	if h.Type == headertype.Req {
		h.Type = headertype.Res
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
