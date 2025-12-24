package main

import (
	"context"
	"sync/atomic"
)

//go:generate gencrpcserverv3

// OrderReq 订单请求
type OrderReq struct {
	OrderID      string   `msg:"oid"`
	UserID       string   `msg:"uid"`
	ProductIDs   []string `msg:"pids"`
	Amount       int64    `msg:"amt"`
	Status       string   `msg:"status"`
	PaymentType  string   `msg:"pay"`
	ReceiverInfo string   `msg:"recv"`
	Timestamp    int64    `msg:"ts"`
	Metadata     []string `msg:"meta"`
}

// OrderRes 订单响应
type OrderRes struct {
	OrderID     string `msg:"oid"`
	Status      string `msg:"status"`
	OrderAmount int64  `msg:"amt"`
	ProcessedAt int64  `msg:"pat"`
	Message     string `msg:"msg"`
}

// OrderService 订单处理服务
type OrderService struct {
	Name      string       `msg:"-"`
	CallCount atomic.Int64 `msg:"-"`
	Processed atomic.Int64 `msg:"-"`
}

// ProcessOrder 处理订单(Go 方法 - 异步不等待返回值)
// @crpc:CallType:Go
func (s *OrderService) ProcessOrder(ctx context.Context, req *OrderReq) (*OrderRes, error) {
	s.CallCount.Add(1)
	s.Processed.Add(1)

	// 模拟订单处理逻辑
	if req.Status != "PENDING" {
		return &OrderRes{
			OrderID:     req.OrderID,
			Status:      "REJECTED",
			OrderAmount: req.Amount,
			ProcessedAt: req.Timestamp,
			Message:     "Invalid order status",
		}, nil
	}

	return &OrderRes{
		OrderID:     req.OrderID,
		Status:      "CONFIRMED",
		OrderAmount: req.Amount,
		ProcessedAt: req.Timestamp,
		Message:     "Order processed successfully",
	}, nil
}
