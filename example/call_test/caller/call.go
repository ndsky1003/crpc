package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ndsky1003/crpc/v3/client"
	"github.com/ndsky1003/crpc/v3/example/comm"
	"github.com/ndsky1003/crpc/v3/example/dto"
	"github.com/ndsky1003/crpc/v3/example/trace"
	"github.com/ndsky1003/log"
)

func main() {
	log.SetDefault(log.Options().SetExtractorAttr(func(ctx context.Context, r *slog.Record) {
		if tid := trace.ExtractorTraceID(ctx); tid != "" {
			r.Add("trace_id", tid)
		}
	}).SetAddSource(true))

	// 初始化 Client5 - 压力测试客户端
	c, err := client.Dial(context.Background(), "client5", ":8080",
		client.Options().SetSecret("ddddd").
			SetWithTraceID(func(ctx context.Context, tid string) context.Context {
				return trace.WithTraceID(ctx, tid)
			}).SetGenTraceID(func(ctx context.Context) string {
			return trace.ExtractorTraceID(ctx)
		}))
	if err != nil {
		fmt.Println("dial error:", err)
		return
	}

	// 设置全局客户端变量
	comm.Default_Client = c
	time.Sleep(1e9)
	req := &dto.Req{Name: "req"}
	meta := &dto.Meta{Source: "client5"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	slog.Info("call ", "call", "dd")
	_, err = comm.Full(ctx, meta, req)
	slog.Error("err", "err", err)

	// 保持程序运行
	select {}
}
