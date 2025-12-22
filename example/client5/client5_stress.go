package main

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ndsky1003/crpc/v3/client"
	"github.com/ndsky1003/crpc/v3/example/comm"
	"github.com/ndsky1003/crpc/v3/example/dto"
	"github.com/ndsky1003/crpc/v3/example/trace"
)

// 预分配请求对象池，减少GC压力
var reqPool = sync.Pool{
	New: func() interface{} {
		return &dto.Req{Name: "stress"}
	},
}

func main() {
	// 优化运行时参数
	runtime.GOMAXPROCS(runtime.NumCPU())

	// 设置GC更激进，减少STW时间
	runtime.SetMutexProfileFraction(1)
	runtime.SetBlockProfileRate(1)

	// 初始化客户端 - 不使用日志以提高性能
	c, err := client.Dial(context.Background(), "client5_max", ":8080",
		client.Options().SetSecret("ddddd").
			SetWithTraceID(func(ctx context.Context, tid string) context.Context {
				return trace.WithTraceID(ctx, tid)
			}).SetGenTraceID(func(ctx context.Context) string {
			return trace.ExtractorTraceID(ctx)
		}))
	if err != nil {
		fmt.Printf("dial error: %v\n", err)
		return
	}

	comm.Default_Client = c
	fmt.Println("Max Stress Test Client started...")

	// 等待连接建立
	time.Sleep(2 * time.Second)

	// 运行极限压力测试
	runExtremeStressTest()

	// 保持程序运行
	select {}
}

func runExtremeStressTest() {
	fmt.Println("\n========== 极限压力测试 ==========")

	// 测试不同场景
	scenarios := []struct {
		name        string
		method      string
		goroutines  int
		duration    time.Duration
		description string
	}{
		{
			name:        "Empty方法极限测试",
			method:      "empty",
			goroutines:  runtime.NumCPU() * 50, // 每个CPU核心50个goroutine
			duration:    10 * time.Second,
			description: "调用无参数无返回值的Empty方法",
		},
		{
			name:        "CtxOnly方法极限测试",
			method:      "ctxOnly",
			goroutines:  runtime.NumCPU() * 40,
			duration:    10 * time.Second,
			description: "调用只有Context的方法",
		},
		{
			name:        "ResOnly方法极限测试",
			method:      "resOnly",
			goroutines:  runtime.NumCPU() * 30,
			duration:    10 * time.Second,
			description: "调用只返回结果的方法",
		},
		{
			name:        "Full方法极限测试",
			method:      "full",
			goroutines:  runtime.NumCPU() * 20,
			duration:    10 * time.Second,
			description: "调用完整签名的方法",
		},
		{
			name:        "最终极限测试",
			method:      "empty",
			goroutines:  runtime.NumCPU() * 100, // 100倍CPU核心数
			duration:    30 * time.Second,
			description: "最大压力测试",
		},
	}

	for _, scenario := range scenarios {
		fmt.Printf("\n--- %s ---\n", scenario.name)
		fmt.Printf("并发数: %d\n", scenario.goroutines)
		fmt.Printf("持续时间: %v\n", scenario.duration)
		fmt.Printf("说明: %s\n", scenario.description)

		switch scenario.method {
		case "empty":
			runEmptyMethodTest(scenario.goroutines, scenario.duration)
		case "ctxOnly":
			runCtxOnlyTest(scenario.goroutines, scenario.duration)
		case "resOnly":
			runResOnlyTest(scenario.goroutines, scenario.duration)
		case "full":
			runFullMethodTest(scenario.goroutines, scenario.duration)
		}

		// 等待GC完成
		runtime.GC()
		time.Sleep(2 * time.Second)
	}

	fmt.Println("\n极限压力测试完成！")
}

// 最轻量的测试 - Empty方法
func runEmptyMethodTest(goroutines int, duration time.Duration) {
	var (
		totalRequests int64
		wg            sync.WaitGroup
		ctx, cancel   = context.WithTimeout(context.Background(), duration)
	)

	defer cancel()

	startTime := time.Now()

	// 启动所有goroutine
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					comm.Empty()
					atomic.AddInt64(&totalRequests, 1)
				}
			}
		}()
	}

	wg.Wait()
	totalTime := time.Since(startTime)

	printExtremeStats(totalRequests, totalTime)
}

// CtxOnly测试
func runCtxOnlyTest(goroutines int, duration time.Duration) {
	var (
		totalRequests int64
		wg            sync.WaitGroup
		ctx, cancel   = context.WithTimeout(context.Background(), duration)
	)

	defer cancel()

	startTime := time.Now()

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := context.Background()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					comm.CtxOnly(c)
					atomic.AddInt64(&totalRequests, 1)
				}
			}
		}()
	}

	wg.Wait()
	totalTime := time.Since(startTime)

	printExtremeStats(totalRequests, totalTime)
}

// ResOnly测试 - 使用对象池
func runResOnlyTest(goroutines int, duration time.Duration) {
	var (
		totalRequests int64
		wg            sync.WaitGroup
		ctx, cancel   = context.WithTimeout(context.Background(), duration)
	)

	defer cancel()

	startTime := time.Now()

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					req := reqPool.Get().(*dto.Req)
					_, _ = comm.ResOnly(req)
					reqPool.Put(req)
					atomic.AddInt64(&totalRequests, 1)
				}
			}
		}()
	}

	wg.Wait()
	totalTime := time.Since(startTime)

	printExtremeStats(totalRequests, totalTime)
}

// Full方法测试 - 包含所有参数
func runFullMethodTest(goroutines int, duration time.Duration) {
	var (
		totalRequests int64
		wg            sync.WaitGroup
		ctx, cancel   = context.WithTimeout(context.Background(), duration)
	)

	defer cancel()

	startTime := time.Now()

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := context.Background()
			meta := &dto.Meta{Source: "stress"}
			req := reqPool.Get().(*dto.Req)
			req.Name = "stress"

			for {
				select {
				case <-ctx.Done():
					reqPool.Put(req)
					return
				default:
					_, _ = comm.Full(c, meta, req)
					atomic.AddInt64(&totalRequests, 1)
				}
			}
		}()
	}

	wg.Wait()
	totalTime := time.Since(startTime)

	printExtremeStats(totalRequests, totalTime)
}

// 打印极限统计
func printExtremeStats(totalRequests int64, totalTime time.Duration) {
	qps := float64(totalRequests) / totalTime.Seconds()

	fmt.Printf("\n【极限测试结果】\n")
	fmt.Printf("总请求数: %d\n", totalRequests)
	fmt.Printf("总耗时: %v\n", totalTime)
	fmt.Printf("QPS: %.0f req/s\n", qps)

	// 计算每秒请求数的更友好表示
	if qps > 1000000 {
		fmt.Printf("QPS: %.2f M req/s\n", qps/1000000)
	} else if qps > 1000 {
		fmt.Printf("QPS: %.2f K req/s\n", qps/1000)
	}

	// 计算平均延迟
	avgLatency := totalTime.Nanoseconds() / totalRequests
	fmt.Printf("平均延迟: %v\n", time.Duration(avgLatency))

	// CPU和内存信息
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Printf("内存使用: %.2f MB\n", float64(m.Alloc)/1024/1024)
	fmt.Printf("GC次数: %d\n", m.NumGC)
	fmt.Println("---------------------------------")
}

// 预热测试
func warmup() {
	fmt.Println("\n预热中...")
	for i := 0; i < 10000; i++ {
		comm.Empty()
	}
	fmt.Println("预热完成")
}