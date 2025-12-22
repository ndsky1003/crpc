package main

//go:generate msgp
import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ndsky1003/crpc/v3/client"
	"github.com/ndsky1003/crpc/v3/example/dto"
	"github.com/ndsky1003/crpc/v3/example/trace"
	"github.com/ndsky1003/log"
)

// 定义广播服务
type BroadcastService struct{}

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

// 统计信息
var (
	broadcastStats struct {
		sync.RWMutex
		TotalBroadcasts int64
		TotalResponses  int64
		LastBroadcast   time.Time
	}
)

func main() {
	log.SetDefault(log.Options().SetExtractorAttr(func(ctx context.Context, r *slog.Record) {
		if tid := trace.ExtractorTraceID(ctx); tid != "" {
			r.Add("trace_id", tid)
		}
	}).SetAddSource(true))

	// 初始化 Client1 - 作为广播服务器提供者
	c, err := client.Dial(context.Background(), "client1_broadcast", ":8080",
		client.Options().SetSecret("ddddd").
			SetWithTraceID(func(ctx context.Context, tid string) context.Context {
				return trace.WithTraceID(ctx, tid)
			}).SetGenTraceID(func(ctx context.Context) string {
			return trace.ExtractorTraceID(ctx)
		}))
	if err != nil {
		fmt.Println("dial error:", err)
		return
	}

	fmt.Println("========================================")
	fmt.Println("Client1 广播服务器启动")
	fmt.Println("========================================")
	fmt.Printf("启动时间: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println("等待连接建立...")
	time.Sleep(2 * time.Second)

	// 创建广播服务实例
	_ = &BroadcastService{}

	// ==========================================
	// 注册广播服务
	// ==========================================

	// 使用 RegisterName 注册结构体，这样方法会自动被识别
	if err := c.RegisterName("BroadcastService", &BroadcastService{}); err != nil {
		fmt.Printf("Register BroadcastService error: %v\n", err)
	} else {
		fmt.Println("Registered 'BroadcastService' successfully")
	}

	fmt.Println("\n已注册的广播方法:")
	fmt.Println("  1. SimpleBroadcast    - 简单广播")
	fmt.Println("  2. TimeBroadcast      - 时间广播")
	fmt.Println("  3. StatsBroadcast     - 统计广播")
	fmt.Println("  4. ComputeBroadcast   - 计算广播")
	fmt.Println("  5. NotifyBroadcast    - 通知广播")
	fmt.Println("\n服务器就绪，等待广播请求...")
	fmt.Println("========================================")

	// 启动统计报告协程
	go statsReporter()

	// 阻塞主程
	select {}
}

// ==========================================
// 广播服务方法实现
// ==========================================

// 1. 简单广播 - 所有服务返回相同消息
// @crpc:CallType:Call
func (s *BroadcastService) SimpleBroadcast(ctx context.Context, req *BroadcastReq) (*BroadcastRes, error) {
	broadcastStats.Lock()
	broadcastStats.TotalBroadcasts++
	broadcastStats.LastBroadcast = time.Now()
	broadcastStats.Unlock()
	slog.Info("ddd", "dd", "dd")

	fmt.Printf("[SimpleBroadcast] 收到广播: %s from %s\n", req.Message, req.From)

	return &BroadcastRes{
		ServerName: "client1_broadcast",
		Message:    fmt.Sprintf("Echo: %s", req.Message),
		Timestamp:  time.Now().Unix(),
		Processed:  true,
	}, nil
}

// 2. 时间广播 - 每个服务返回自己的时间
// @crpc:CallType:Call
func (s *BroadcastService) TimeBroadcast(ctx context.Context, req *BroadcastReq) (*BroadcastRes, error) {
	broadcastStats.Lock()
	broadcastStats.TotalBroadcasts++
	broadcastStats.LastBroadcast = time.Now()
	broadcastStats.Unlock()

	return &BroadcastRes{
		ServerName: "client1_broadcast",
		Message:    fmt.Sprintf("Server time: %s", time.Now().Format("2006-01-02 15:04:05.000")),
		Timestamp:  time.Now().Unix(),
		Processed:  true,
	}, nil
}

// 3. 统计广播 - 返回服务器统计信息
// @crpc:CallType:Call
func (s *BroadcastService) StatsBroadcast(ctx context.Context, req *BroadcastReq) (*BroadcastRes, error) {
	broadcastStats.Lock()
	broadcastStats.TotalBroadcasts++
	broadcastStats.TotalResponses++
	broadcastStats.LastBroadcast = time.Now()
	broadcastStats.Unlock()

	return &BroadcastRes{
		ServerName: "client1_broadcast",
		Message:    fmt.Sprintf("Total broadcasts: %d, Total responses: %d", broadcastStats.TotalBroadcasts, broadcastStats.TotalResponses),
		Timestamp:  time.Now().Unix(),
		Processed:  true,
	}, nil
}

// 4. 计算广播 - 执行一些计算并返回结果
// @crpc:CallType:Call
func (s *BroadcastService) ComputeBroadcast(ctx context.Context, req *BroadcastReq) (*BroadcastRes, error) {
	broadcastStats.Lock()
	broadcastStats.TotalBroadcasts++
	broadcastStats.LastBroadcast = time.Now()
	broadcastStats.Unlock()

	// 模拟一些计算
	result := 0
	for i := 0; i < 1000; i++ {
		result += i
	}

	return &BroadcastRes{
		ServerName: "client1_broadcast",
		Message:    fmt.Sprintf("Computation result: %d", result),
		Timestamp:  time.Now().Unix(),
		Processed:  true,
	}, nil
}

// 5. 通知广播 - 模拟通知所有服务器
// @crpc:CallType:Call
func (s *BroadcastService) NotifyBroadcast(ctx context.Context, req *BroadcastReq) (*BroadcastRes, error) {
	broadcastStats.Lock()
	broadcastStats.TotalBroadcasts++
	broadcastStats.TotalResponses++
	broadcastStats.LastBroadcast = time.Now()
	broadcastStats.Unlock()

	// // fmt.Printf("[NotifyBroadcast] 通知内容: %s\n", req.Message)

	return &BroadcastRes{
		ServerName: "client1_broadcast",
		Message:    fmt.Sprintf("Notification received: %s at %s", req.Message, time.Now().Format("15:04:05")),
		Timestamp:  time.Now().Unix(),
		Processed:  true,
	}, nil
}

// ==========================================
// 其他服务方法（非广播）
// ==========================================

// 普通的查询方法
func QueryServer(ctx context.Context, req *dto.Req) (*dto.Res, error) {
	return &dto.Res{
		Msg: fmt.Sprintf("Server: client1_broadcast, Uptime: %s", time.Since(time.Now().Add(-24*time.Hour)).String()),
	}, nil
}

// 统计报告器
func statsReporter() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		broadcastStats.RLock()
		total := broadcastStats.TotalBroadcasts
		responses := broadcastStats.TotalResponses
		last := broadcastStats.LastBroadcast
		broadcastStats.RUnlock()

		fmt.Printf("\n[统计报告] 总广播数: %d, 总响应数: %d, 最后广播: %v\n",
			total, responses, last.Format("15:04:05"))
	}
}
