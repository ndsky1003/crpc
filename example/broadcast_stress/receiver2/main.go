package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ndsky1003/crpc/v3"
	"github.com/ndsky1003/crpc/v3/client"
	"github.com/ndsky1003/crpc/v3/example/trace"
	"github.com/ndsky1003/log"
)

var orderSvc *OrderService

func main() {
	log.SetDefault(log.Options().SetExtractorAttr(func(ctx context.Context, r *slog.Record) {
		if tid := trace.ExtractorTraceID(ctx); tid != "" {
			r.Add("trace_id", tid)
		}
	}).SetAddSource(true).SetLevel(log.LevelInfo))

	slog.Info("=== Order Receiver2 启动 ===")

	ctx := context.Background()

	// 创建客户端
	cli, err := crpc.Dial(ctx, "broadcast_stress", ":9091",
		client.Options().SetSecret("test_secret_123456").
			SetWithTraceID(func(ctx context.Context, tid string) context.Context {
				return trace.WithTraceID(ctx, tid)
			}).SetGenTraceID(func(ctx context.Context) string {
			return trace.ExtractorTraceID(ctx)
		}))
	if err != nil {
		slog.Error("Failed to dial", "error", err)
		return
	}
	defer cli.Close()

	// 注册本地服务
	orderSvc = &OrderService{Name: "receiver2"}
	if err := cli.RegisterName("broadcast_stress", orderSvc); err != nil {
		slog.Error("Failed to register service", "error", err)
		return
	}

	slog.Info("Order Receiver2 started")

	// 定期报告统计
	go reportStats("Receiver2")

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Order Receiver2 shutting down...")
}

func reportStats(name string) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	lastCount := int64(0)

	for range ticker.C {
		current := orderSvc.CallCount.Load()
		processed := orderSvc.Processed.Load()
		delta := current - lastCount
		slog.Info("["+name+"] 2分钟统计",
			"总调用", current,
			"已处理", processed,
			"新增调用", delta,
			"速率(次/秒)", delta/120)
		lastCount = current
	}
}
