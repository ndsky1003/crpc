package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	_ "net/http/pprof" // 自动注册 pprof handler

	"github.com/ndsky1003/crpc/v3"
	"github.com/ndsky1003/crpc/v3/client"
	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/log"
)

// 压力测试统计
type StressStats struct {
	TotalSent     atomic.Int64
	TotalReceived atomic.Int64
	TotalErrors   atomic.Int64

	// 延迟统计 (微秒)
	MinLatency   atomic.Int64
	MaxLatency   atomic.Int64
	TotalLatency atomic.Int64

	// 数据包大小统计 (字节)
	TotalReqBytes atomic.Int64 // 请求总字节数
	TotalResBytes atomic.Int64 // 响应总字节数

	LastSummary  time.Time
	lastSent     int64
	lastReceived int64
	lastErrs     int64
	lastReqBytes int64
	lastResBytes int64

	mu           sync.Mutex
	inflightReqs sync.Map // map[int64]time.Time (requestID -> startTime)
}

var stats = &StressStats{
	LastSummary: time.Now(),
}

// 初始化 MinLatency 为一个大值
func init() {
	stats.MinLatency.Store(1<<63 - 1) // max int64
}

func main() {
	log.SetDefault(log.Options().SetAddSource(true).SetLevel(log.LevelInfo))

	slog.Info("=== Call 压力测试 - Caller 启动 ===")

	// 启动 pprof HTTP 服务器(可选,用于性能分析)
	go func() {
		slog.Info("pprof HTTP server started", "addr", "localhost:6062")
		slog.Error("pprof server error", "err", http.ListenAndServe("localhost:6062", nil))
	}()

	// 使用多核
	numCPU := runtime.NumCPU()
	runtime.GOMAXPROCS(numCPU)
	slog.Info("CPU 核心数", "cores", numCPU)

	ctx := context.Background()

	// 创建 Client 作为调用者
	cli, err := crpc.Dial(ctx, "call_stress_caller", ":9092",
		client.Options().SetSecret("call_stress_secret_123456"))
	if err != nil {
		slog.Error("Failed to dial", "error", err)
		return
	}
	defer cli.Close()

	slog.Info("Caller 已连接到 Server")

	// 等待 Provider 注册服务
	slog.Info("等待 Provider 注册服务...")
	time.Sleep(2 * time.Second)
	slog.Info("开始压力测试")

	// 启动压力测试 - 多并发跑满 CPU
	go runStressTest(ctx, cli, numCPU)

	// 启动统计报告(每5分钟打印一次)
	go reportStats()

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("=== 测试结束 ===")
	printFinalStats()
}

// runStressTest 运行多并发压力测试
func runStressTest(ctx context.Context, cli *client.Client, numWorkers int) {
	var globalRequestID atomic.Int64
	globalRequestID.Store(0)

	numWorkers *= 2
	// 启动多个并发 worker
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			workerStressTest(ctx, cli, workerID, &globalRequestID)
		}(i)
	}

	slog.Info("所有 worker 已启动", "workers", numWorkers)
	wg.Wait()
}

// workerStressTest 单个 worker 的压力测试逻辑
func workerStressTest(ctx context.Context, cli *client.Client, workerID int, globalRequestID *atomic.Int64) {
	// 支付方式列表
	paymentTypes := []string{"alipay", "wechat", "card", "balance"}

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// 获取全局请求 ID
			requestID := globalRequestID.Add(1)
			stats.TotalSent.Add(1)

			// 创建新的请求对象(不使用对象池,确保数据独立性)
			req := &OrderReq{}

			// 生成真实的订单数据
			req.OrderID = fmt.Sprintf("ORD%d", requestID)
			req.UserID = fmt.Sprintf("USER%d", requestID%10000) // 模拟10000个用户
			req.Amount = (requestID % 10000) * 100              // 订单金额(分),模拟1-10000元

			// 1000次后增加随机性
			if requestID > 1000 {
				// 随机商品数量 (1-5个)
				productCount := int(requestID%5) + 1
				req.ProductIDs = make([]string, productCount)
				for i := 0; i < productCount; i++ {
					req.ProductIDs[i] = fmt.Sprintf("PROD%d", int64(i)*1000+(requestID%1000)+1)
				}

				// 随机元数据数量 (3-8个)
				metaCount := int(requestID%6) + 3
				req.Metadata = make([]string, metaCount)
				req.Metadata[0] = fmt.Sprintf("channel:%d", requestID%10)
				req.Metadata[1] = fmt.Sprintf("source:%d", requestID%8)
				req.Metadata[2] = fmt.Sprintf("device:%s", []string{"mobile", "tablet", "desktop"}[requestID%3])
				for i := 3; i < metaCount; i++ {
					// 生成随机的额外元数据,控制长度不超过1KB
					extraLen := int(requestID % 50) // 随机长度0-49
					extraValue := make([]byte, extraLen)
					for j := range extraValue {
						extraValue[j] = byte('a' + (requestID+int64(j))%26)
					}
					req.Metadata[i] = fmt.Sprintf("meta%d:%s", i, string(extraValue))
				}

				// 随机收货信息长度(控制在1KB以内)
				addrLen := int(requestID % 200) // 随机地址长度0-199
				randomAddr := make([]byte, addrLen)
				for i := range randomAddr {
					randomAddr[i] = byte('a' + (requestID+int64(i))%26)
				}
				req.ReceiverInfo = fmt.Sprintf(`{"name":"用户%d","phone":"138%08d","address":"%s","postcode":"%d"}`,
					requestID%1000,
					requestID%100000000,
					string(randomAddr),
					requestID%1000000)
			} else {
				// 前1000次使用固定数据
				req.ProductIDs = make([]string, 3)
				req.ProductIDs[0] = fmt.Sprintf("PROD%d", (requestID%1000)+1)
				req.ProductIDs[1] = fmt.Sprintf("PROD%d", (requestID%1000)+1001)
				req.ProductIDs[2] = fmt.Sprintf("PROD%d", (requestID%1000)+2001)

				req.Metadata = make([]string, 5)
				req.Metadata[0] = fmt.Sprintf("channel:%d", requestID%5)
				req.Metadata[1] = fmt.Sprintf("source:%d", requestID%3)
				req.Metadata[2] = "device:mobile"
				req.Metadata[3] = fmt.Sprintf("version:2.0.%d", requestID%10)
				req.Metadata[4] = fmt.Sprintf("region:%d", requestID%20)

				req.ReceiverInfo = `{"name":"测试用户","phone":"13800138000","address":"北京市朝阳区测试路123号"}`
			}

			// 订单状态
			req.Status = "PENDING"

			// 随机支付方式
			req.PaymentType = paymentTypes[requestID%int64(len(paymentTypes))]

			req.Timestamp = time.Now().UnixMicro()

			// 序列化请求以获取实际数据包大小
			reqBytes, _ := coder.Marshal(coder.Msgp, req)

			// 记录请求开始时间
			startTime := time.Now()
			stats.inflightReqs.Store(requestID, startTime)

			// 发起 Call 调用
			var res OrderRes
			err := cli.Call(ctx, "call_stress_provider", "call_stress.ProcessOrder", req, &res,
				client.Options().
					SetReqCoderT(coder.Msgp).
					SetResCoderT(coder.Msgp).
					SetDebug(false), // 压力测试关闭 debug 日志
			)

			if err != nil {
				stats.TotalErrors.Add(1)
				stats.inflightReqs.Delete(requestID)
				continue
			}

			// 序列化响应以获取实际数据包大小
			resBytes, _ := coder.Marshal(coder.Msgp, &res)

			// 成功接收响应
			stats.TotalReceived.Add(1)

			// 统计数据包大小
			stats.TotalReqBytes.Add(int64(len(reqBytes)))
			stats.TotalResBytes.Add(int64(len(resBytes)))

			// 计算延迟
			elapsed := time.Since(startTime).Microseconds()
			stats.updateLatency(elapsed)

			// 清理
			stats.inflightReqs.Delete(requestID)
		}
	}
}

// updateLatency 更新延迟统计
func (s *StressStats) updateLatency(latency int64) {
	// 更新最小延迟
	for {
		min := s.MinLatency.Load()
		if latency >= min {
			break
		}
		if s.MinLatency.CompareAndSwap(min, latency) {
			break
		}
	}

	// 更新最大延迟
	for {
		max := s.MaxLatency.Load()
		if latency <= max {
			break
		}
		if s.MaxLatency.CompareAndSwap(max, latency) {
			break
		}
	}

	// 累加总延迟用于计算平均值
	s.TotalLatency.Add(latency)
}

// reportStats 报告统计信息(每5分钟)
func reportStats() {
	ticker := time.NewTicker(5 * time.Minute) // 每 5 分钟报告一次
	defer ticker.Stop()

	for range ticker.C {
		printStats()
	}
}

func printStats() {
	sent := stats.TotalSent.Load()
	received := stats.TotalReceived.Load()
	errs := stats.TotalErrors.Load()
	reqBytes := stats.TotalReqBytes.Load()
	resBytes := stats.TotalResBytes.Load()
	elapsed := time.Since(stats.LastSummary)

	// 计算增量
	deltaSent := sent - stats.lastSent
	deltaReceived := received - stats.lastReceived
	deltaErrs := errs - stats.lastErrs
	deltaReqBytes := reqBytes - stats.lastReqBytes
	deltaResBytes := resBytes - stats.lastResBytes

	// 保存当前值作为下次的基准
	stats.lastSent = sent
	stats.lastReceived = received
	stats.lastErrs = errs
	stats.lastReqBytes = reqBytes
	stats.lastResBytes = resBytes

	minLat := stats.MinLatency.Load()
	maxLat := stats.MaxLatency.Load()
	totalLat := stats.TotalLatency.Load()
	avgLat := int64(0)
	if deltaReceived > 0 {
		avgLat = totalLat / received // 使用累计计算平均
	}

	// 计算平均数据包大小(使用增量数据)
	avgReqSize := int64(0)
	avgResSize := int64(0)
	if deltaReceived > 0 {
		avgReqSize = deltaReqBytes / deltaReceived
		avgResSize = deltaResBytes / deltaReceived
	}

	// 计算吞吐量(使用增量数据)
	throughput := float64(deltaSent) / elapsed.Seconds()
	throughputMB := (float64(deltaReqBytes+deltaResBytes) / 1024 / 1024) / elapsed.Seconds()

	// 计算成功率
	totalAttempts := deltaReceived + deltaErrs
	successRate := 0.0
	if totalAttempts > 0 {
		successRate = float64(deltaReceived) / float64(totalAttempts) * 100
	}

	// 获取运行时指标
	numGoroutine := runtime.NumGoroutine()
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	slog.Info("=== 5分钟统计 ===",
		"发送", deltaSent,
		"接收", deltaReceived,
		"错误", deltaErrs,
		"吞吐量", fmt.Sprintf("%.0f req/s", throughput),
		"吞吐量(MB)", fmt.Sprintf("%.2f MB/s", throughputMB),
		"延迟(微秒)", fmt.Sprintf("min=%d max=%d avg=%d", minLat, maxLat, avgLat),
		"数据包大小", fmt.Sprintf("req=%dB res=%dB", avgReqSize, avgResSize),
		"成功率", fmt.Sprintf("%.2f%%", successRate),
		"goroutine", numGoroutine,
		"内存(堆)", fmt.Sprintf("%.1fMB", float64(memStats.HeapAlloc)/1024/1024),
		"内存(系统)", fmt.Sprintf("%.1fMB", float64(memStats.Sys)/1024/1024),
		"GC次数", memStats.NumGC,
		"GC暂停(总)", fmt.Sprintf("%.2fms", float64(memStats.PauseTotalNs)/1000000),
		"累计发送", sent,
		"累计接收", received,
		"累计错误", errs,
		"累计流量", fmt.Sprintf("%.2fMB", float64(reqBytes+resBytes)/1024/1024),
	)

	stats.LastSummary = time.Now()
}

func printFinalStats() {
	sent := stats.TotalSent.Load()
	received := stats.TotalReceived.Load()
	errs := stats.TotalErrors.Load()
	reqBytes := stats.TotalReqBytes.Load()
	resBytes := stats.TotalResBytes.Load()

	minLat := stats.MinLatency.Load()
	maxLat := stats.MaxLatency.Load()
	totalLat := stats.TotalLatency.Load()
	avgLat := int64(0)
	if received > 0 {
		avgLat = totalLat / received
	}

	// 计算平均数据包大小
	avgReqSize := int64(0)
	avgResSize := int64(0)
	if received > 0 {
		avgReqSize = reqBytes / received
		avgResSize = resBytes / received
	}

	// 获取运行时指标
	numGoroutine := runtime.NumGoroutine()
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// 计算成功率
	totalAttempts := received + errs
	successRate := 0.0
	if totalAttempts > 0 {
		successRate = float64(received) / float64(totalAttempts) * 100
	}

	// 计算总体吞吐量
	totalElapsed := time.Since(stats.LastSummary)
	throughput := 0.0
	throughputMB := 0.0
	if totalElapsed.Seconds() > 0 {
		throughput = float64(sent) / totalElapsed.Seconds()
		throughputMB = (float64(reqBytes+resBytes) / 1024 / 1024) / totalElapsed.Seconds()
	}

	slog.Info("=== 最终统计 ===",
		"累计发送", sent,
		"累计接收", received,
		"累计错误", errs,
		"总体吞吐量", fmt.Sprintf("%.0f req/s", throughput),
		"总体吞吐量(MB)", fmt.Sprintf("%.2f MB/s", throughputMB),
		"延迟(微秒)", fmt.Sprintf("min=%d max=%d avg=%d", minLat, maxLat, avgLat),
		"数据包大小", fmt.Sprintf("req=%dB res=%dB", avgReqSize, avgResSize),
		"成功率", fmt.Sprintf("%.2f%%", successRate),
		"总流量", fmt.Sprintf("%.2fMB", float64(reqBytes+resBytes)/1024/1024),
		"goroutine", numGoroutine,
		"内存(堆)", fmt.Sprintf("%.1fMB", float64(memStats.HeapAlloc)/1024/1024),
		"内存(系统)", fmt.Sprintf("%.1fMB", float64(memStats.Sys)/1024/1024),
		"GC次数", memStats.NumGC,
		"GC暂停(总)", fmt.Sprintf("%.2fms", float64(memStats.PauseTotalNs)/1000000),
	)
}
