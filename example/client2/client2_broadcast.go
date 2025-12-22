package main

//go:generate msgp
import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ndsky1003/crpc/v3/client"
	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/crpc/v3/example/dto"
	"github.com/ndsky1003/crpc/v3/example/trace"
	"github.com/ndsky1003/log"
)

// 广播请求结构
type BroadcastReq struct {
	Message   string `msg:"message"`
	Timestamp int64  `msg:"timestamp"` // 使用 int64 存储 Unix 时间戳
	From      string `msg:"from"`
}

// 广播响应结构
type BroadcastRes struct {
	ServerName string `msg:"server_name"`
	Message    string `msg:"message"`
	Timestamp  int64  `msg:"timestamp"` // 使用 int64 存储 Unix 时间戳
	Processed  bool   `msg:"processed"`
}

var c *client.Client

// 收集广播结果的结构
type broadcastResultCollector struct {
	mu          sync.RWMutex
	responses   []*BroadcastRes
	serverCount int32
	done        bool
}

func main() {
	log.SetDefault(log.Options().SetExtractorAttr(func(ctx context.Context, r *slog.Record) {
		if tid := trace.ExtractorTraceID(ctx); tid != "" {
			r.Add("trace_id", tid)
		}
	}).SetAddSource(true))

	var err error
	c, err = client.Dial(context.Background(), "client2_broadcast", ":8080",
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

	fmt.Println("========================================")
	fmt.Println("Client2 广播调用者")
	fmt.Println("========================================")
	fmt.Printf("启动时间: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println("等待连接建立...")
	time.Sleep(3 * time.Second)

	// 注册一个服务，这样服务器端会有多个实例
	// registerDummyService()

	// 等待服务注册
	time.Sleep(2 * time.Second)

	fmt.Println("\n========================================")
	fmt.Println("开始广播测试")
	fmt.Println("========================================\n")

	// 运行各种广播测试
	runBroadcastTests()

	fmt.Println("\n========================================")
	fmt.Println("测试完成，保持程序运行...")
	fmt.Println("按 Ctrl+C 退出")
	fmt.Println("========================================")

	select {}
}

// 注册一个虚拟服务，让服务器端有多个实例
func registerDummyService() {
	type DummyReq struct {
		Name string `msg:"name"`
	}
	type DummyRes struct {
		Msg string `msg:"msg"`
	}

	dummySvc := struct {
		// 导出方法名（首字母大写）
		Ping func(ctx context.Context, req *DummyReq) (*DummyRes, error)
	}{
		Ping: func(ctx context.Context, req *DummyReq) (*DummyRes, error) {
			return &DummyRes{Msg: fmt.Sprintf("Pong from client2_broadcast: %s", req.Name)}, nil
		},
	}

	if err := c.RegisterName("DummyService", dummySvc); err != nil {
		fmt.Printf("Register DummyService error: %v\n", err)
	} else {
		fmt.Println("DummyService 注册成功")
	}
}

// ==========================================
// 广播测试
// ==========================================

func runBroadcastTests() {
	// 测试1: 简单广播
	testSimpleBroadcast()

	// 等待一下
	time.Sleep(2 * time.Second)

	// 测试2: 时间广播
	testTimeBroadcast()

	// 等待一下
	time.Sleep(2 * time.Second)

	// 测试3: 统计广播
	testStatsBroadcast()

	// 等待一下
	time.Sleep(2 * time.Second)

	// 测试4: 计算广播
	testComputeBroadcast()

	// 等待一下
	time.Sleep(2 * time.Second)

	// 测试5: 通知广播
	testNotifyBroadcast()

	// 等待一下
	time.Sleep(2 * time.Second)

	// 测试6: 连续广播
	testContinuousBroadcast()
}

// 测试1: 简单广播
func testSimpleBroadcast() {
	fmt.Println("========== 测试1: 简单广播 ==========")

	req := &BroadcastReq{
		Message:   "Hello, Broadcast!",
		Timestamp: time.Now().Unix(),
		From:      "client2_broadcast",
	}

	collector := &broadcastResultCollector{}

	// 调用广播 - 方法名格式: 服务名.方法名
	ctx := context.Background()
	err := c.Call(ctx, "BroadcastService", "BroadcastService.SimpleBroadcast", req, nil,
		client.Options().
			SetBroadcast().                        // 启用广播
			SetMeta(&dto.Meta{Source: "client2"}). // 设置元数据
			SetBroadcastChanCap(10).               // 广播结果通道容量
			SetMetaCoderT(coder.Msgp).
			SetReqCoderT(coder.Msgp).
			SetResCoderT(coder.Msgp).
			SetBroadcastResNewFunc(func() any { // 创建响应对象的函数
				return &BroadcastRes{}
			}).
			SetBroadcastResCallBack(func(ret any, err error, eos bool) bool { // 广播结果回调
				if err != nil {
					fmt.Printf("  [错误1] %v\n", err)
					return true
				}

				if res, ok := ret.(*BroadcastRes); ok {
					collector.mu.Lock()
					collector.responses = append(collector.responses, res)
					atomic.AddInt32(&collector.serverCount, 1)
					collector.mu.Unlock()
					fmt.Printf("  [响应] 来自: %s, 消息: %s, 时间: %s\n",
						res.ServerName, res.Message, time.Unix(res.Timestamp, 0).Format("15:04:05.000"))
				}

				if eos {
					collector.mu.Lock()
					collector.done = true
					collector.mu.Unlock()
					fmt.Println("  [完成] 广播结束")
					return false // 停止接收更多结果
				}

				return true // 继续接收结果
			}),
	)

	if err != nil {
		fmt.Printf("广播调用失败: %v\n", err)
		return
	}

	// 等待广播完成
	waitForBroadcastComplete(collector, 5*time.Second)

	fmt.Printf("结果统计: 收到 %d 个响应\n", atomic.LoadInt32(&collector.serverCount))
}

// 测试2: 时间广播
func testTimeBroadcast() {
	fmt.Println("\n========== 测试2: 时间广播 ==========")

	req := &BroadcastReq{
		Message:   "Get Server Time",
		Timestamp: time.Now().Unix(),
		From:      "client2_broadcast",
	}

	collector := &broadcastResultCollector{}

	ctx := context.Background()
	err := c.Call(ctx, "BroadcastService", "BroadcastService.TimeBroadcast", req, nil,
		client.Options().
			SetBroadcast().
			SetMeta(&dto.Meta{Source: "client2"}).
			SetBroadcastChanCap(10).
			SetBroadcastResNewFunc(func() any {
				return &BroadcastRes{}
			}).
			SetBroadcastResCallBack(func(ret any, err error, eos bool) bool {
				if err != nil {
					fmt.Printf("  [错误] %v\n", err)
					return true
				}

				if res, ok := ret.(*BroadcastRes); ok {
					collector.mu.Lock()
					collector.responses = append(collector.responses, res)
					atomic.AddInt32(&collector.serverCount, 1)
					collector.mu.Unlock()
					fmt.Printf("  [响应] %s: %s\n", res.ServerName, res.Message)
				}

				if eos {
					collector.mu.Lock()
					collector.done = true
					collector.mu.Unlock()
					return false
				}

				return true
			}),
	)

	if err != nil {
		fmt.Printf("广播调用失败: %v\n", err)
		return
	}

	waitForBroadcastComplete(collector, 5*time.Second)

	fmt.Printf("结果统计: 收到 %d 个服务器的时间响应\n", atomic.LoadInt32(&collector.serverCount))

	// 显示所有时间差异
	collector.mu.RLock()
	responses := make([]*BroadcastRes, len(collector.responses))
	copy(responses, collector.responses)
	collector.mu.RUnlock()

	if len(responses) > 1 {
		fmt.Println("时间差异分析:")
		for i := 1; i < len(responses); i++ {
			t1 := time.Unix(responses[0].Timestamp, 0)
			t2 := time.Unix(responses[i].Timestamp, 0)
			diff := t2.Sub(t1)
			fmt.Printf("  服务器 %d 与 服务器 0 的时间差: %v\n", i, diff)
		}
	}
}

// 测试3: 统计广播
func testStatsBroadcast() {
	fmt.Println("\n========== 测试3: 统计广播 ==========")

	req := &BroadcastReq{
		Message:   "Get Statistics",
		Timestamp: time.Now().Unix(),
		From:      "client2_broadcast",
	}

	collector := &broadcastResultCollector{}

	ctx := context.Background()
	err := c.Call(ctx, "BroadcastService", "BroadcastService.StatsBroadcast", req, nil,
		client.Options().
			SetBroadcast().
			SetMeta(&dto.Meta{Source: "client2"}).
			SetBroadcastChanCap(10).
			SetBroadcastResNewFunc(func() any {
				return &BroadcastRes{}
			}).
			SetBroadcastResCallBack(func(ret any, err error, eos bool) bool {
				if err != nil {
					fmt.Printf("  [错误] %v\n", err)
					return true
				}

				if res, ok := ret.(*BroadcastRes); ok {
					collector.mu.Lock()
					collector.responses = append(collector.responses, res)
					atomic.AddInt32(&collector.serverCount, 1)
					collector.mu.Unlock()
					fmt.Printf("  [响应] %s\n", res.Message)
				}

				if eos {
					collector.mu.Lock()
					collector.done = true
					collector.mu.Unlock()
					return false
				}

				return true
			}),
	)

	if err != nil {
		fmt.Printf("广播调用失败: %v\n", err)
		return
	}

	waitForBroadcastComplete(collector, 5*time.Second)

	fmt.Printf("结果统计: 收到 %d 个服务器的统计信息\n", atomic.LoadInt32(&collector.serverCount))
}

// 测试4: 计算广播
func testComputeBroadcast() {
	fmt.Println("\n========== 测试4: 计算广播 ==========")

	req := &BroadcastReq{
		Message:   "Perform Computation",
		Timestamp: time.Now().Unix(),
		From:      "client2_broadcast",
	}

	collector := &broadcastResultCollector{}

	ctx := context.Background()
	err := c.Call(ctx, "BroadcastService", "BroadcastService.ComputeBroadcast", req, nil,
		client.Options().
			SetBroadcast().
			SetMeta(&dto.Meta{Source: "client2"}).
			SetBroadcastChanCap(10).
			SetBroadcastResNewFunc(func() any {
				return &BroadcastRes{}
			}).
			SetBroadcastResCallBack(func(ret any, err error, eos bool) bool {
				if err != nil {
					fmt.Printf("  [错误] %v\n", err)
					return true
				}

				if res, ok := ret.(*BroadcastRes); ok {
					collector.mu.Lock()
					collector.responses = append(collector.responses, res)
					atomic.AddInt32(&collector.serverCount, 1)
					collector.mu.Unlock()
					fmt.Printf("  [响应] %s: %s (耗时: %v)\n",
						res.ServerName, res.Message, time.Since(time.Unix(req.Timestamp, 0)))
				}

				if eos {
					collector.mu.Lock()
					collector.done = true
					collector.mu.Unlock()
					return false
				}

				return true
			}),
	)

	if err != nil {
		fmt.Printf("广播调用失败: %v\n", err)
		return
	}

	waitForBroadcastComplete(collector, 5*time.Second)

	fmt.Printf("结果统计: 收到 %d 个服务器的计算结果，总耗时: %v\n",
		atomic.LoadInt32(&collector.serverCount), time.Since(time.Unix(req.Timestamp, 0)))
}

// 测试5: 通知广播
func testNotifyBroadcast() {
	fmt.Println("\n========== 测试5: 通知广播 ==========")

	req := &BroadcastReq{
		Message:   "System Maintenance at 02:00 AM",
		Timestamp: time.Now().Unix(),
		From:      "client2_broadcast",
	}

	collector := &broadcastResultCollector{}

	ctx := context.Background()
	err := c.Call(ctx, "BroadcastService", "BroadcastService.NotifyBroadcast", req, nil,
		client.Options().
			SetBroadcast().
			SetMeta(&dto.Meta{Source: "client2"}).
			SetBroadcastChanCap(10).
			SetBroadcastResNewFunc(func() any {
				return &BroadcastRes{}
			}).
			SetBroadcastResCallBack(func(ret any, err error, eos bool) bool {
				if err != nil {
					fmt.Printf("  [错误] %v\n", err)
					return true
				}

				if res, ok := ret.(*BroadcastRes); ok {
					collector.mu.Lock()
					collector.responses = append(collector.responses, res)
					atomic.AddInt32(&collector.serverCount, 1)
					collector.mu.Unlock()
					fmt.Printf("  [响应] %s 确认: %s\n", res.ServerName, res.Message)
				}

				if eos {
					collector.mu.Lock()
					collector.done = true
					collector.mu.Unlock()
					return false
				}

				return true
			}),
	)

	if err != nil {
		fmt.Printf("广播调用失败: %v\n", err)
		return
	}

	waitForBroadcastComplete(collector, 5*time.Second)

	fmt.Printf("结果统计: 已通知 %d 个服务器\n", atomic.LoadInt32(&collector.serverCount))
}

// 测试6: 连续广播
func testContinuousBroadcast() {
	fmt.Println("\n========== 测试6: 连续广播 (3次) ==========")

	for i := 1; i <= 3; i++ {
		fmt.Printf("\n第 %d 次广播:\n", i)

		req := &BroadcastReq{
			Message:   fmt.Sprintf("Continuous Broadcast #%d", i),
			Timestamp: time.Now().Unix(),
			From:      "client2_broadcast",
		}

		collector := &broadcastResultCollector{}

		ctx := context.Background()
		err := c.Call(ctx, "BroadcastService", "BroadcastService.SimpleBroadcast", req, nil,
			client.Options().
				SetBroadcast().
				SetMeta(&dto.Meta{Source: "client2"}).
				SetBroadcastChanCap(10).
				SetBroadcastResNewFunc(func() any {
					return &BroadcastRes{}
				}).
				SetBroadcastResCallBack(func(ret any, err error, eos bool) bool {
					if err != nil {
						return true
					}

					if res, ok := ret.(*BroadcastRes); ok {
						collector.mu.Lock()
						collector.responses = append(collector.responses, res)
						count := atomic.AddInt32(&collector.serverCount, 1)
						collector.mu.Unlock()
						fmt.Printf("  [%d.%d] %s: %s\n", i, count, res.ServerName, res.Message)
					}

					if eos {
						collector.mu.Lock()
						collector.done = true
						collector.mu.Unlock()
						return false
					}

					return true
				}),
		)

		if err != nil {
			fmt.Printf("  广播 #%d 失败: %v\n", i, err)
			continue
		}

		waitForBroadcastComplete(collector, 5*time.Second)
		fmt.Printf("  广播 #%d 完成，收到 %d 个响应\n", i, atomic.LoadInt32(&collector.serverCount))

		// 等待一下再发送下一个
		time.Sleep(500 * time.Millisecond)
	}

	fmt.Println("\n连续广播测试完成")
}

// ==========================================
// 辅助函数
// ==========================================

// 等待广播完成
func waitForBroadcastComplete(collector *broadcastResultCollector, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			collector.mu.RLock()
			done := collector.done
			collector.mu.RUnlock()

			if done {
				return
			}

			if time.Now().After(deadline) {
				fmt.Println("  [警告] 等待广播完成超时")
				return
			}

		case <-time.After(timeout):
			fmt.Println("  [警告] 等待广播完成超时")
			return
		}
	}
}
