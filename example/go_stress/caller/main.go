package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ndsky1003/crpc/v3"
	"github.com/ndsky1003/crpc/v3/client"
	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/log"
)

// 对象池,复用 OrderReq
var reqPool = sync.Pool{
	New: func() any {
		return &OrderReq{
			ProductIDs:   make([]string, 5),
			Metadata:     make([]string, 8),
			PaymentType:  "alipay",
			ReceiverInfo: `{"name":"测试用户","phone":"13800138000","address":"北京市朝阳区测试路123号"}`,
		}
	},
}

// 压力测试统计
type StressStats struct {
	TotalSent      atomic.Int64
	TotalErrors    atomic.Int64
	MinLatency     atomic.Int64
	MaxLatency     atomic.Int64
	TotalLatency   atomic.Int64
	TotalReqBytes  atomic.Int64
	LastSummary    time.Time
	lastSent       int64
	lastErrs       int64
}

var stats = &StressStats{
	LastSummary: time.Now(),
}

func init() {
	stats.MinLatency.Store(1<<63 - 1)
}

func main() {
	log.SetDefault(log.Options().SetAddSource(true).SetLevel(log.LevelInfo))

	slog.Info("=== Go 方法压力测试 - Caller 启动 ===")

	// 使用多核
	numCPU := runtime.NumCPU()
	runtime.GOMAXPROCS(numCPU)
	slog.Info("CPU 核心数", "cores", numCPU)

	ctx := context.Background()

	// 创建 Client 作为调用者
	cli, err := crpc.Dial(ctx, "go_stress_caller", ":9093",
		client.Options().SetSecret("go_stress_secret_123456"))
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

	// 启动压力测试
	go runStressTest(ctx, cli, numCPU*2)

	// 启动统计报告
	go reportStats()

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("=== 测试结束 ===")
	printFinalStats()
}

func runStressTest(ctx context.Context, cli *client.Client, numWorkers int) {
	var globalRequestID atomic.Int64
	globalRequestID.Store(0)

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

func workerStressTest(ctx context.Context, cli *client.Client, workerID int, globalRequestID *atomic.Int64) {
	paymentTypes := []string{"alipay", "wechat", "card", "balance"}

	for {
		select {
		case <-ctx.Done():
			return
		default:
			requestID := globalRequestID.Add(1)
			stats.TotalSent.Add(1)

			req := reqPool.Get().(*OrderReq)
			req.OrderID = fmt.Sprintf("ORD%d", requestID)
			req.UserID = fmt.Sprintf("USER%d", requestID%10000)
			req.Amount = (requestID % 10000) * 100

			// 1000次后增加随机性
			if requestID > 1000 {
				productCount := int(requestID%5) + 1
				if cap(req.ProductIDs) >= productCount {
					req.ProductIDs = req.ProductIDs[:productCount]
					for i := 0; i < productCount; i++ {
						req.ProductIDs[i] = fmt.Sprintf("PROD%d", int64(i)*1000+(requestID%1000)+1)
					}
				}

				metaCount := int(requestID%6) + 3
				if cap(req.Metadata) >= metaCount {
					req.Metadata = req.Metadata[:metaCount]
					req.Metadata[0] = fmt.Sprintf("channel:%d", requestID%10)
					req.Metadata[1] = fmt.Sprintf("source:%d", requestID%8)
					req.Metadata[2] = fmt.Sprintf("device:%s", []string{"mobile", "tablet", "desktop"}[requestID%3])
					for i := 3; i < metaCount; i++ {
						extraLen := int(requestID % 50)
						extraValue := make([]byte, extraLen)
						for j := range extraValue {
							extraValue[j] = byte('a' + (requestID+int64(j))%26)
						}
						req.Metadata[i] = fmt.Sprintf("meta%d:%s", i, string(extraValue))
					}
				}

				addrLen := int(requestID % 200)
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
				req.ProductIDs = req.ProductIDs[:3]
				req.ProductIDs[0] = fmt.Sprintf("PROD%d", (requestID%1000)+1)
				req.ProductIDs[1] = fmt.Sprintf("PROD%d", (requestID%1000)+1001)
				req.ProductIDs[2] = fmt.Sprintf("PROD%d", (requestID%1000)+2001)

				req.Metadata = req.Metadata[:5]
				req.Metadata[0] = fmt.Sprintf("channel:%d", requestID%5)
				req.Metadata[1] = fmt.Sprintf("source:%d", requestID%3)
				req.Metadata[2] = fmt.Sprintf("device:mobile")
				req.Metadata[3] = fmt.Sprintf("version:2.0.%d", requestID%10)
				req.Metadata[4] = fmt.Sprintf("region:%d", requestID%20)

				req.ReceiverInfo = `{"name":"测试用户","phone":"13800138000","address":"北京市朝阳区测试路123号"}`
			}

			req.Status = "PENDING"
			req.PaymentType = paymentTypes[requestID%int64(len(paymentTypes))]
			req.Timestamp = time.Now().UnixMicro()

			// 序列化请求以获取实际数据包大小
			reqBytes, _ := coder.Marshal(coder.Msgp, req)

			startTime := time.Now()

			// 使用 Go 方法调用 - 异步不等待返回值
			err := cli.Go(ctx, "go_stress_provider", "go_stress.ProcessOrder", req,
				client.Options().
					SetReqCoderT(coder.Msgp).
					SetDebug(false),
			)

			elapsed := time.Since(startTime).Microseconds()
			stats.updateLatency(elapsed)

			stats.TotalReqBytes.Add(int64(len(reqBytes)))

			if err != nil {
				stats.TotalErrors.Add(1)
			}

			reqPool.Put(req)
		}
	}
}

func (s *StressStats) updateLatency(latency int64) {
	for {
		min := s.MinLatency.Load()
		if latency >= min {
			break
		}
		if s.MinLatency.CompareAndSwap(min, latency) {
			break
		}
	}

	for {
		max := s.MaxLatency.Load()
		if latency <= max {
			break
		}
		if s.MaxLatency.CompareAndSwap(max, latency) {
			break
		}
	}

	s.TotalLatency.Add(latency)
}

func reportStats() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		printStats()
	}
}

func printStats() {
	sent := stats.TotalSent.Load()
	errs := stats.TotalErrors.Load()
	reqBytes := stats.TotalReqBytes.Load()
	elapsed := time.Since(stats.LastSummary)

	deltaSent := sent - stats.lastSent
	deltaErrs := errs - stats.lastErrs

	stats.lastSent = sent
	stats.lastErrs = errs

	minLat := stats.MinLatency.Load()
	maxLat := stats.MaxLatency.Load()
	totalLat := stats.TotalLatency.Load()
	avgLat := int64(0)
	if deltaSent > 0 {
		avgLat = totalLat / sent
	}

	avgReqSize := int64(0)
	if sent > 0 {
		avgReqSize = reqBytes / sent
	}

	throughput := float64(deltaSent) / elapsed.Seconds()
	throughputMB := (float64(reqBytes) / 1024 / 1024) / elapsed.Seconds()

	numGoroutine := runtime.NumGoroutine()
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	slog.Info("=== 5分钟统计 (Go方法) ===",
		"发送", deltaSent,
		"错误", deltaErrs,
		"吞吐量", fmt.Sprintf("%.0f req/s", throughput),
		"吞吐量(MB)", fmt.Sprintf("%.2f MB/s", throughputMB),
		"延迟(微秒)", fmt.Sprintf("min=%d max=%d avg=%d", minLat, maxLat, avgLat),
		"数据包大小", fmt.Sprintf("req=%dB", avgReqSize),
		"goroutine", numGoroutine,
		"内存(堆)", fmt.Sprintf("%.1fMB", float64(memStats.HeapAlloc)/1024/1024),
		"内存(系统)", fmt.Sprintf("%.1fMB", float64(memStats.Sys)/1024/1024),
		"GC次数", memStats.NumGC,
		"GC暂停(总)", fmt.Sprintf("%.2fms", float64(memStats.PauseTotalNs)/1000000),
		"累计发送", sent,
		"累计错误", errs,
		"累计流量", fmt.Sprintf("%.2fMB", float64(reqBytes)/1024/1024),
	)

	stats.LastSummary = time.Now()
}

func printFinalStats() {
	sent := stats.TotalSent.Load()
	errs := stats.TotalErrors.Load()
	reqBytes := stats.TotalReqBytes.Load()

	minLat := stats.MinLatency.Load()
	maxLat := stats.MaxLatency.Load()
	totalLat := stats.TotalLatency.Load()
	avgLat := int64(0)
	if sent > 0 {
		avgLat = totalLat / sent
	}

	avgReqSize := int64(0)
	if sent > 0 {
		avgReqSize = reqBytes / sent
	}

	numGoroutine := runtime.NumGoroutine()
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	totalElapsed := time.Since(stats.LastSummary)
	throughput := 0.0
	throughputMB := 0.0
	if totalElapsed.Seconds() > 0 {
		throughput = float64(sent) / totalElapsed.Seconds()
		throughputMB = (float64(reqBytes) / 1024 / 1024) / totalElapsed.Seconds()
	}

	slog.Info("=== 最终统计 (Go方法) ===",
		"累计发送", sent,
		"累计错误", errs,
		"总体吞吐量", fmt.Sprintf("%.0f req/s", throughput),
		"总体吞吐量(MB)", fmt.Sprintf("%.2f MB/s", throughputMB),
		"延迟(微秒)", fmt.Sprintf("min=%d max=%d avg=%d", minLat, maxLat, avgLat),
		"数据包大小", fmt.Sprintf("req=%dB", avgReqSize),
		"总流量", fmt.Sprintf("%.2fMB", float64(reqBytes)/1024/1024),
		"goroutine", numGoroutine,
		"内存(堆)", fmt.Sprintf("%.1fMB", float64(memStats.HeapAlloc)/1024/1024),
		"内存(系统)", fmt.Sprintf("%.1fMB", float64(memStats.Sys)/1024/1024),
		"GC次数", memStats.NumGC,
		"GC暂停(总)", fmt.Sprintf("%.2fms", float64(memStats.PauseTotalNs)/1000000),
	)
}
