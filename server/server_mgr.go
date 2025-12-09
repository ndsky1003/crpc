package server

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
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
	"github.com/ndsky1003/net/logger"
	"github.com/ndsky1003/net/server"
)

type server_mgr struct {
	opt       *Option
	services  sync.Map // map[string]*ServiceGroup (服务名 -> 服务组)
	connCache sync.Map // map[uuid.UUID]*conn.Conn (临时存储连接)

	// [新增] 广播计数器
	// Key: string (格式 "ClientID:Seq")
	// Value: *int32 (剩余等待的响应数)
	broadcastCounter sync.Map
}

type broadcastCounterItem struct {
	key   string
	count atomic.Int32
	timer *time.Timer
}

func (s *server_mgr) Close() error {
	return nil
}

// --- net.service_manager 接口实现 ---

func (s *server_mgr) OnConnect(sess server.Session) error {
	// 暂时只存入缓存，等待后续的 VerifyReq 消息进行服务注册
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

	s.broadcastCounter.Range(func(key, value any) bool {
		k := key.(string)
		// key 格式 "ClientID:Seq"
		if len(k) > len(sid.String()) && k[:len(sid.String())] == sid.String() {
			if item, ok := value.(*broadcastCounterItem); ok && item.timer != nil {
				item.timer.Stop() // 显式停止定时器
			}
			s.broadcastCounter.Delete(k)
		}
		return true
	})

	logger.Infof("Session disconnected: %s, err: %v", sid, err)
	return nil
}

func (s *server_mgr) OnMessage(sess server.Session, data []byte) error {

	// 1. 解包
	h, meta, body, err := protocol.Unpack(data)
	if err != nil {
		return fmt.Errorf("unpack error: %v", err)
	}
	defer h.Release()

	if h.Type.IsReq() {
		if h.Deadline > 0 {
			now := uint64(time.Now().UnixMicro())
			if now >= h.Deadline {
				// 超时了！记录日志，直接丢弃，或者回一个 Timeout 错误包
				logger.Warnf("Request %d dropped due to timeout. Deadline: %d, Now: %d", h.Seq, h.Deadline, now)

				// 可选：回包告知 Client 已超时（虽然 Client 可能已经不等了，但为了协议完整性建议回）
				s.replyError(sess, h, headercode.FailedRequestTimeout, "server-side timeout deadline exceeded")
				return nil
			}
		}
	}

	// 2. 处理鉴权请求 (VerifyReq)
	if h.Type == headertype.VerifyReq {
		return s.handleVerify(sess, h, body)
	}

	// 3. 处理 RPC 路由转发 (Req / BroadcastReq / Send)
	return s.route(sess, h, meta, body)
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
	code := headercode.OK

	if verifyErr != nil {
		resp.Message = verifyErr.Error()
		code = headercode.Failed
	} else {
		if req.UUID != uuid.Nil {
			// 设置 Session ID 为客户端指定的 UUID
			sess.SetID(req.UUID)
		}
		resp.UUID = sess.ID()
	}

	// 1. 序列化 VerifyRes
	body, err := coder.Marshal(coder.Msgp, resp)
	if err != nil {
		return err
	}

	// 2. 封装 JWT (Client 需要验证 server_mgr 的回复)
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

	// 3. 构建响应 Header
	// 必须使用 VerifyRes 类型，Client 端的 onConnected 正在 Read() 等待
	h := header.Get()
	h.Type = headertype.VerifyRes
	h.Code = code
	h.Seq = reqH.Seq // 保持 Seq 一致

	// 4. 发送
	packet, err := protocol.Pack(h, nil, []byte(tokenString))
	h.Release()
	if err != nil {
		return err
	}

	return sess.Sends(context.Background(), packet)
}

// route 核心路由逻辑
func (s *server_mgr) route(srcSess server.Session, h *header.Header, meta, body []byte) error {
	// 1. 查找目标服务组
	if h.Type == headertype.None {
		return s.replyError(srcSess, h, headercode.FailedServerPanic, "invalid header type: None")
	}

	if h.Type.IsReq() {
		val, ok := s.services.Load(h.ToService)
		if !ok {
			return s.replyError(srcSess, h, headercode.FailedServiceNotFound, fmt.Sprintf("service %s not found", h.ToService))
		}
		group := val.(*ServiceGroup)
		// 2. 处理广播请求
		if h.Type == headertype.BroadcastReq {
			targets := group.GetAll()
			if len(targets) == 0 {
				// 广播如果没人在线，通常不需要报错，或者报 warning
				return nil
			}
			// 重新打包一次 (header 可能会变，比如 TTL 或其他，这里保持原样)
			// 注意：Header 中的 Length 字段在 Pack 时会重新计算
			copy_meta := make([]byte, len(meta))
			copy(copy_meta, meta)
			copy_body := make([]byte, len(body))
			copy(copy_body, body)
			packet, err := protocol.Pack(h, copy_meta, copy_body)
			if err != nil {
				return err
			}

			timeout := 30 * time.Second //default timeout
			deadline := h.Deadline
			if deadline != 0 {
				if t := time.Until(time.UnixMicro(int64(deadline))); t <= 0 {
					return s.replyError(srcSess, h, headercode.FailedRequestTimeout, "broadcast request deadline exceeded")
				} else {
					timeout = t
				}
			}

			// [新增] 初始化计数器
			// 记录来源 ClientID 和 Seq，初始值为目标节点数量
			count := int32(len(targets))
			key := getBroadcastKey(srcSess.ID().String(), h.Seq)
			item := &broadcastCounterItem{
				key: key,
				timer: time.AfterFunc(timeout, func() {
					// 超时清理，防止内存泄漏
					s.broadcastCounter.Delete(key)
				}),
			}
			item.count.Store(count)
			s.broadcastCounter.Store(key, item)

			for _, target := range targets {
				// 异步发送，防止阻塞广播循环
				// 注意：这里 packet 是共享内存，Send 内部如果是异步写入 buffer 是安全的
				// 但如果 target.Conn.Send 会修改 data，则需要 copy。根据 net/conn 代码，Send -> Sends -> msg struct，安全。
				go target.Sends(context.Background(), packet)
			}
			return nil
		}

		// 3. 普通请求负载均衡
		// Header 中没有 ShardingKey 或 TargetSid 字段，因此只能进行默认负载均衡
		target := group.Select()
		if target == nil {
			return s.replyError(srcSess, h, headercode.FailedServiceUnavailable, "no available service instance")
		}

		// 4. 转发
		return s.forward(target, h, meta, body)
	} else { //res
		tosid := h.UUID
		target, ok := s.connCache.Load(tosid)
		if !ok {
			// 如果客户端掉线，记得清理计数器防止内存泄漏
			if h.Type == headertype.BroadcastRes {
				key := getBroadcastKey(tosid.String(), h.Seq)
				s.broadcastCounter.Delete(key)
			}
			return fmt.Errorf("target session %s not found for response", tosid)
		}
		targetSess := target.(server.Session)
		// [新增] 广播响应的拦截处理
		if h.Type == headertype.BroadcastRes {
			key := getBroadcastKey(tosid.String(), h.Seq)
			if val, ok := s.broadcastCounter.Load(key); ok {
				item := val.(*broadcastCounterItem)
				// 原子递减
				remain := item.count.Add(-1)

				// 如果是最后一个响应，打上 EOS 标记
				if remain <= 0 {
					h.Flags.Add(headerflags.EOS)
					if item.timer != nil {
						item.timer.Stop()
					}
					s.broadcastCounter.Delete(key) // 任务完成，清理内存
				}
			}
			// 注意：如果重启了 Server 或者超时清理了 Map，
			// 可能会导致 EOS 丢失，Client 会依赖超时机制兜底。
		}
		return s.forward(targetSess, h, meta, body)

	}
}

func (s *server_mgr) forward(sess server.Session, h *header.Header, meta, body []byte) error {
	// 直接透传，协议头保持不变
	packet, err := protocol.Pack(h, meta, body)
	if err != nil {
		return err
	}
	return sess.Sends(context.Background(), packet)
}

func (s *server_mgr) replyError(srcSess server.Session, reqH *header.Header, code headercode.T, errMsg string) error {
	// 构造错误回包
	h := header.Get()
	// 根据请求类型设置响应类型
	if reqH.Type == headertype.Req {
		h.Type = headertype.Res
	} else if reqH.Type == headertype.BroadcastReq {
		h.Type = headertype.BroadcastRes
	} else {
		// Send 类型不需要回包
		h.Release()
		return nil
	}

	h.Code = code
	h.Seq = reqH.Seq
	h.ResCoderT = coder.Msgp // 错误信息默认用 Msgp
	defer h.Release()

	rpcErr := errors.New(errors.ServerInternal, errMsg)
	body, err := coder.Marshal(coder.Msgp, rpcErr)
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
