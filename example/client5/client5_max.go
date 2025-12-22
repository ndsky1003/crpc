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
)

func main() {
	// 优化运行时参数以获得最大性能
	runtime.GOMAXPROCS(runtime.NumCPU())

	// 禁用GC以减少STW暂停（仅在测试期间）
	// runtime.GC()
	// 注意：生产环境不要这样做

	// 初始化客户端 - 使用最简配置
	c, err := client.Dial(context.Background(), "client5_max", ":8080",
		client.Options().SetSecret("ddddd"))
	if err != nil {
		fmt.Printf("dial error: %v\n", err)
		return
	}

	comm.Default_Client = c
	fmt.Println("Maximum Performance Stress Test Client started...")

	// 等待连接建立
	time.Sleep(2 * time.Second)

	// 运行最大性能测试
	runMaximumPerformanceTest()

	// 保持程序运行
	select {}
}

func runMaximumPerformanceTest() {
	fmt.Println("\n========== 最大性能压力测试 ==========")

	// 预热
	fmt.Println("\n预热中...")
	for i := 0; i < 50000; i++ {
		comm.Empty()
	}
	fmt.Println("预热完成")

	// 强制GC
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	// 测试场景 - 只测试最轻量的方法
	testScenarios := []struct {
		name        string
		goroutines  int
		duration    time.Duration
	}{
		{
			name:       "基线测试",
			goroutines: runtime.NumCPU() * 10,
			duration:   10 * time.Second,
		},
		{
			name:       "高并发测试",
			goroutines: runtime.NumCPU() * 50,
			duration:   10 * time.Second,
		},
		{
			name:       "极限测试",
			goroutines: runtime.NumCPU() * 100,
			duration:   15 * time.Second,
		},
		{
			name:       "超越极限测试",
			goroutines: runtime.NumCPU() * 200,
			duration:   20 * time.Second,
		},
		{
			name:       "系统最大测试",
			goroutines: runtime.NumCPU() * 500,
			duration:   30 * time.Second,
		},
	}

	for _, scenario := range testScenarios {
		fmt.Printf("\n--- %s ---\n", scenario.name)
		fmt.Printf("CPU核心数: %d\n", runtime.NumCPU())
		fmt.Printf("Goroutine数: %d\n", scenario.goroutines)
		fmt.Printf("持续时间: %v\n", scenario.duration)

		// 记录测试前的内存状态
		var m1 runtime.MemStats
		runtime.ReadMemStats(&m1)

		// 运行测试
		qps, avgLatency, totalRequests := runPerformanceTest(scenario.goroutines, scenario.duration)

		// 记录测试后的内存状态
		var m2 runtime.MemStats
		runtime.ReadMemStats(&m2)

		fmt.Printf("\n【测试结果】\n")
		fmt.Printf("QPS: %.2f req/s", qps)
		if qps > 1000000 {
			fmt.Printf(" (%.2f M req/s)", qps/1000000)
		}
		fmt.Printf("\n")

		fmt.Printf("平均延迟: %v\n", avgLatency)
		fmt.Printf("总请求数: %d\n", totalRequests)
		fmt.Printf("内存使用: %.2f MB\n", float64(m2.Alloc)/1024/1024)
		fmt.Printf("内存增长: %.2f MB\n", float64(m2.Alloc-m1.Alloc)/1024/1024)
		fmt.Printf("GC次数: %d\n", m2.NumGC-m1.NumGC)

		// 等待系统恢复
		runtime.GC()
		time.Sleep(3 * time.Second)
	}
}

func runPerformanceTest(goroutines int, duration time.Duration) (float64, time.Duration, int64) {
	var (
		totalRequests int64
		wg           sync.WaitGroup
		ctx, cancel  = context.WithTimeout(context.Background(), duration)
	)

	defer cancel()

	startTime := time.Now()

	// 启动所有goroutine，每个循环调用Empty方法
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 无限循环调用，直到时间到
			for {
				select {
				case <-ctx.Done():
					return
				default:
					// 调用最轻量的方法
					comm.Empty()
					atomic.AddInt64(&totalRequests, 1)
				}
			}
		}()
	}

	wg.Wait()
	totalTime := time.Since(startTime)

	qps := float64(totalRequests) / totalTime.Seconds()
	var avgLatency time.Duration
	if totalRequests > 0 {
		avgLatency = totalTime / time.Duration(totalRequests)
	}

	return qps, avgLatency, totalRequests
}