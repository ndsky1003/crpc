package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ndsky1003/crpc/v3/client"
	"github.com/ndsky1003/crpc/v3/example/comm"
	"github.com/ndsky1003/crpc/v3/example/dto"
	"github.com/ndsky1003/crpc/v3/example/trace"
	"github.com/ndsky1003/log"
)

// ==========================================
// 2. 定义服务结构体
// ==========================================
type MyStructService struct{}

// -------------------------------------------------------
// 场景 1: 标准全量 (Ctx, Meta, Req) -> (Res, error)
// -------------------------------------------------------
// @crpc:CallType:Call,Send,Go
func (s *MyStructService) Full(ctx context.Context, meta *dto.Meta, req *dto.Req) (*dto.Res, error) {
	// 	fmt.Printf("[Struct] Full Called: Meta=%v, Req=%v\n", meta, req)
	return &dto.Res{Msg: "OK from Struct.Full"}, nil
}

// -------------------------------------------------------
// 场景 2: 常用组合 (Ctx, Req) -> (Res, error)
// -------------------------------------------------------
// 指针类型
func (s *MyStructService) CtxReqPtr(ctx context.Context, req *dto.Req) (*dto.Res, error) {
	// 	fmt.Printf("[Struct] CtxReqPtr Called: Req=%v\n", req)
	return &dto.Res{Msg: "OK from Struct.CtxReqPtr"}, nil
}

// 值类型 (Req/Res 为 string)
func (s *MyStructService) CtxReqVal(ctx context.Context, req string) (string, error) {
	// 	fmt.Printf("[Struct] CtxReqVal Called: Req=%s\n", req)
	return "Struct.CtxReqVal:" + req, nil
}

// -------------------------------------------------------
// 场景 3: 省略 Context
// -------------------------------------------------------
// (Meta, Req) -> (Res, error)
func (s *MyStructService) MetaReq(meta *dto.Meta, req *dto.Req) (*dto.Res, error) {
	// 	fmt.Printf("[Struct] MetaReq Called: Meta=%v, Req=%v\n", meta, req)
	return &dto.Res{Msg: "OK from Struct.MetaReq"}, nil
}

// -------------------------------------------------------
// 场景 4: 极简模式 (Req Only)
// -------------------------------------------------------
// (Req) -> (Res, error)
func (s *MyStructService) ReqOnly(req *dto.Req) (*dto.Res, error) {
	// 	fmt.Printf("[Struct] ReqOnly Called: Req=%v\n", req)
	return &dto.Res{Msg: "OK from Struct.ReqOnly"}, nil
}

// -------------------------------------------------------
// 场景 5: 返回值变体
// -------------------------------------------------------
// 只返回结果: (Req) -> (Res)
// 注意：如果发生错误框架层无法捕获，仅用于必定成功的逻辑
func (s *MyStructService) ResOnly(req *dto.Req) *dto.Res {
	// 	fmt.Printf("[Struct] ResOnly Called: Req=%v,%p\n", req, req)
	return &dto.Res{Msg: "OK from Struct.ResOnly"}
}

// 只返回错误: (Req) -> (error)
func (s *MyStructService) ErrOnly(req *dto.Req) error {
	// 	fmt.Printf("[Struct] ErrOnly Called: Req=%v\n", req)
	return nil
}

// 无返回值: (Req) -> ()
func (s *MyStructService) NoReturn(req *dto.Req) {
	// fmt.Printf("[Struct] NoReturn Called: Req=%v\n", req)
}

// -------------------------------------------------------
// 场景 6: 特殊边界
// -------------------------------------------------------
// 无参无返: () -> ()
func (s *MyStructService) Empty() {
	// fmt.Println("[Struct] Empty Called")
}

// 仅 Context: (Ctx) -> ()
func (s *MyStructService) CtxOnly(ctx context.Context) {
	// fmt.Println("[Struct] CtxOnly Called")
}

// -------------------------------------------------------
// 遗漏的方法签名
// -------------------------------------------------------

// (Ctx) -> (Res, error)
func (s *MyStructService) CtxOnlyResErr(ctx context.Context) (*dto.Res, error) {
	// 	fmt.Println("[Struct] CtxOnlyResErr Called")
	return &dto.Res{Msg: "OK from Struct.CtxOnlyResErr"}, nil
}

// (Ctx) -> (Res)
func (s *MyStructService) CtxOnlyRes(ctx context.Context) *dto.Res {
	// 	fmt.Println("[Struct] CtxOnlyRes Called")
	return &dto.Res{Msg: "OK from Struct.CtxOnlyRes"}
}

// (Ctx) -> (error)
func (s *MyStructService) CtxOnlyErr(ctx context.Context) error {
	// 	fmt.Println("[Struct] CtxOnlyErr Called")
	return nil
}

// () -> (Res, error)
func (s *MyStructService) NoParamsResErr() (*dto.Res, error) {
	// 	fmt.Println("[Struct] NoParamsResErr Called")
	return &dto.Res{Msg: "OK from Struct.NoParamsResErr"}, nil
}

// () -> (Res)
func (s *MyStructService) NoParamsRes() *dto.Res {
	// 	fmt.Println("[Struct] NoParamsRes Called")
	return &dto.Res{Msg: "OK from Struct.NoParamsRes"}
}

// () -> (error)
func (s *MyStructService) NoParamsErr() error {
	// 	fmt.Println("[Struct] NoParamsErr Called")
	return nil
}

// -------------------------------------------------------
// (Ctx, Meta, Req) 组合的完整变体
// -------------------------------------------------------

// (Ctx, Meta, Req) -> (Res, error)
func (s *MyStructService) CtxMetaReqResErr(ctx context.Context, meta *dto.Meta, req *dto.Req) (*dto.Res, error) {
	// 	fmt.Printf("[Struct] CtxMetaReqResErr Called: Meta=%v, Req=%v\n", meta, req)
	return &dto.Res{Msg: "OK from Struct.CtxMetaReqResErr"}, nil
}

// (Ctx, Meta, Req) -> (Res)
func (s *MyStructService) CtxMetaReqRes(ctx context.Context, meta *dto.Meta, req *dto.Req) *dto.Res {
	// 	fmt.Printf("[Struct] CtxMetaReqRes Called: Meta=%v, Req=%v\n", meta, req)
	return &dto.Res{Msg: "OK from Struct.CtxMetaReqRes"}
}

// (Ctx, Meta, Req) -> (error)
func (s *MyStructService) CtxMetaReqErr(ctx context.Context, meta *dto.Meta, req *dto.Req) error {
	// 	fmt.Printf("[Struct] CtxMetaReqErr Called: Meta=%v, Req=%v\n", meta, req)
	return nil
}

// (Ctx, Meta, Req) -> ()
func (s *MyStructService) CtxMetaReq(ctx context.Context, meta *dto.Meta, req *dto.Req) {
	// fmt.Printf("[Struct] CtxMetaReq Called: Meta=%v, Req=%v\n", meta, req)
}

// ==========================================
// 3. Main 测试入口
// ==========================================
func main() {
	log.SetDefault(log.Options().SetExtractorAttr(func(ctx context.Context, r *slog.Record) {
		if tid := trace.ExtractorTraceID(ctx); tid != "" {
			r.Add("trace_id", tid)
		}
	}).SetAddSource(true))
	// 1. 初始化 Client
	c, err := client.Dial(context.Background(), "client3", ":8080",
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
	comm.Default_Client = c
	fmt.Println("Client started...")

	// 2. 实例化服务对象
	svc := &MyStructService{}

	// 3. 注册结构体
	// 方式 A: 使用 RegisterName 指定服务名为 "MyService"
	// 此时方法会映射为 "MyService.Full", "MyService.CtxReqPtr" 等
	if err := c.RegisterName("MyService", svc); err != nil {
		fmt.Printf("RegisterName error: %v\n", err)
	} else {
		fmt.Println("Registered 'MyService' successfully")
	}

	// 方式 B: 使用 Register 自动推导服务名 (通常是结构体名 "MyStructService")
	// if err := c.Register(svc); err != nil { ... }

	// 阻塞以保持服务运行
	select {}
}
