package main

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ndsky1003/crpc/v3/client"
	"github.com/ndsky1003/crpc/v3/example/comm"
	"github.com/ndsky1003/crpc/v3/example/dto"
	"github.com/ndsky1003/crpc/v3/example/trace"
	"github.com/ndsky1003/log"
	"github.com/panjf2000/ants/v2"
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
	fmt.Println("Client5 started... 准备进行压力测试")

	// 等待连接建立
	time.Sleep(2 * time.Second)

	// 运行压力测试
	runStressTest()

	// 运行不同方法类型的性能测试
	testMethodPerformance()

	// 保持程序运行
	select {}
}

func runStressTest() {
	// 预热
	warmup()

	// 压力测试配置 - 更激进的参数
	configs := []struct {
		name          string
		concurrency   int
		totalRequests int
		duration      time.Duration
	}{
		{"轻度测试", 100, 10000, 0},
		{"中度测试", 500, 50000, 0},
		{"重度测试", 1000, 100000, 0},
		{"疯狂测试", 2000, 200000, 0},
		{"极限测试(30秒)", runtime.NumCPU() * 50, 0, 30 * time.Second},
		{"超越极限测试(60秒)", runtime.NumCPU() * 100, 0, 60 * time.Second},
	}

	for _, config := range configs {
		fmt.Printf("\n========== 开始 %s ==========\n", config.name)
		fmt.Printf("并发数: %d\n", config.concurrency)
		if config.totalRequests > 0 {
			fmt.Printf("总请求数: %d\n", config.totalRequests)
		}
		if config.duration > 0 {
			fmt.Printf("持续时间: %v\n", config.duration)
		}

		if config.duration > 0 {
			runDurationTest(config.concurrency, config.duration)
		} else {
			runCountTest(config.concurrency, config.totalRequests)
		}

		// 测试间隔
		fmt.Println("\n等待 5 秒后进行下一个测试...")
		time.Sleep(5 * time.Second)
	}

	fmt.Println("\n========== 所有压力测试完成 ==========")
}

// 预热函数
func warmup() {
	fmt.Println("\n预热中...")
	for i := 0; i < 10000; i++ {
		comm.Empty()
	}
	fmt.Println("预热完成")
}

func runCountTest(concurrency, totalRequests int) {
	var (
		successCount   int64
		failureCount   int64
		totalLatency   int64
		startTime      = time.Now()
		wg             sync.WaitGroup
	)

	// 创建 goroutine 池
	pool, _ := ants.NewPoolWithFunc(concurrency, func(i interface{}) {
		defer wg.Done()

		req := i.(*dto.Req)
		ctx := context.Background()
		meta := &dto.Meta{Source: "client5"}

		requestStart := time.Now()
		_, err := comm.Full(ctx, meta, req)
		latency := time.Since(requestStart)

		atomic.AddInt64(&totalLatency, latency.Nanoseconds())

		if err != nil {
			atomic.AddInt64(&failureCount, 1)
		} else {
			atomic.AddInt64(&successCount, 1)
		}
	})
	defer pool.Release()

	// 分发请求
	fmt.Println("\n开始发送请求...")
	for i := 0; i < totalRequests; i++ {
		wg.Add(1)
		req := &dto.Req{Name: fmt.Sprintf("stress_test_%d", i)}
		_ = pool.Invoke(req)
	}

	wg.Wait()
	totalTime := time.Since(startTime)

	// 输出统计结果
	printStats(totalRequests, successCount, failureCount, totalLatency, totalTime)
}

func runDurationTest(concurrency int, duration time.Duration) {
	var (
		successCount   int64
		failureCount   int64
		totalLatency   int64
		totalRequests  int64
		startTime      = time.Now()
		wg             sync.WaitGroup
		ctx, cancel    = context.WithTimeout(context.Background(), duration)
	)

	defer cancel()

	// 创建 goroutine 池
	pool, _ := ants.NewPoolWithFunc(concurrency, func(i interface{}) {
		defer wg.Done()

		for {
			select {
			case <-ctx.Done():
				return
			default:
				atomic.AddInt64(&totalRequests, 1)

				req := &dto.Req{Name: fmt.Sprintf("stress_test_%d", atomic.LoadInt64(&totalRequests))}
				meta := &dto.Meta{Source: "client5"}

				requestStart := time.Now()
				_, err := comm.Full(context.Background(), meta, req)
				latency := time.Since(requestStart)

				atomic.AddInt64(&totalLatency, latency.Nanoseconds())

				if err != nil {
					atomic.AddInt64(&failureCount, 1)
				} else {
					atomic.AddInt64(&successCount, 1)
				}
			}
		}
	})
	defer pool.Release()

	// 启动所有 worker
	fmt.Printf("\n开始 %v 的压力测试...\n", duration)
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		_ = pool.Invoke(i)
	}

	// 等待时间结束
	<-ctx.Done()

	// 等待所有 goroutine 退出
	time.Sleep(100 * time.Millisecond)
	pool.Release()
	wg.Wait()

	totalTime := time.Since(startTime)

	// 输出统计结果
	printStats(int(totalRequests), successCount, failureCount, totalLatency, totalTime)
}

func printStats(totalRequests int, successCount, failureCount, totalLatency int64, totalTime time.Duration) {
	fmt.Println("\n---------- 测试结果统计 ----------")
	fmt.Printf("总请求数: %d\n", totalRequests)
	fmt.Printf("成功请求: %d (%.2f%%)\n", successCount, float64(successCount)/float64(totalRequests)*100)
	fmt.Printf("失败请求: %d (%.2f%%)\n", failureCount, float64(failureCount)/float64(totalRequests)*100)

	if totalRequests > 0 {
		avgLatency := time.Duration(totalLatency / int64(totalRequests))
		fmt.Printf("平均延迟: %v\n", avgLatency)

		qps := float64(totalRequests) / totalTime.Seconds()
		fmt.Printf("QPS: %.2f\n", qps)
	}

	fmt.Printf("总耗时: %v\n", totalTime)
	fmt.Println("---------------------------------")
}

// 测试不同方法类型的性能
func testMethodPerformance() {
	fmt.Println("\n========== 不同方法类型性能测试 ==========")

	methods := []struct {
		name string
		fn   func() error
	}{
		{
			name: "Full (Ctx, Meta, Req) -> (Res, error)",
			fn: func() error {
				ctx := context.Background()
				meta := &dto.Meta{Source: "client5"}
				req := &dto.Req{Name: "perf_test"}
				_, err := comm.Full(ctx, meta, req)
				return err
			},
		},
		{
			name: "CtxReqPtr (Ctx, Req) -> (Res, error)",
			fn: func() error {
				ctx := context.Background()
				req := &dto.Req{Name: "perf_test"}
				_, err := comm.CtxReqPtr(ctx, req)
				return err
			},
		},
		{
			name: "ReqOnly (Req) -> (Res, error)",
			fn: func() error {
				req := &dto.Req{Name: "perf_test"}
				_, err := comm.ReqOnly(req)
				return err
			},
		},
		{
			name: "ResOnly (Req) -> (Res, error)",
			fn: func() error {
				req := &dto.Req{Name: "perf_test"}
				_, err := comm.ResOnly(req)
				return err
			},
		},
		{
			name: "CtxOnlyResErr (Ctx) -> (Res, error)",
			fn: func() error {
				ctx := context.Background()
				_, err := comm.CtxOnlyResErr(ctx)
				return err
			},
		},
	}

	for _, method := range methods {
		fmt.Printf("\n测试方法: %s\n", method.name)

		var (
			successCount   int64
			failureCount   int64
			totalLatency   int64
			testRequests   = 1000
			concurrency    = 20
			wg             sync.WaitGroup
		)

		pool, _ := ants.NewPoolWithFunc(concurrency, func(i interface{}) {
			defer wg.Done()

			start := time.Now()
			err := method.fn()
			latency := time.Since(start)

			atomic.AddInt64(&totalLatency, latency.Nanoseconds())

			if err != nil {
				atomic.AddInt64(&failureCount, 1)
			} else {
				atomic.AddInt64(&successCount, 1)
			}
		})
		defer pool.Release()

		testStart := time.Now()
		for i := 0; i < testRequests; i++ {
			wg.Add(1)
			_ = pool.Invoke(i)
		}
		wg.Wait()
		testTime := time.Since(testStart)

		fmt.Printf("  成功率: %.2f%% (%d/%d)\n",
			float64(successCount)/float64(testRequests)*100,
			successCount, testRequests)
		fmt.Printf("  平均延迟: %v\n",
			time.Duration(totalLatency/int64(testRequests)))
		fmt.Printf("  QPS: %.2f\n",
			float64(testRequests)/testTime.Seconds())
	}
}