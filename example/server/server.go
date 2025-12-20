package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ndsky1003/crpc/v3/example/trace"
	"github.com/ndsky1003/crpc/v3/server"
	"github.com/ndsky1003/log"
)

func main() {
	fmt.Println("start")
	log.SetDefault(log.Options().SetExtractorAttr(func(ctx context.Context, r *slog.Record) {
		if tid := trace.ExtractorTraceID(ctx); tid != "" {
			r.Add("trace_id", tid)
		}
	}).SetAddSource(true))
	s, err := server.New(context.Background(), server.Options().SetSecret("ddddd"))
	if err != nil {
		slog.Error("e", "err", err)
		return
	}

	s.Listen(":8080")
	fmt.Println("dd")
}
