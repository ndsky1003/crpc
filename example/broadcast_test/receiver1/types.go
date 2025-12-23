package main

//go:generate msgp -tests=false

import (
	"context"
	"sync/atomic"
)

// BroadcastReq 广播请求
type BroadcastReq struct {
	Message string `msg:"m"`
	Counter int64  `msg:"c"`
}

// BroadcastRes 广播响应
type BroadcastRes struct {
	From    string `msg:"f"`
	Message string `msg:"m"`
	Counter int64  `msg:"c"`
}

// BroadcastService 广播测试服务
type BroadcastService struct {
	Name      string `msg:"name"`    // 用于标识不同的服务实例
	CallCount atomic.Int64 `msg:"-"` // 调用计数（不序列化）
}

// Broadcast 广播方法
func (s *BroadcastService) Broadcast(ctx context.Context, req *BroadcastReq) (*BroadcastRes, error) {
	s.CallCount.Add(1)
	return &BroadcastRes{
		From:    s.Name,
		Message: "echo from " + s.Name,
		Counter: req.Counter + 1,
	}, nil
}
