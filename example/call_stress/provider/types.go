package main

import (
	"context"
	"sync/atomic"
)

//go:generate gencrpcserverv3

// OrderReq 订单请求(真实业务场景 - 电商订单处理)
type OrderReq struct {
	OrderID      string   `msg:"oid"`      // 订单ID
	UserID       string   `msg:"uid"`      // 用户ID
	ProductIDs   []string `msg:"pids"`     // 商品ID列表(多个商品)
	Amount       int64    `msg:"amt"`      // 订单金额(分)
	Status       string   `msg:"status"`   // 订单状态: PENDING/PAID/SHIPPED
	PaymentType  string   `msg:"pay"`      // 支付方式: alipay/wechat/card
	ReceiverInfo string   `msg:"recv"`     // 收货信息JSON
	Timestamp    int64    `msg:"ts"`       // 下单时间戳(微秒)
	Metadata     []string `msg:"meta"`     // 扩展元数据(渠道、来源等)
}

// OrderRes 订单响应
type OrderRes struct {
	OrderID     string `msg:"oid"`    // 订单ID
	Status      string `msg:"status"` // 处理后状态
	OrderAmount int64  `msg:"amt"`    // 订单金额
	ProcessedAt int64  `msg:"pat"`    // 处理时间戳
	Message     string `msg:"msg"`    // 处理消息
}

// OrderService 订单处理服务
type OrderService struct {
	Name      string       `msg:"-"` // 不序列化
	CallCount atomic.Int64 `msg:"-"` // 调用计数
	Processed atomic.Int64 `msg:"-"` // 处理完成计数
}

// ProcessOrder 处理订单(真实业务逻辑)
// @crpc:CallType:Call,Send,Go
func (s *OrderService) ProcessOrder(ctx context.Context, req *OrderReq) (*OrderRes, error) {
	s.CallCount.Add(1)
	s.Processed.Add(1)

	// 模拟真实的订单处理逻辑
	// 1. 验证订单状态
	if req.Status != "PENDING" {
		return &OrderRes{
			OrderID:     req.OrderID,
			Status:      "REJECTED",
			OrderAmount: req.Amount,
			ProcessedAt: req.Timestamp,
			Message:     "Invalid order status",
		}, nil
	}

	// 2. 模拟库存检查、支付验证等业务逻辑
	// 这里简化处理,直接返回成功

	return &OrderRes{
		OrderID:     req.OrderID,
		Status:      "CONFIRMED",
		OrderAmount: req.Amount,
		ProcessedAt: req.Timestamp,
		Message:     "Order processed successfully",
	}, nil
}
