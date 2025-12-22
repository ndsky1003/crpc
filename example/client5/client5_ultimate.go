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
	// 显示系统信息
	fmt.Printf("系统信息:\n")
	fmt.Printf("  CPU核心数: %d\n", runtime.NumCPU())
	fmt.Printf("  GOOS: %s\n", runtime.GOOS)
	fmt.Printf("  GOARCH: %s\n", runtime.GOARCH)

	// 优化运行时
	runtime.GOMAXPROCS(runtime.NumCPU())
	runtime.SetMutexProfileFraction(0)
	runtime.SetBlockProfileRate(0)

	// 调整GOGC以减少GC频率（更激进的设置）
	// fmt.Println("设置 GOGC=1000")
	// os.Setenv("GOGC", "1000")

	// 初始化客户端 - 最简配置
	c, err := client.Dial(context.Background(), "client5_ultimate", ":8080",
		client.Options().SetSecret("ddddd"))
	if err != nil {
		fmt.Printf("dial error: %v\n", err)
		return
	}

	comm.Default_Client = c
	fmt.Println("\nUltimate Performance Stress Test Client started...")

	// 等待连接建立
	time.Sleep(2 * time.Second)

	// 运行终极压力测试
	runUltimateTest()
}

func runUltimateTest() {
	fmt.Println("\n========== 终极压力测试 ==========")

	// 预热阶段
	fmt.Println("\n第一阶段：预热...")
	preWarm()

	// 测试阶段
	phases := []struct {
		name       string
		goroutines int
		duration   time.Duration
		method     string
	}{
		{"Phase 1: Empty方法基准", runtime.NumCPU() * 10, 5 * time.Second, "empty"},
		{"Phase 2: Empty方法扩展", runtime.NumCPU() * 50, 10 * time.Second, "empty"},
		{"Phase 3: Empty方法极限", runtime.NumCPU() * 100, 15 * time.Second, "empty"},
		{"Phase 4: Empty方法疯狂", runtime.NumCPU() * 200, 20 * time.Second, "empty"},
		{"Phase 5: Empty方法终极", runtime.NumCPU() * 500, 30 * time.Second, "empty"},
		{"Phase 6: CtxOnly测试", runtime.NumCPU() * 100, 10 * time.Second, "ctxOnly"},
		{"Phase 7: 系统最大负载", runtime.NumCPU() * 1000, 60 * time.Second, "empty"},
	}

	var bestQPS float64
	var bestPhase string

	for i, phase := range phases {
		fmt.Printf("\n%s\n", phase.name)
		fmt.Printf("  并发数: %d\n", phase.goroutines)
		fmt.Printf("  持续时间: %v\n", phase.duration)

		// 记录系统状态
		runtime.GC()
		time.Sleep(100 * time.Millisecond)

		var mStart runtime.MemStats
		runtime.ReadMemStats(&mStart)

		// 执行测试
		var qps float64
		var totalRequests int64

		if phase.method == "empty" {
			qps, totalRequests = runEmptyTest(phase.goroutines, phase.duration)
		} else {
			qps, totalRequests = runCtxOnlyTest(phase.goroutines, phase.duration)
		}

		var mEnd runtime.MemStats
		runtime.ReadMemStats(&mEnd)

		// 输出结果
		fmt.Printf("  结果:\n")
		fmt.Printf("    QPS: %.2f", qps)
		if qps > 1000000 {
			fmt.Printf(" (%.2f M req/s)", qps/1000000)
		}
		fmt.Printf("\n")
		fmt.Printf("    总请求: %d\n", totalRequests)
		fmt.Printf("    内存使用: %.2f MB\n", float64(mEnd.Alloc)/1024/1024)
		fmt.Printf("    GC次数: %d\n", mEnd.NumGC-mStart.NumGC)

		// 更新最佳成绩
		if qps > bestQPS {
			bestQPS = qps
			bestPhase = phase.name
		}

		// 恢复时间
		if i < len(phases)-1 {
			fmt.Printf("  等待系统恢复...\n")
			time.Sleep(3 * time.Second)
		}
	}

	// 总结
	fmt.Printf("\n========== 测试总结 ==========\n")
	fmt.Printf("最佳性能: %s\n", bestPhase)
	fmt.Printf("最高QPS: %.2f", bestQPS)
	if bestQPS > 1000000 {
		fmt.Printf(" (%.2f M req/s)", bestQPS/1000000)
	}
	fmt.Printf("\n")

	// 性能评级
	if bestQPS > 1000000 {
		fmt.Printf("性能评级: 超顶级 (>1M QPS)\n")
	} else if bestQPS > 500000 {
		fmt.Printf("性能评级: 顶级 (500K-1M QPS)\n")
	} else if bestQPS > 100000 {
		fmt.Printf("性能评级: 高性能 (100K-500K QPS)\n")
	} else {
		fmt.Printf("性能评级: 正常 (<100K QPS)\n")
	}
}

func preWarm() {
	// 逐步预热
	stages := []int{1000, 5000, 10000, 50000}
	for _, n := range stages {
		for i := 0; i < n; i++ {
			comm.Empty()
		}
		fmt.Printf("  完成 %d 次预热调用\n", n)
		runtime.GC()
	}
	fmt.Println("  预热完成")
}

// 优化的 Empty 方法测试
func runEmptyTest(goroutines int, duration time.Duration) (float64, int64) {
	var (
		totalRequests int64
		wg           sync.WaitGroup
		ctx, cancel  = context.WithTimeout(context.Background(), duration)
	)

	defer cancel()

	start := time.Now()

	// 使用批量启动模式
	batchSize := 1000
	batches := goroutines / batchSize
	if batches == 0 {
		batches = 1
	}

	for b := 0; b < batches; b++ {
		// 批量启动 goroutines
		for i := 0; i < batchSize; i++ {
			if len(ctx.Done()) > 0 {
				break
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				// 无限循环调用
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
	}

	// 处理剩余的
	remaining := goroutines - batches*batchSize
	for i := 0; i < remaining; i++ {
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
	elapsed := time.Since(start)

	return float64(totalRequests) / elapsed.Seconds(), totalRequests
}

// CtxOnly 测试
func runCtxOnlyTest(goroutines int, duration time.Duration) (float64, int64) {
	var (
		totalRequests int64
		wg           sync.WaitGroup
		ctx, cancel  = context.WithTimeout(context.Background(), duration)
	)

	defer cancel()

	start := time.Now()

	// 预创建 context 以减少开销
	c := context.Background()

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
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
	elapsed := time.Since(start)

	return float64(totalRequests) / elapsed.Seconds(), totalRequests
}