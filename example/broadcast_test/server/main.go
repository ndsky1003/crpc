package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ndsky1003/crpc/v3"
	"github.com/ndsky1003/crpc/v3/example/trace"
	"github.com/ndsky1003/crpc/v3/server"
	"github.com/ndsky1003/log"
)

func main() {
	log.SetDefault(log.Options().SetExtractorAttr(func(ctx context.Context, r *slog.Record) {
		if tid := trace.ExtractorTraceID(ctx); tid != "" {
			r.Add("trace_id", tid)
		}
	}).SetAddSource(true).SetLevel(log.LevelDebug))

	// 创建服务端
	srv, err := crpc.NewServer(context.Background(),
		server.Options().SetSecret("test_secret_123456"))
	if err != nil {
		slog.Error("Failed to create server", "error", err)
		return
	}

	// 监听地址
	addr := ":9090"
	err1 := srv.Listen(addr)
	slog.Info("Server started", "addr", addr)
	slog.Error("dd", "dd", err1)

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Server shutting down...")
	srv.Close()
}
