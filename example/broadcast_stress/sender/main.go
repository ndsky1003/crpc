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

	"github.com/ndsky1003/crpc/v3"
	"github.com/ndsky1003/crpc/v3/client"
	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/crpc/v3/example/trace"
	"github.com/ndsky1003/log"
	_ "net/http/pprof" // 自动注册 pprof handler
)

// 对象池，复用 OrderReq
var reqPool = sync.Pool{
	New: func() any {
		return &OrderReq{
			ProductIDs:   make([]string, 3), // 假设每个订单3个商品
			Metadata:     make([]string, 5), // 5个元数据字段
			PaymentType:  "alipay",
			ReceiverInfo: `{"name":"test","phone":"13800138000","address":"beijing"}`,
		}
	},
}

// 压力测试统计
type StressStats struct {
	TotalSent       atomic.Int64
	TotalReceived   atomic.Int64
	TotalErrors     atomic.Int64
	TotalEOS        atomic.Int64
	TotalLocal      atomic.Int64
	TotalReceiver1  atomic.Int64
	TotalReceiver2  atomic.Int64

	// 延迟统计 (微秒)
	MinLatency      atomic.Int64
	MaxLatency      atomic.Int64
	TotalLatency    atomic.Int64

	LastSummary     time.Time
	lastSent        int64
	lastReceived    int64
	lastErrs        int64
	lastEOS         int64
	mu              sync.Mutex
	missingSeqs     []int64
	inflightReqs    sync.Map // map[int64]time.Time (counter -> startTime)
}

var stats = &StressStats{
	LastSummary: time.Now(),
	MinLatency:  atomic.Int64{},
}

// 初始化 MinLatency 为一个大值
func init() {
	stats.MinLatency.Store(1<<63 - 1) // max int64
}

func main() {
	log.SetDefault(log.Options().SetExtractorAttr(func(ctx context.Context, r *slog.Record) {
		if tid := trace.ExtractorTraceID(ctx); tid != "" {
			r.Add("trace_id", tid)
		}
	}).SetAddSource(true).SetLevel(log.LevelInfo))

	slog.Info("=== 广播压力测试启动 ===")

	// 启动 pprof HTTP 服务器（可选，用于性能分析）
	go func() {
		slog.Info("pprof HTTP server started", "addr", "localhost:6060")
		slog.Error("pprof server error", "err", http.ListenAndServe("localhost:6060", nil))
	}()

	// 使用多核
	numCPU := runtime.NumCPU()
	runtime.GOMAXPROCS(numCPU)
	slog.Info("CPU 核心数", "cores", numCPU)

	ctx := context.Background()

	// 创建客户端
	cli, err := crpc.Dial(ctx, "broadcast_stress", ":9091",
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

	// 注册本地服务
	localSvc := &OrderService{Name: "sender_local"}
	if err := cli.RegisterName("broadcast_stress", localSvc); err != nil {
		slog.Error("Failed to register service", "error", err)
		return
	}

	slog.Info("Sender client started, registered local service")

	// 等待其他接收者连接建立
	slog.Info("等待其他接收者连接...")
	time.Sleep(3 * time.Second)
	slog.Info("开始压力测试")

	// 启动压力测试 - 多并发跑满 CPU
	go runStressTest(ctx, cli, numCPU)

	// 启动统计报告
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
	var globalCounter atomic.Int64
	globalCounter.Store(0)

	// 准备 1KB 的 payload
	payload := make([]byte, 1024)
	for i := range payload {
		payload[i] = byte(i % 256)
	}

	// 启动多个并发 worker
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			workerStressTest(ctx, cli, workerID, &globalCounter, payload)
		}(i)
	}

	slog.Info("所有 worker 已启动", "workers", numWorkers)
	wg.Wait()
}

// workerStressTest 单个 worker 的压力测试逻辑
func workerStressTest(ctx context.Context, cli *client.Client, workerID int, globalCounter *atomic.Int64, payload []byte) {
	// 每个 worker 自己的响应计数器
	localResponseCounts := make(map[int64]int64)
	var localMu sync.Mutex

	// 定期清理 map，防止无限增长
	cleanupTicker := time.NewTicker(10 * time.Second)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-cleanupTicker.C:
			// 定期清理已完成的请求记录
			localMu.Lock()
			for k := range localResponseCounts {
				delete(localResponseCounts, k)
			}
			localMu.Unlock()
		default:
			// 获取全局计数器
			counter := globalCounter.Add(1)
			stats.TotalSent.Add(1)

			// 从对象池获取请求对象
			req := reqPool.Get().(*OrderReq)
			req.OrderID = fmt.Sprintf("ORD-%d", counter)
			req.UserID = fmt.Sprintf("USER-%d", counter%10000) // 模拟10000个用户
			req.Amount = counter * 100 // 订单金额（分）
			req.Status = "PENDING"
			req.Timestamp = time.Now().UnixMicro()

			// 记录请求开始时间
			startTime := time.Now()
			stats.inflightReqs.Store(counter, startTime)

			localMu.Lock()
			localResponseCounts[counter] = 0
			localMu.Unlock()

			ctx := trace.WithTraceID(ctx, fmt.Sprintf("trace-order-%d", counter))

			err := cli.Call(ctx, "broadcast_stress", "broadcast_stress.ProcessOrder", req, nil,
				client.Options().
					SetBroadcast().
					SetReqCoderT(coder.Msgp).
					SetResCoderT(coder.Msgp).
					SetDebug(false). // 压力测试关闭 debug 日志
					SetBroadcastResNewFunc(func() any {
						return &OrderRes{}
					}).
					SetBroadcastResCallBack(func(ret any, err error, eos bool) bool {
						if err != nil {
							stats.TotalErrors.Add(1)
							stats.inflightReqs.Delete(counter)
							// 清理本地 map
							localMu.Lock()
							delete(localResponseCounts, counter)
							localMu.Unlock()
							// 归还请求对象到池
							reqPool.Put(req)
							return true
						}

						if res, ok := ret.(*OrderRes); ok {
							stats.TotalReceived.Add(1)

							// 记录来源
							switch res.From {
							case "sender_local":
								stats.TotalLocal.Add(1)
							case "receiver1":
								stats.TotalReceiver1.Add(1)
							case "receiver2":
								stats.TotalReceiver2.Add(1)
							}

							// 更新响应计数
							localMu.Lock()
							localResponseCounts[counter]++
							count := localResponseCounts[counter]
							localMu.Unlock()

							if eos {
								stats.TotalEOS.Add(1)

								// 计算延迟
								elapsed := time.Since(startTime).Microseconds()
								stats.updateLatency(elapsed)

								// 检查响应数量
								if count < 3 {
									stats.mu.Lock()
									// 限制 missingSeqs 大小，最多保留 1000 条
									if len(stats.missingSeqs) < 1000 {
										stats.missingSeqs = append(stats.missingSeqs, counter)
									}
									stats.mu.Unlock()
								}

								// 清理本地 map
								localMu.Lock()
								delete(localResponseCounts, counter)
								localMu.Unlock()

								stats.inflightReqs.Delete(counter)
								// 归还请求对象到池
								reqPool.Put(req)
								return false // EOS，停止接收
							}
						}

						return true // 继续接收
					}),
			)

			if err != nil {
				slog.Error("Send failed", "error", err, "counter", counter)
				stats.TotalErrors.Add(1)
				stats.inflightReqs.Delete(counter)
				// 清理本地 map
				localMu.Lock()
				delete(localResponseCounts, counter)
				localMu.Unlock()
				// 归还请求对象到池
				reqPool.Put(req)
			}
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

// reportStats 报告统计信息
func reportStats() {
	ticker := time.NewTicker(2 * time.Minute) // 每 2 分钟报告一次
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
	elapsed := time.Since(stats.LastSummary)

	// 计算增量
	deltaSent := sent - stats.lastSent
	deltaReceived := received - stats.lastReceived
	deltaErrs := errs - stats.lastErrs
	deltaEOS := eos - stats.lastEOS

	// 保存当前值作为下次的基准
	stats.lastSent = sent
	stats.lastReceived = received
	stats.lastErrs = errs
	stats.lastEOS = eos

	expected := deltaSent * 3
	missing := expected - deltaReceived - deltaErrs

	minLat := stats.MinLatency.Load()
	maxLat := stats.MaxLatency.Load()
	totalLat := stats.TotalLatency.Load()
	avgLat := int64(0)
	if deltaReceived > 0 {
		avgLat = totalLat / received // 使用累计计算平均
	}

	stats.mu.Lock()
	missingCount := len(stats.missingSeqs)
	stats.mu.Unlock()

	// 获取运行时指标
	numGoroutine := runtime.NumGoroutine()
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// 计算吞吐量 (req/s) - 使用增量
	throughput := float64(deltaSent) / elapsed.Seconds()

	slog.Info("=== 2分钟统计 ===",
		"发送", deltaSent,
		"接收", deltaReceived,
		"错误", deltaErrs,
		"EOS", deltaEOS,
		"预期", expected,
		"缺失", missing,
		"缺失序号数", missingCount,
		"吞吐量", fmt.Sprintf("%.0f req/s", throughput),
		"延迟(微秒)", fmt.Sprintf("min=%d max=%d avg=%d", minLat, maxLat, avgLat),
		"成功率", fmt.Sprintf("%.2f%%", float64(deltaReceived)/float64(expected)*100),
		"goroutine", numGoroutine,
		"内存(堆)", fmt.Sprintf("%.1fMB", float64(memStats.HeapAlloc)/1024/1024),
		"内存(系统)", fmt.Sprintf("%.1fMB", float64(memStats.Sys)/1024/1024),
		"GC次数", memStats.NumGC,
		"GC暂停(总)", fmt.Sprintf("%.2fms", float64(memStats.PauseTotalNs)/1000000))

	stats.LastSummary = time.Now()
}

func printFinalStats() {
	sent := stats.TotalSent.Load()
	received := stats.TotalReceived.Load()
	errs := stats.TotalErrors.Load()
	eos := stats.TotalEOS.Load()

	expected := sent * 3
	missing := expected - received - errs

	stats.mu.Lock()
	missingCount := len(stats.missingSeqs)
	stats.mu.Unlock()

	minLat := stats.MinLatency.Load()
	maxLat := stats.MaxLatency.Load()
	totalLat := stats.TotalLatency.Load()
	avgLat := int64(0)
	if received > 0 {
		avgLat = totalLat / received
	}

	// 获取运行时指标
	numGoroutine := runtime.NumGoroutine()
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	slog.Info("=== 最终统计 ===",
		"发送", sent,
		"接收", received,
		"错误", errs,
		"EOS", eos,
		"预期", expected,
		"缺失", missing,
		"缺失序号数", missingCount,
		"延迟(微秒)", fmt.Sprintf("min=%d max=%d avg=%d", minLat, maxLat, avgLat),
		"成功率", fmt.Sprintf("%.2f%%", float64(received)/float64(expected)*100),
		"goroutine", numGoroutine,
		"内存(堆)", fmt.Sprintf("%.1fMB", float64(memStats.HeapAlloc)/1024/1024),
		"内存(系统)", fmt.Sprintf("%.1fMB", float64(memStats.Sys)/1024/1024),
		"GC次数", memStats.NumGC,
		"GC暂停(总)", fmt.Sprintf("%.2fms", float64(memStats.PauseTotalNs)/1000000))

	if missingCount > 0 {
		stats.mu.Lock()
		allMissing := make([]int64, len(stats.missingSeqs))
		copy(allMissing, stats.missingSeqs)
		stats.mu.Unlock()
		slog.Warn("缺失的请求序号", "count", len(allMissing), "seqs", fmt.Sprintf("%v", allMissing))
	}
}
