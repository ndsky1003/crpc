package main

import (
	"context"
	"log/slog"
	"net/http"
	"runtime"

	_ "net/http/pprof" // 自动注册 pprof handler

	"github.com/ndsky1003/crpc/v3"
	"github.com/ndsky1003/crpc/v3/server"
	"github.com/ndsky1003/log"
)

func main() {
	log.SetDefault(log.Options().SetAddSource(true).SetLevel(log.LevelInfo))

	slog.Info("=== Call 压力测试 - Server 启动 ===")

	// 启动 pprof HTTP 服务器(可选,用于性能分析)
	go func() {
		slog.Info("pprof HTTP server started", "addr", "localhost:6061")
		slog.Error("pprof server error", "err", http.ListenAndServe("localhost:6061", nil))
	}()

	// 使用多核
	numCPU := runtime.NumCPU()
	runtime.GOMAXPROCS(numCPU)
	slog.Info("CPU 核心数", "cores", numCPU)

	ctx := context.Background()

	// 创建 Server (仅用于转发)
	s, err := crpc.NewServer(ctx, server.Options().SetSecret("call_stress_secret_123456"))
	if err != nil {
		slog.Error("Failed to create server", "error", err)
		return
	}

	slog.Info("Server 已启动,监听端口 :9092")

	// 开始监听
	s.Listen(":9092")

	// 阻塞主程序
	select {}
}
