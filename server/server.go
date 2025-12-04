package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/ndsky1003/crpc/v3/protocol"
	"github.com/ndsky1003/net/conn"
	netServer "github.com/ndsky1003/net/server"
)

type CrpcServer struct {
	netServer *netServer.server // 注意: net包server结构体首字母小写的话这里需要调整，假设是公开的或通过接口引用
	// 实际上 netServer.New 返回的是 *server.server (小写)，外部只能通过接口或指针操作
	// 这里我们直接用 *netServer.server 只要在同一个项目或它导出了方法即可

	services  sync.Map // map[string]*ServiceGroup
	connCache sync.Map // map[string]*conn.Conn (临时存储连接，用于Reply)
}

func New(ctx context.Context, opts ...*netServer.Option) *CrpcServer {
	s := &CrpcServer{}
	// 这里假设 netServer.New 接受 service_manager 接口
	// 我们需要将 s 转换为 service_manager
	// s.netServer = netServer.New(ctx, s, opts...)
	// 由于 Go 的循环导入限制或私有类型，通常 New 返回的是 *server (struct)
	// 请确保 net 库的 New 返回类型有 Listen/Close 方法

	// 这里通过接口调用
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
	s.services.Range(func(key, value any) bool {
		group := value.(*ServiceGroup)
		group.Remove(sid)
		return true
	})
	return nil
}

// VerifyReq 握手包结构
type VerifyReq struct {
	ServiceName string
	Weight      int
}

func (s *CrpcServer) OnMessage(srcSid string, data []byte) error {
	h, meta, body, err := protocol.Unpack(data)
	if err != nil {
		return fmt.Errorf("unpack error: %v", err)
	}

	// 1. 处理握手 (需求 1 & 2)
	if h.Type == protocol.TypeVerify {
		var req VerifyReq
		if err := json.Unmarshal(body, &req); err != nil {
			return err
		}

		connVal, ok := s.connCache.Load(srcSid)
		if !ok {
			return fmt.Errorf("connection not found for sid: %s", srcSid)
		}

		val, _ := s.services.LoadOrStore(req.ServiceName, NewServiceGroup(req.ServiceName))
		group := val.(*ServiceGroup)
		group.Add(&Session{
			Sid:    srcSid,
			Weight: req.Weight,
			Conn:   connVal.(*conn.Conn),
		})

		log.Printf("Service Registered: %s [Sid:%s, Weight:%d]", req.ServiceName, srcSid, req.Weight)
		return nil
	}

	// 2. 路由
	return s.route(srcSid, h, meta, body)
}

func (s *CrpcServer) route(srcSid string, h *protocol.CrpcHeader, meta, body []byte) error {
	val, ok := s.services.Load(h.ServiceName)
	if !ok {
		return s.replyError(srcSid, h, fmt.Sprintf("service %s not found", h.ServiceName))
	}
	group := val.(*ServiceGroup)

	// 场景 A: 广播 (需求 4)
	if h.Type == protocol.TypeBroadcast {
		targets := group.GetAll()
		for _, sess := range targets {
			s.forward(sess.Conn, h, meta, body)
		}
		return nil
	}

	// 场景 B: 指定目标 (需求 4)
	if h.TargetSid != "" {
		target := group.GetBySid(h.TargetSid)
		if target != nil {
			s.forward(target.Conn, h, meta, body)
		} else {
			return s.replyError(srcSid, h, "target sid not found")
		}
		return nil
	}

	// 场景 C: 负载均衡 (需求 3)
	target := group.Select()
	if target != nil {
		s.forward(target.Conn, h, meta, body)
	} else {
		return s.replyError(srcSid, h, "no available service session")
	}
	return nil
}

func (s *CrpcServer) forward(c *conn.Conn, h *protocol.CrpcHeader, meta, body []byte) {
	// 转发时保持原Header，通常不需要修改
	packet, _ := protocol.Pack(h, meta, body)
	c.Send(context.Background(), packet)
}

func (s *CrpcServer) replyError(sid string, reqH *protocol.CrpcHeader, errMsg string) error {
	val, ok := s.connCache.Load(sid)
	if !ok {
		return nil
	}
	conn := val.(*conn.Conn)

	respH := &protocol.CrpcHeader{
		Seq:   reqH.Seq,
		Type:  protocol.TypeReply,
		Error: errMsg,
	}
	packet, _ := protocol.Pack(respH, nil, nil)
	return conn.Send(context.Background(), packet)
}
