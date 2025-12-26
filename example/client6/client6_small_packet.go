//go:build small
// +build small

package main

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof" // 导入pprof
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ndsky1003/crpc/v3/client"
	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/crpc/v3/example/comm"
	"github.com/ndsky1003/crpc/v3/example/dto"
)

// 性能统计
type PerformanceStats struct {
	TotalRequests   int64            `json:"total_requests"`
	SuccessRequests int64            `json:"success_requests"`
	FailedRequests  int64            `json:"failed_requests"`
	TotalLatency    int64            `json:"total_latency_ns"`
	MinLatency      int64            `json:"min_latency_ns"`
	MaxLatency      int64            `json:"max_latency_ns"`
	StartTime       time.Time        `json:"start_time"`
	TotalReqBytes   int64            `json:"total_req_bytes"`
	TotalResBytes   int64            `json:"total_res_bytes"`
	LastGCStats     runtime.MemStats `json:"last_gc_stats"` // 上次GC统计
}

// 测试场景
type TestScenario struct {
	Name     string
	Requests []*dto.BusinessReq
	Execute  func(context.Context, *dto.BusinessReq, ...*client.Option) (*dto.BusinessRes, error)
	ResBytes int // 预计算的响应大小(字节)
	ReqBytes int // 预计算的请求大小(字节)
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

	// 【GC分析】添加pprof支持,用于内存分析
	// 使用方法:
	//   1. 运行测试后,在另一个终端执行:
	//      curl http://localhost:6060/debug/pprof/heap > heap.prof
	//   2. 分析堆内存:
	//      go tool pprof heap.prof
	//      (pprof) top           # 查看内存分配最多的函数
	//      (pprof) list <func>   # 查看具体函数的内存分配
	//      (pprof) web           # 生成可视化图(需要graphviz)
	go func() {
		fmt.Println("pprof HTTP server started on :6060")
		fmt.Println("获取内存profile: curl http://localhost:6060/debug/pprof/heap > heap.prof")
		fmt.Println("获取分配profile: curl http://localhost:6060/debug/pprof/allocs > allocs.prof")
		if err := http.ListenAndServe("localhost:6060", nil); err != nil {
			fmt.Printf("pprof server error: %v\n", err)
		}
	}()

	// 初始化客户端
	c, err := client.Dial(context.Background(), "client6_small", ":8080",
		client.Options().SetSecret("ddddd").
			SetMetaCoderT(coder.Msgp).
			SetReqCoderT(coder.Msgp).
			SetResCoderT(coder.Msgp))
	// 【GC优化】移除TraceID相关配置,压测场景不需要链路追踪
	// SetWithTraceID(func(ctx context.Context, tid string) context.Context {
	//     return trace.WithTraceID(ctx, tid)
	// }).SetGenTraceID(func(ctx context.Context) string {
	//     return trace.ExtractorTraceID(ctx)
	// }))
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

	// 启动内存分配监控协程(每秒监控一次)
	stopMemMonitor := make(chan bool)
	go memoryAllocationMonitor(stopMemMonitor)

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

					// 使用预计算的请求和响应大小,避免频繁序列化
					if res == nil {
						// 如果响应为nil,响应大小记为0
						updateStats(stats, err, latency, scenario.ReqBytes, 0)
					} else {
						// 使用预计算的响应大小
						updateStats(stats, err, latency, scenario.ReqBytes, scenario.ResBytes)
					}
				}
			}
		}(i)
	}

	// 等待测试完成
	wg.Wait()
	cancel()
	close(stopMonitor)
	close(stopMemMonitor)

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
			UserID:   "user_12345",
			Username: "john_doe",
			Email:    "john@example.com",
			Phone:    "13800138000",
			IsVIP:    true,
			Status:   1,
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

	// 创建测试场景
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

	// 预计算请求和响应大小,避免在测试循环中频繁序列化
	fmt.Println("\n========== 请求数据包大小 ==========")
	for i := range scenarios {
		// 预计算请求大小
		reqBytes, _ := coder.Marshal(coder.Msgp, scenarios[i].Requests[0])
		scenarios[i].ReqBytes = len(reqBytes)

		// 执行一次请求获取响应,并预计算响应大小
		ctx := context.Background()
		res, err := scenarios[i].Execute(ctx, scenarios[i].Requests[0])
		if err != nil {
			fmt.Printf("警告: %s 预执行失败: %v\n", scenarios[i].Name, err)
			scenarios[i].ResBytes = 0
		} else {
			resBytes, _ := coder.Marshal(coder.Msgp, res)
			scenarios[i].ResBytes = len(resBytes)
		}

		fmt.Printf("%s: 请求=%d bytes (%.2f KB), 响应=%d bytes (%.2f KB)\n",
			scenarios[i].Name,
			scenarios[i].ReqBytes,
			float64(scenarios[i].ReqBytes)/1024,
			scenarios[i].ResBytes,
			float64(scenarios[i].ResBytes)/1024)
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

// 内存分配监控器(每秒监控一次,用于定位GC热点)
func memoryAllocationMonitor(stop chan bool) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var lastM runtime.MemStats
	runtime.ReadMemStats(&lastM)

	count := 0
	totalAlloc := 0.0
	totalGC := uint32(0)

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			count++
			var m runtime.MemStats
			runtime.ReadMemStats(&m)

			// 计算每秒的内存分配和GC
			allocDiff := float64(m.TotalAlloc-lastM.TotalAlloc) / 1024 / 1024
			numGC := m.NumGC - lastM.NumGC

			totalAlloc += allocDiff
			totalGC += numGC

			// 每10秒打印一次详细信息和平均值
			if count%10 == 0 {
				avgAlloc := totalAlloc / 10.0
				avgGC := float64(totalGC) / 10.0
				fmt.Printf("\n[内存监控 %ds] 近10秒平均: 每秒分配 %.2f MB/秒, GC %.1f 次/秒 | 本次: %.2f MB, %d GC\n",
					count,
					avgAlloc,
					avgGC,
					allocDiff,
					numGC)

				// 重置累计
				totalAlloc = 0
				totalGC = 0
			} else {
				// 非报告周期，简单打印当前秒
				fmt.Printf("[内存监控 %ds] 分配: %.2f MB, GC: %d 次\n",
					count, allocDiff, numGC)
			}

			lastM = m
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
	fmt.Printf("  GC频率: %.2f 次/秒\n", float64(m.NumGC)/elapsed.Seconds())

	// 计算GC相关的详细指标
	gcPauseTotal := time.Duration(m.PauseTotalNs)
	if m.NumGC > 0 {
		avgGCPause := gcPauseTotal / time.Duration(m.NumGC)
		fmt.Printf("  GC总暂停时间: %v\n", gcPauseTotal.Round(time.Millisecond))
		fmt.Printf("  平均GC暂停: %v\n", avgGCPause.Round(time.Microsecond))
		fmt.Printf("  最后GC暂停: %v\n", time.Duration(m.PauseNs[(m.NumGC+255)%256]).Round(time.Microsecond))
	}

	// 计算从上次报告以来的GC增量
	if reportCount > 1 && m.NumGC > stats.LastGCStats.NumGC {
		gcDelta := m.NumGC - stats.LastGCStats.NumGC
		gcRate := float64(gcDelta) / 300.0 // 5分钟=300秒
		fmt.Printf("  本报告周期GC增量: %d (%.2f 次/秒)\n", gcDelta, gcRate)
	}

	// 保存本次GC统计供下次对比
	stats.LastGCStats = m

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
