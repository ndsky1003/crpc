//go:build !business && !small
// +build !business,!small

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

// 性能统计
type PerformanceStats struct {
	TotalRequests    int64     `json:"total_requests"`
	SuccessRequests  int64     `json:"success_requests"`
	FailedRequests   int64     `json:"failed_requests"`
	TotalLatency     int64     `json:"total_latency_ns"`
	MinLatency       int64     `json:"min_latency_ns"`
	MaxLatency       int64     `json:"max_latency_ns"`
	StartTime        time.Time `json:"start_time"`
	LastUpdateTime   time.Time `json:"last_update_time"`
	BytesSent        int64     `json:"bytes_sent"`
	BytesReceived    int64     `json:"bytes_received"`
}

// 测试场景
type TestScenario struct {
	Name        string
	Method      string
	Description string
	RequestGen  func(int) interface{}
}

func main() {
	// 显示系统信息
	fmt.Println("========== 长期压力测试 Client6 ==========")
	fmt.Printf("CPU核心数: %d\n", runtime.NumCPU())
	fmt.Printf("GOOS: %s\n", runtime.GOOS)
	fmt.Printf("GOARCH: %s\n", runtime.GOARCH)
	fmt.Printf("开始时间: %s\n", time.Now().Format("2006-01-02 15:04:05"))

	// 优化运行时
	runtime.GOMAXPROCS(runtime.NumCPU())

	// 初始化客户端
	c, err := client.Dial(context.Background(), "client6", ":8080",
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
	fmt.Println("Client6 初始化成功，等待连接建立...")
	time.Sleep(3 * time.Second)

	// 运行1小时的压力测试
	runOneHourTest()

	fmt.Println("\n测试完成！")
}

func runOneHourTest() {
	fmt.Println("\n========== 开始1小时长期压力测试 ==========")

	// 预热
	fmt.Println("\n预热中...")
	preWarm()

	// 定义测试场景
	scenarios := []TestScenario{
		{
			Name:        "用户查询",
			Method:      "userQuery",
			Description: "查询用户信息",
			RequestGen:  generateUserQueryRequest,
		},
		{
			Name:        "订单查询",
			Method:      "orderQuery",
			Description: "查询订单列表",
			RequestGen:  generateOrderQueryRequest,
		},
		{
			Name:        "商品搜索",
			Method:      "productSearch",
			Description: "搜索商品",
			RequestGen:  generateProductSearchRequest,
		},
		{
			Name:        "创建订单",
			Method:      "createOrder",
			Description: "创建新订单",
			RequestGen:  generateCreateOrderRequest,
		},
		{
			Name:        "购物车操作",
			Method:      "cartOperation",
			Description: "购物车增删改查",
			RequestGen:  generateCartRequest,
		},
	}

	// 创建性能统计
	stats := &PerformanceStats{
		StartTime:      time.Now(),
		MinLatency:     int64(^uint64(0) >> 1), // 最大值
	}

	// 启动性能监控协程
	stopMonitor := make(chan bool)
	go performanceMonitor(stats, stopMonitor)

	// 测试参数
	testDuration := time.Hour
	concurrency := runtime.NumCPU() * 20 // 每个CPU核心20个goroutine
	ctx, cancel := context.WithTimeout(context.Background(), testDuration)

	fmt.Printf("\n测试配置:\n")
	fmt.Printf("  持续时间: %v\n", testDuration)
	fmt.Printf("  并发数: %d\n", concurrency)
	fmt.Printf("  场景数: %d\n", len(scenarios))
	fmt.Printf("\n测试开始时间: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println("\n========== 测试进行中 ==========")

	var wg sync.WaitGroup

	// 启动压力测试goroutines
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			// 每个goroutine使用不同的场景
			scenarioIndex := goroutineID % len(scenarios)
			scenario := scenarios[scenarioIndex]
			requestCounter := 0

			for {
				select {
				case <-ctx.Done():
					return
				default:
					// 生成请求
					req := scenario.RequestGen(requestCounter)
					requestCounter++

					// 执行请求
					start := time.Now()
					err := executeBusinessMethod(scenario.Method, req)
					latency := time.Since(start)

					// 更新统计
					updateStats(stats, err, latency)

					// 模拟真实业务的少量延迟
					time.Sleep(time.Microsecond * time.Duration(100+requestCounter%200))
				}
			}
		}(i)
	}

	// 等待测试完成
	wg.Wait()
	cancel()
	close(stopMonitor)

	// 打印最终统计
	printFinalStats(stats)
}

// 执行业务方法
func executeBusinessMethod(method string, req interface{}) error {
	ctx := context.Background()
	meta := &dto.Meta{Source: "client6"}

	// 所有方法都使用 req
	if request, ok := req.(*dto.Req); ok {
		_, err := comm.Full(ctx, meta, request)
		return err
	}

	return fmt.Errorf("invalid request type for method: %s", method)
}

// 生成用户查询请求
func generateUserQueryRequest(counter int) interface{} {
	return &dto.Req{
		Name: fmt.Sprintf("user_query_%d_%d", counter, time.Now().UnixNano()),
	}
}

// 生成订单查询请求
func generateOrderQueryRequest(counter int) interface{} {
	return &dto.Req{
		Name: fmt.Sprintf("order_query_%d_%d", counter, time.Now().UnixNano()),
	}
}

// 生成商品搜索请求
func generateProductSearchRequest(counter int) interface{} {
	return &dto.Req{
		Name: fmt.Sprintf("product_search_%d_%d", counter, time.Now().UnixNano()),
	}
}

// 生成创建订单请求
func generateCreateOrderRequest(counter int) interface{} {
	return &dto.Req{
		Name: fmt.Sprintf("create_order_%d_%d", counter, time.Now().UnixNano()),
	}
}

// 生成购物车请求
func generateCartRequest(counter int) interface{} {
	return &dto.Req{
		Name: fmt.Sprintf("cart_op_%d_%d", counter, time.Now().UnixNano()),
	}
}

// 预热
func preWarm() {
	for i := 0; i < 1000; i++ {
		req := generateUserQueryRequest(i)
		if request, ok := req.(*dto.Req); ok {
			ctx := context.Background()
			meta := &dto.Meta{Source: "client6"}
			_, _ = comm.Full(ctx, meta, request)
		}
	}
	fmt.Println("预热完成")
}

// 更新统计信息
func updateStats(stats *PerformanceStats, err error, latency time.Duration) {
	atomic.AddInt64(&stats.TotalRequests, 1)
	atomic.AddInt64(&stats.TotalLatency, latency.Nanoseconds())

	latencyNs := latency.Nanoseconds()

	// 更新最小延迟
	for {
		old := atomic.LoadInt64(&stats.MinLatency)
		if latencyNs >= old || atomic.CompareAndSwapInt64(&stats.MinLatency, old, latencyNs) {
			break
		}
	}

	// 更新最大延迟
	for {
		old := atomic.LoadInt64(&stats.MaxLatency)
		if latencyNs <= old || atomic.CompareAndSwapInt64(&stats.MaxLatency, old, latencyNs) {
			break
		}
	}

	if err != nil {
		atomic.AddInt64(&stats.FailedRequests, 1)
	} else {
		atomic.AddInt64(&stats.SuccessRequests, 1)
	}

	stats.LastUpdateTime = time.Now()
}

// 性能监控器
func performanceMonitor(stats *PerformanceStats, stop chan bool) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	reportCount := 0

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			reportCount++
			printStats(stats, reportCount)
		}
	}
}

// 打印统计信息
func printStats(stats *PerformanceStats, reportCount int) {
	totalReq := atomic.LoadInt64(&stats.TotalRequests)
	successReq := atomic.LoadInt64(&stats.SuccessRequests)
	failedReq := atomic.LoadInt64(&stats.FailedRequests)
	totalLatency := atomic.LoadInt64(&stats.TotalLatency)
	minLatency := atomic.LoadInt64(&stats.MinLatency)
	maxLatency := atomic.LoadInt64(&stats.MaxLatency)

	elapsed := time.Since(stats.StartTime)
	qps := float64(totalReq) / elapsed.Seconds()
	avgLatency := time.Duration(0)
	if totalReq > 0 {
		avgLatency = time.Duration(totalLatency / totalReq)
	}

	successRate := float64(0)
	if totalReq > 0 {
		successRate = float64(successReq) / float64(totalReq) * 100
	}

	// 获取内存信息
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	fmt.Printf("\n========== 第 %d 次性能报告 (运行时间: %v) ==========\n", reportCount, elapsed.Round(time.Second))
	fmt.Printf("请求统计:\n")
	fmt.Printf("  总请求数: %d\n", totalReq)
	fmt.Printf("  成功请求: %d (%.2f%%)\n", successReq, successRate)
	fmt.Printf("  失败请求: %d\n", failedReq)
	fmt.Printf("\n性能指标:\n")
	fmt.Printf("  QPS: %.2f req/s", qps)
	if qps > 1000000 {
		fmt.Printf(" (%.2f M req/s)", qps/1000000)
	}
	fmt.Printf("\n")
	fmt.Printf("  平均延迟: %v\n", avgLatency)
	fmt.Printf("  最小延迟: %v\n", time.Duration(minLatency))
	fmt.Printf("  最大延迟: %v\n", time.Duration(maxLatency))
	fmt.Printf("\n系统资源:\n")
	fmt.Printf("  内存使用: %.2f MB\n", float64(m.Alloc)/1024/1024)
	fmt.Printf("  系统内存: %.2f MB\n", float64(m.Sys)/1024/1024)
	fmt.Printf("  GC次数: %d\n", m.NumGC)
	fmt.Printf("  Goroutine数: %d\n", runtime.NumGoroutine())
	fmt.Println("==============================================")
}

// 打印最终统计
func printFinalStats(stats *PerformanceStats) {
	fmt.Printf("\n========== 最终测试报告 ==========\n")
	fmt.Printf("测试开始时间: %s\n", stats.StartTime.Format("2006-01-02 15:04:05"))
	fmt.Printf("测试结束时间: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("总运行时间: %v\n", time.Since(stats.StartTime).Round(time.Second))

	totalReq := atomic.LoadInt64(&stats.TotalRequests)
	successReq := atomic.LoadInt64(&stats.SuccessRequests)
	failedReq := atomic.LoadInt64(&stats.FailedRequests)

	elapsed := time.Since(stats.StartTime)
	qps := float64(totalReq) / elapsed.Seconds()

	fmt.Printf("\n总请求统计:\n")
	fmt.Printf("  总请求数: %d\n", totalReq)
	fmt.Printf("  成功请求: %d\n", successReq)
	fmt.Printf("  失败请求: %d\n", failedReq)
	fmt.Printf("  成功率: %.2f%%\n", float64(successReq)/float64(totalReq)*100)

	fmt.Printf("\n最终性能:\n")
	fmt.Printf("  平均QPS: %.2f req/s\n", qps)
	fmt.Printf("  总数据量: %.2f MB\n", float64(totalReq*1024)/1024/1024) // 假设每个请求1KB

	// 性能评级
	if qps > 100000 {
		fmt.Printf("\n性能评级: 超高性能 (>100K QPS)\n")
	} else if qps > 50000 {
		fmt.Printf("\n性能评级: 高性能 (50K-100K QPS)\n")
	} else if qps > 10000 {
		fmt.Printf("\n性能评级: 中高性能 (10K-50K QPS)\n")
	} else {
		fmt.Printf("\n性能评级: 标准 (<10K QPS)\n")
	}
}