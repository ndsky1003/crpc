package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ndsky1003/crpc/v3"
	"github.com/ndsky1003/crpc/v3/client"
	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/crpc/v3/example/trace"
	"github.com/ndsky1003/log"
)

// 广播统计
type BroadcastStats struct {
	TotalSent      atomic.Int64
	TotalReceived  atomic.Int64
	TotalErrors    atomic.Int64
	TotalEOS       atomic.Int64 // EOS 次数
	TotalLocal     atomic.Int64 // 本地响应数
	TotalReceiver1 atomic.Int64 // receiver1 响应数
	TotalReceiver2 atomic.Int64 // receiver2 响应数
	TotalUnknown   atomic.Int64 // 未知来源响应数
	LastSummary    time.Time
	mu             sync.Mutex
	missingSeqs    []int64 // 缺失的请求序号
}

var stats = &BroadcastStats{
	LastSummary: time.Now(),
}

func main() {
	log.SetDefault(log.Options().SetExtractorAttr(func(ctx context.Context, r *slog.Record) {
		if tid := trace.ExtractorTraceID(ctx); tid != "" {
			r.Add("trace_id", tid)
		}
	}).SetAddSource(true).SetLevel(log.LevelDebug))

	slog.Info("=== 广播测试启动 ===")

	ctx := context.Background()

	// 创建客户端（作为发起者，同时注册本地服务）
	cli, err := crpc.Dial(ctx, "broadcast", ":9090",
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

	// 注册本地服务（这样本地调用也会被触发）
	localSvc := &BroadcastService{Name: "local"}
	if err := cli.RegisterName("broadcast", localSvc); err != nil {
		slog.Error("Failed to register service", "error", err)
		return
	}

	slog.Info("Sender client started, registered local service")

	// 等待其他接收者连接建立
	slog.Info("等待其他接收者连接...")
	time.Sleep(3 * time.Second)
	slog.Info("开始广播测试")

	// 启动广播测试
	go runBroadcastTest(ctx, cli)

	// 启动统计报告
	go reportStats()

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("=== 测试结束 ===")
	printFinalStats()
}

func runBroadcastTest(ctx context.Context, cli *client.Client) {
	counter := int64(0)
	ticker := time.NewTicker(100 * time.Millisecond) // 每100ms发送一次
	defer ticker.Stop()

	// 用于追踪每个请求收到的响应数
	responseTracker := struct {
		sync.Mutex
		counts map[int64]int64 // counter -> 收到的响应数
	}{
		counts: make(map[int64]int64),
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			counter++
			stats.TotalSent.Add(1)

			req := &BroadcastReq{
				Message: fmt.Sprintf("broadcast msg #%d", counter),
				Counter: counter,
			}

			// 初始化此请求的响应计数
			responseTracker.Lock()
			responseTracker.counts[counter] = 0
			responseTracker.Unlock()
			ctx := trace.WithTraceID(ctx, fmt.Sprintf("trace-broadcast-%d", counter))
			// 发起广播调用 - 使用 Call 方法而不是 Send
			err := cli.Call(ctx, "broadcast", "broadcast.Broadcast", req, nil,
				client.Options().
					SetBroadcast().
					SetReqCoderT(coder.Msgp).
					SetResCoderT(coder.Msgp).
					// SetDebug(true).
					SetBroadcastResNewFunc(func() any {
						return &BroadcastRes{}
					}).
					SetBroadcastResCallBack(func(ret any, err error, eos bool) bool {
						if err != nil {
							stats.TotalErrors.Add(1)
							if counter%1000 == 0 {
								slog.Warn("Broadcast error", "counter", counter, "error", err)
							}
							return true
						}

						if res, ok := ret.(*BroadcastRes); ok {
							stats.TotalReceived.Add(1)

							// 记录来源
							switch res.From {
							case "local":
								stats.TotalLocal.Add(1)
							case "receiver1":
								stats.TotalReceiver1.Add(1)
							case "receiver2":
								stats.TotalReceiver2.Add(1)
							default:
								stats.TotalUnknown.Add(1)
							}

							// 增加此请求的响应计数
							responseTracker.Lock()
							responseTracker.counts[counter]++
							count := responseTracker.counts[counter]
							responseTracker.Unlock()

							if counter%1000 == 0 {
								slog.Debug("Broadcast response",
									"counter", counter,
									"from", res.From,
									"count", count)
							}
						}

						if eos {
							stats.TotalEOS.Add(1)

							// 检查响应数量
							responseTracker.Lock()
							count := responseTracker.counts[counter]
							responseTracker.Unlock()

							if count < 3 {
								stats.mu.Lock()
								stats.missingSeqs = append(stats.missingSeqs, counter)
								stats.mu.Unlock()

								if counter%100 == 0 {
									slog.Warn("Broadcast incomplete",
										"counter", counter,
										"received", count,
										"expected", 3)
								}
							} else if counter%1000 == 0 {
								slog.Info("Broadcast complete", "counter", counter, "received", count)
							}
							return false
						}

						return true // 继续接收
					}),
			)

			if err != nil {
				slog.Error("Send failed", "error", err, "counter", counter)
				stats.TotalErrors.Add(1)
			}
		}
	}
}

func reportStats() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		printStats()
	}
}

func printStats() {
	sent := stats.TotalSent.Load()
	received := stats.TotalReceived.Load()
	errs := stats.TotalErrors.Load()
	eos := stats.TotalEOS.Load()
	local := stats.TotalLocal.Load()
	r1 := stats.TotalReceiver1.Load()
	r2 := stats.TotalReceiver2.Load()
	unknown := stats.TotalUnknown.Load()
	elapsed := time.Since(stats.LastSummary)

	// 计算预期接收数（每次广播应该有 3 个响应）
	expected := sent * 3
	missing := expected - received - errs

	// 获取缺失的请求序号
	stats.mu.Lock()
	missingCount := len(stats.missingSeqs)
	var recentMissing []int64
	if missingCount > 0 {
		if missingCount > 10 {
			recentMissing = stats.missingSeqs[missingCount-10:]
		} else {
			recentMissing = stats.missingSeqs
		}
	}
	stats.mu.Unlock()

	slog.Info("=== 1分钟统计 ===",
		"发送", sent,
		"接收", received,
		"错误", errs,
		"EOS", eos,
		"local", local,
		"receiver1", r1,
		"receiver2", r2,
		"unknown", unknown,
		"预期", expected,
		"缺失", missing,
		"缺失序号", fmt.Sprintf("%v...", recentMissing),
		"速率", fmt.Sprintf("%d/min", int(float64(sent)/elapsed.Minutes())),
		"成功率", fmt.Sprintf("%.2f%%", float64(received)/float64(expected)*100))

	stats.LastSummary = time.Now()
}

func printFinalStats() {
	sent := stats.TotalSent.Load()
	received := stats.TotalReceived.Load()
	errs := stats.TotalErrors.Load()
	eos := stats.TotalEOS.Load()
	local := stats.TotalLocal.Load()
	r1 := stats.TotalReceiver1.Load()
	r2 := stats.TotalReceiver2.Load()
	unknown := stats.TotalUnknown.Load()
	elapsed := time.Since(stats.LastSummary)

	// 计算预期接收数（每次广播应该有 3 个响应）
	expected := sent * 3
	missing := expected - received - errs

	// 获取所有缺失的请求序号
	stats.mu.Lock()
	allMissing := make([]int64, len(stats.missingSeqs))
	copy(allMissing, stats.missingSeqs)
	stats.mu.Unlock()

	slog.Info("=== 最终统计 ===",
		"发送", sent,
		"接收", received,
		"错误", errs,
		"EOS", eos,
		"local", local,
		"receiver1", r1,
		"receiver2", r2,
		"unknown", unknown,
		"预期", expected,
		"缺失", missing,
		"总耗时", elapsed,
		"成功率", fmt.Sprintf("%.2f%%", float64(received)/float64(expected)*100))

	if len(allMissing) > 0 {
		slog.Warn("缺失的请求序号", "count", len(allMissing), "seqs", fmt.Sprintf("%v", allMissing))
	}
}
