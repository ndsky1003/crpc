package main

import (
	"context"
	"log/slog"
	"runtime"

	"github.com/ndsky1003/crpc/v3"
	"github.com/ndsky1003/crpc/v3/server"
	"github.com/ndsky1003/log"
)

func main() {
	log.SetDefault(log.Options().SetAddSource(true).SetLevel(log.LevelInfo))

	slog.Info("=== Go 方法压力测试 - Server 启动 ===")

	// 使用多核
	numCPU := runtime.NumCPU()
	runtime.GOMAXPROCS(numCPU)
	slog.Info("CPU 核心数", "cores", numCPU)

	ctx := context.Background()

	// 创建 Server
	s, err := crpc.NewServer(ctx, server.Options().SetSecret("go_stress_secret_123456"))
	if err != nil {
		slog.Error("Failed to create server", "error", err)
		return
	}

	slog.Info("Server 已启动,监听端口 :9093")

	// 开始监听
	s.Listen(":9093")

	// 阻塞主程序
	select {}
}
