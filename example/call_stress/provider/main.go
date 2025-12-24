package main

import (
	"context"
	"log/slog"
	"runtime"

	"github.com/ndsky1003/crpc/v3"
	"github.com/ndsky1003/crpc/v3/client"
	"github.com/ndsky1003/log"
)

func main() {
	log.SetDefault(log.Options().SetAddSource(true).SetLevel(log.LevelInfo))

	slog.Info("=== Call 压力测试 - Provider 启动 ===")

	// 使用多核
	numCPU := runtime.NumCPU()
	runtime.GOMAXPROCS(numCPU)
	slog.Info("CPU 核心数", "cores", numCPU)

	ctx := context.Background()

	// 创建 Client 作为服务提供者
	cli, err := crpc.Dial(ctx, "call_stress_provider", ":9092",
		client.Options().SetSecret("call_stress_secret_123456"))
	if err != nil {
		slog.Error("Failed to dial", "error", err)
		return
	}
	defer cli.Close()

	// 注册服务
	orderService := &OrderService{Name: "order_service"}
	if err := cli.RegisterName("call_stress", orderService); err != nil {
		slog.Error("Failed to register service", "error", err)
		return
	}

	slog.Info("Provider 已连接到 Server 并注册服务")

	// 阻塞主程序
	select {}
}
