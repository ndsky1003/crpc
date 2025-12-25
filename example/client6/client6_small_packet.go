//go:build small
// +build small

package main

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ndsky1003/crpc/v3/client"
	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/crpc/v3/example/comm"
	"github.com/ndsky1003/crpc/v3/example/dto"
	"github.com/ndsky1003/crpc/v3/example/trace"
)

// 性能统计
type PerformanceStats struct {
	TotalRequests int64     `json:"total_requests"`
	SuccessRequests int64    `json:"success_requests"`
	FailedRequests  int64    `json:"failed_requests"`
	TotalLatency    int64    `json:"total_latency_ns"`
	MinLatency      int64    `json:"min_latency_ns"`
	MaxLatency      int64    `json:"max_latency_ns"`
	StartTime       time.Time `json:"start_time"`
	TotalReqBytes   int64     `json:"total_req_bytes"`
	TotalResBytes   int64     `json:"total_res_bytes"`
}

// 测试场景
type TestScenario struct {
	Name     string
	Requests []*dto.BusinessReq
	Execute  func(context.Context, *dto.BusinessReq, ...*client.Option) (*dto.BusinessRes, error)
}

func main() {
	// 显示系统信息
	fmt.Println("========== 小包压力测试 Client6 (目标 ~1KB 包) ==========")
	fmt.Printf("CPU核心数: %d\n", runtime.NumCPU())
	fmt.Printf("GOOS: %s\n", runtime.GOOS)
	fmt.Printf("GOARCH: %s\n", runtime.GOARCH)
	fmt.Printf("开始时间: %s\n", time.Now().Format("2006-01-02 15:04:05"))

	// 优化运行时
	runtime.GOMAXPROCS(runtime.NumCPU())

	// 初始化客户端
	c, err := client.Dial(context.Background(), "client6_small", ":8080",
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

	// 预生成静态测试数据（使用小包 ~1KB）
	fmt.Println("\n预生成测试数据中...")
	scenarios := initTestScenarios()
	fmt.Println("测试数据生成完成")

	// 预热
	fmt.Println("\n预热中...")
	preWarm(scenarios)

	// 创建性能统计
	stats := &PerformanceStats{
		StartTime:  time.Now(),
		MinLatency: int64(^uint64(0) >> 1), // 最大值
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

			// 预计算请求大小（固定不变）
			reqBytes, _ := coder.Marshal(coder.Msgp, scenario.Requests[0])

			for {
				select {
				case <-ctx.Done():
					return
				default:
					// 直接使用预生成的请求（只有一个）
					req := scenario.Requests[0]

					// 执行请求
					start := time.Now()
					res, err := scenario.Execute(context.Background(), req)
					latency := time.Since(start)

					// 统计数据包大小
					var resBytes int
					if res != nil {
						resData, _ := coder.Marshal(coder.Msgp, res)
						resBytes = len(resData)
					}

					// 更新统计
					updateStats(stats, err, latency, len(reqBytes), resBytes)
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

// 初始化测试场景并预生成请求数据（使用小包 ~1KB）
func initTestScenarios() []TestScenario {
	baseTime := time.Now()
	fixedTimestamp := baseTime.Unix()

	// 用户查询请求 - 小包
	userQueryReq := &dto.BusinessReq{
		UserInfo: &dto.UserInfo{
			UserID:    "user_12345",
			Username:  "john_doe",
			Email:     "john@example.com",
			Phone:     "13800138000",
			IsVIP:     true,
			Status:    1,
		},
		QueryParam: &dto.QueryParam{
			PageNum:   1,
			PageSize:  10,
			SortBy:    "create_time",
			SortOrder: "desc",
		},
		RequestID:  "req_user_query_001",
		ClientType: "mobile",
		Version:    "1.0.0",
		Timestamp:  fixedTimestamp,
	}

	// 订单查询请求 - 小包
	orderQueryReq := &dto.BusinessReq{
		QueryParam: &dto.QueryParam{
			PageNum:   1,
			PageSize:  10,
			SortBy:    "create_time",
			SortOrder: "desc",
			Filters: map[string]string{
				"user_id": "user_12345",
				"status":  "1",
			},
		},
		RequestID:  "req_order_query_001",
		ClientType: "web",
		Version:    "1.0.0",
		Timestamp:  fixedTimestamp,
	}

	// 商品搜索请求 - 小包
	productSearchReq := &dto.BusinessReq{
		QueryParam: &dto.QueryParam{
			PageNum:    1,
			PageSize:   10,
			SortBy:     "sales",
			SortOrder:  "desc",
			Keywords:   "laptop",
			CategoryID: "cat_001",
			PriceMin:   100.0,
			PriceMax:   5000.0,
		},
		RequestID:  "req_product_search_001",
		ClientType: "mobile",
		Version:    "1.0.0",
		Timestamp:  fixedTimestamp,
	}

	// 创建订单请求 - 小包（含少量商品）
	order := &dto.OrderInfo{
		OrderID:        "order_67890",
		UserID:         "user_12345",
		OrderNo:        "ORD0000006789",
		TotalAmount:    599.99,
		PayAmount:      579.99,
		DiscountAmount: 20.00,
		Status:         1,
		PayMethod:      "alipay",
		PayTime:        baseTime.Add(-time.Hour * 2),
		DeliveryAddr:   "Beijing China",
		Remark:         "Fast delivery",
		CreateTime:     baseTime,
		UpdateTime:     baseTime,
	}

	products := []*dto.ProductInfo{
		dto.CreateTestProduct(1),
		dto.CreateTestProduct(2),
	}

	createOrderReq := &dto.BusinessReq{
		OrderInfo:  order,
		Products:   products,
		RequestID:  "req_create_order_001",
		TraceID:    "trace_12345",
		ClientType: "app",
		Version:    "2.1.0",
		Timestamp:  fixedTimestamp,
	}

	// 购物车请求 - 小包（少量商品）
	items := []dto.CartItemInfo{
		{ProductID: "prod_001", Name: "Mouse", Price: 49.99, Quantity: 1},
		{ProductID: "prod_002", Name: "Keyboard", Price: 199.99, Quantity: 1},
	}

	cartInfo := &dto.CartInfo{
		CartID:      "cart_98765",
		UserID:      "user_12345",
		Items:       items,
		TotalAmount: 249.98,
		UpdateTime:  baseTime,
	}

	cartReq := &dto.BusinessReq{
		CartInfo:   cartInfo,
		RequestID:  "req_cart_op_001",
		ClientType: "mobile",
		Version:    "1.5.0",
		Timestamp:  fixedTimestamp,
	}

	// 批量查询请求 - 小包
	batchQueryReq := &dto.BusinessReq{
		UserInfo: &dto.UserInfo{
			UserID: "user_12345",
		},
		QueryParam: &dto.QueryParam{
			PageNum:   1,
			PageSize:  10,
			SortBy:    "create_time",
			SortOrder: "desc",
			Filters: map[string]string{
				"user_id":    "user_12345",
				"status":     "1",
				"start_time": "2024-01-01",
				"end_time":   "2024-12-31",
			},
		},
		RequestID:  "req_batch_query_001",
		ClientType: "api",
		Version:    "2.0.0",
		Timestamp:  fixedTimestamp,
	}

	// 打印请求大小
	scenarios := []TestScenario{
		{
			Name:     "用户查询",
			Requests: []*dto.BusinessReq{userQueryReq},
			Execute:  comm.QueryUser,
		},
		{
			Name:     "订单查询",
			Requests: []*dto.BusinessReq{orderQueryReq},
			Execute:  comm.QueryOrders,
		},
		{
			Name:     "商品搜索",
			Requests: []*dto.BusinessReq{productSearchReq},
			Execute:  comm.SearchProducts,
		},
		{
			Name:     "创建订单",
			Requests: []*dto.BusinessReq{createOrderReq},
			Execute:  comm.CreateOrder,
		},
		{
			Name:     "获取购物车",
			Requests: []*dto.BusinessReq{cartReq},
			Execute:  comm.GetCart,
		},
		{
			Name:     "批量查询",
			Requests: []*dto.BusinessReq{batchQueryReq},
			Execute:  comm.BatchQuery,
		},
	}

	// 打印请求大小
	fmt.Println("\n========== 请求数据包大小 ==========")
	for _, s := range scenarios {
		reqBytes, _ := coder.Marshal(coder.Msgp, s.Requests[0])
		fmt.Printf("%s: %d bytes (%.2f KB)\n", s.Name, len(reqBytes), float64(len(reqBytes))/1024)
	}
	fmt.Println("=====================================")

	return scenarios
}

// 预热
func preWarm(scenarios []TestScenario) {
	for _, scenario := range scenarios {
		for i := 0; i < 100; i++ {
			req := scenario.Requests[i%len(scenario.Requests)]
			ctx := context.Background()
			_, _ = scenario.Execute(ctx, req)
		}
	}
	fmt.Println("预热完成")
}

// 更新统计信息
func updateStats(stats *PerformanceStats, err error, latency time.Duration, reqBytes, resBytes int) {
	atomic.AddInt64(&stats.TotalRequests, 1)
	atomic.AddInt64(&stats.TotalLatency, latency.Nanoseconds())
	atomic.AddInt64(&stats.TotalReqBytes, int64(reqBytes))
	atomic.AddInt64(&stats.TotalResBytes, int64(resBytes))

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
	totalReqBytes := atomic.LoadInt64(&stats.TotalReqBytes)
	totalResBytes := atomic.LoadInt64(&stats.TotalResBytes)

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

	// 计算数据传输量
	totalDataMB := float64(totalReqBytes+totalResBytes) / 1024 / 1024
	avgReqKB := float64(0)
	avgResKB := float64(0)
	if totalReq > 0 {
		avgReqKB = float64(totalReqBytes) / float64(totalReq) / 1024
		avgResKB = float64(totalResBytes) / float64(totalReq) / 1024
	}

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
	fmt.Printf("\n数据传输:\n")
	fmt.Printf("  请求数据: %.2f MB (平均 %.2f KB/请求)\n", float64(totalReqBytes)/1024/1024, avgReqKB)
	fmt.Printf("  响应数据: %.2f MB (平均 %.2f KB/响应)\n", float64(totalResBytes)/1024/1024, avgResKB)
	fmt.Printf("  总传输量: %.2f MB\n", totalDataMB)
	fmt.Printf("  吞吐量: %.2f MB/s\n", totalDataMB/elapsed.Seconds())
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
	totalReqBytes := atomic.LoadInt64(&stats.TotalReqBytes)
	totalResBytes := atomic.LoadInt64(&stats.TotalResBytes)

	elapsed := time.Since(stats.StartTime)
	qps := float64(totalReq) / elapsed.Seconds()

	fmt.Printf("\n总请求统计:\n")
	fmt.Printf("  总请求数: %d\n", totalReq)
	fmt.Printf("  成功请求: %d\n", successReq)
	fmt.Printf("  失败请求: %d\n", failedReq)
	fmt.Printf("  成功率: %.2f%%\n", float64(successReq)/float64(totalReq)*100)

	avgReqKB := float64(0)
	avgResKB := float64(0)
	if totalReq > 0 {
		avgReqKB = float64(totalReqBytes) / float64(totalReq) / 1024
		avgResKB = float64(totalResBytes) / float64(totalReq) / 1024
	}

	fmt.Printf("\n数据包统计:\n")
	fmt.Printf("  平均请求包大小: %.2f KB\n", avgReqKB)
	fmt.Printf("  平均响应包大小: %.2f KB\n", avgResKB)
	fmt.Printf("  总传输数据: %.2f MB\n", float64(totalReqBytes+totalResBytes)/1024/1024)

	fmt.Printf("\n最终性能:\n")
	fmt.Printf("  平均QPS: %.2f req/s\n", qps)
	fmt.Printf("  吞吐量: %.2f MB/s\n", float64(totalReqBytes+totalResBytes)/1024/1024/elapsed.Seconds())

	// 性能评级
	if qps > 100000 {
		fmt.Printf("\n性能评级: 超超高性能 (>100K QPS)\n")
	} else if qps > 50000 {
		fmt.Printf("\n性能评级: 超高性能 (50K-100K QPS)\n")
	} else if qps > 20000 {
		fmt.Printf("\n性能评级: 高性能 (20K-50K QPS)\n")
	} else if qps > 10000 {
		fmt.Printf("\n性能评级: 中高性能 (10K-20K QPS)\n")
	} else {
		fmt.Printf("\n性能评级: 标准 (<10K QPS)\n")
	}

	// 建议
	fmt.Printf("\n优化建议:\n")
	if qps < 50000 {
		fmt.Printf("  - 检查网络带宽是否充足\n")
		fmt.Printf("  - 检查服务端 CPU 使用率\n")
	}
	fmt.Printf("  - 监控GC频率，考虑调整GOGC参数\n")
}
