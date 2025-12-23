package main

//go:generate msgp -tests=false

import (
	"context"
	"sync/atomic"
)

// OrderReq 订单请求（真实业务场景 - 电商订单广播）
type OrderReq struct {
	OrderID      string   `msg:"oid"`      // 订单ID
	UserID       string   `msg:"uid"`       // 用户ID
	ProductIDs   []string `msg:"pids"`     // 商品ID列表
	Amount       int64    `msg:"amt"`       // 订单金额（分）
	Status       string   `msg:"status"`    // 订单状态
	PaymentType  string   `msg:"pay"`       // 支付方式
	ReceiverInfo string   `msg:"recv"`      // 收货信息JSON
	Timestamp    int64    `msg:"ts"`        // 时间戳(微秒)
	Metadata     []string `msg:"meta"`      // 扩展元数据
}

// OrderRes 订单响应
type OrderRes struct {
	OrderID     string `msg:"oid"`    // 订单ID
	From        string `msg:"f"`      // 处理节点
	Status      string `msg:"status"` // 处理后状态
	ProcessedAt int64  `msg:"pat"`    // 处理时间戳
	Message     string `msg:"msg"`    // 处理消息
}

// OrderService 订单处理服务
type OrderService struct {
	Name      string        `msg:"-"` // 不序列化
	CallCount atomic.Int64  `msg:"-"` // 调用计数
	Processed atomic.Int64  `msg:"-"` // 处理完成计数
}

// ProcessOrder 处理订单广播方法
func (s *OrderService) ProcessOrder(ctx context.Context, req *OrderReq) (*OrderRes, error) {
	s.CallCount.Add(1)
	s.Processed.Add(1)
	return &OrderRes{
		OrderID:     req.OrderID,
		From:        s.Name,
		Status:      "PROCESSED",
		ProcessedAt: req.Timestamp,
		Message:     "Order processed successfully at " + s.Name,
	}, nil
}
