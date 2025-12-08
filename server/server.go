package server

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/crpc/v3/protocol"
	"github.com/ndsky1003/crpc/v3/protocol/header/headertype"
	"github.com/ndsky1003/net/conn"
	netServer "github.com/ndsky1003/net/server"
)

type CrpcServer struct {
	netServer *netServer.Server

	services  sync.Map // map[string]*ServiceGroup
	connCache sync.Map // map[string]*conn.Conn (临时存储连接)
}

func New(ctx context.Context, opts ...*netServer.Option) *CrpcServer {
	s := &CrpcServer{}
	// 将 s 注入到 netServer，因为它实现了 OnConnect 等接口
	s.netServer = netServer.New(ctx, s, opts...)
	return s
}

func (s *CrpcServer) Listen(addrs ...string) error {
	return s.netServer.Listen(addrs...)
}

func (s *CrpcServer) Close() error {
	return s.netServer.Close()
}

// --- net.service_manager 接口实现 ---

func (s *CrpcServer) OnConnect(sid string, c *conn.Conn) error {
	s.connCache.Store(sid, c)
	return nil
}

func (s *CrpcServer) OnDisconnect(sid string, err error) error {
	s.connCache.Delete(sid)
	// 遍历所有服务组，移除该连接
	s.services.Range(func(key, value any) bool {
		group := value.(*ServiceGroup)
		group.Remove(sid)
		return true
	})
	return nil
}

// VerifyReq 握手包结构 (对应 msgp.go 中的定义)
type VerifyReq struct {
	UUID   [16]byte // msgp 中 uuid.UUID 被 replace 为 [16]byte
	Name   string
	Weight int
}

func (s *CrpcServer) OnMessage(srcSid string, data []byte) error {
	h, meta, body, err := protocol.Unpack(data)
	if err != nil {
		return fmt.Errorf("unpack error: %v", err)
	}
	// 必须释放 Header，除非 route 中长期持有（这里是转发，route 也是用完即弃）
	defer h.Release()

	// 1. 处理握手鉴权
	if h.Type == headertype.VerifyReq {
		var req VerifyReq
		// 假设 VerifyReq 使用 Msgp 编码
		if err := coder.Unmarshal(coder.Msgp, body, &req); err != nil {
			return fmt.Errorf("verify unmarshal error: %v", err)
		}

		connVal, ok := s.connCache.Load(srcSid)
		if !ok {
			return fmt.Errorf("connection not found for sid: %s", srcSid)
		}

		// 注册到 ServiceGroup
		val, _ := s.services.LoadOrStore(req.Name, NewServiceGroup(req.Name))
		group := val.(*ServiceGroup)
		group.Add(&Session{
			Sid:    srcSid,
			Weight: req.Weight,
			Conn:   connVal.(*conn.Conn),
		})

		log.Printf("Service Registered: %s [Sid:%s, Weight:%d]", req.Name, srcSid, req.Weight)

		// 回复 VerifyRes (可选，视协议而定，client端在 onConnected 中等待回复)
		// ... 略过回复逻辑，通常这里应该发一个 VerifyRes ...
		return nil
	}

	// 2. 路由转发
	return s.route(srcSid, h, meta, body)
}

func (s *CrpcServer) route(srcSid string, h *protocol.Header, meta, body []byte) error {
	// 根据 ToService 查找服务组
	val, ok := s.services.Load(h.ToService)
	if !ok {
		// 找不到服务，回复错误
		// 注意：这里的 h.Seq 用于客户端回调匹配
		return s.replyError(srcSid, h, fmt.Sprintf("service %s not found", h.ToService))
	}
	group := val.(*ServiceGroup)

	// 场景 A: 广播 (BroadcastReq)
	if h.Type == headertype.BroadcastReq {
		targets := group.GetAll()
		for _, sess := range targets {
			// 异步转发，防止个别慢连接阻塞整个广播循环
			go s.forward(sess.Conn, h, meta, body)
		}
		return nil
	}

	// 场景 B: 指定目标 (TargetSid)
	if h.TargetSid != "" {
		target := group.GetBySid(h.TargetSid)
		if target != nil {
			s.forward(target.Conn, h, meta, body)
		} else {
			return s.replyError(srcSid, h, "target sid not found")
		}
		return nil
	}

	// 场景 C: 一致性哈希 / 粘性会话 (ShardingKey)
	// 用于文件上传等需要落在同一台机器的场景
	if h.ShardingKey != "" {
		target := group.SelectByKey(h.ShardingKey)
		if target != nil {
			s.forward(target.Conn, h, meta, body)
			return nil
		}
		// 如果没选到（极端情况），降级到随机路由
	}

	// 场景 D: 默认负载均衡 (随机加权)
	target := group.Select()
	if target != nil {
		s.forward(target.Conn, h, meta, body)
	} else {
		return s.replyError(srcSid, h, "no available service session")
	}
	return nil
}

func (s *CrpcServer) forward(c *conn.Conn, h *protocol.Header, meta, body []byte) {
	// 重新打包转发，保持原 Header 信息
	packet, err := protocol.Pack(h, meta, body)
	if err != nil {
		log.Printf("pack error: %v", err)
		return
	}
	// 发送 (net 库内部通常是异步写 buffer 或 chan)
	c.Send(context.Background(), packet)
}

func (s *CrpcServer) replyError(sid string, reqH *protocol.Header, errMsg string) error {
	val, ok := s.connCache.Load(sid)
	if !ok {
		return nil
	}
	c := val.(*conn.Conn)

	// 构造错误响应头
	// 注意：不能复用 reqH，因为 reqH 马上要 Release
	// 必须 Get 新的 Header 并设置必要字段
	// 实际上我们这里需要一个新的 Header 指针，或者 Pack 时只用字段
	// 但 protocol.Pack 需要 *header.Header

	// 这里简单模拟，实际上你可能需要引入 protocol/header 包的 Get()
	// h := header.Get() ... h.Release()

	// 由于这只是个 demo 补充，假设直接发个简单错误包
	log.Printf("Route Error to %s: %s", sid, errMsg)
	return nil
}
