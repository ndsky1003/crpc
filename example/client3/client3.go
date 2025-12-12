//go:generate gencrpcserverv3
//go:generate gencrpcclientv3 --out_dir ../comm --package comm --client Default_lient --service client3 --module  MyService
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/ndsky1003/crpc/v3/client"
	"github.com/ndsky1003/crpc/v3/example/comm"
	"github.com/ndsky1003/crpc/v3/example/dto"
)

// ==========================================
// 2. 定义服务结构体
// ==========================================
type MyStructService struct{}

// -------------------------------------------------------
// 场景 1: 标准全量 (Ctx, Meta, Req) -> (Res, error)
// -------------------------------------------------------
func (s *MyStructService) Full(ctx context.Context, meta *dto.Meta, req *dto.Req) (*dto.Res, error) {
	fmt.Printf("[Struct] Full Called: Meta=%v, Req=%v\n", meta, req)
	return &dto.Res{Msg: "OK from Struct.Full"}, nil
}

// -------------------------------------------------------
// 场景 2: 常用组合 (Ctx, Req) -> (Res, error)
// -------------------------------------------------------
// 指针类型
func (s *MyStructService) CtxReqPtr(ctx context.Context, req *dto.Req) (*dto.Res, error) {
	fmt.Printf("[Struct] CtxReqPtr Called: Req=%v\n", req)
	return &dto.Res{Msg: "OK from Struct.CtxReqPtr"}, nil
}

// 值类型 (Req/Res 为 string)
func (s *MyStructService) CtxReqVal(ctx context.Context, req string) (string, error) {
	fmt.Printf("[Struct] CtxReqVal Called: Req=%s\n", req)
	return "Struct.CtxReqVal:" + req, nil
}

// -------------------------------------------------------
// 场景 3: 省略 Context
// -------------------------------------------------------
// (Meta, Req) -> (Res, error)
func (s *MyStructService) MetaReq(meta *dto.Meta, req *dto.Req) (*dto.Res, error) {
	fmt.Printf("[Struct] MetaReq Called: Meta=%v, Req=%v\n", meta, req)
	return &dto.Res{Msg: "OK from Struct.MetaReq"}, nil
}

// -------------------------------------------------------
// 场景 4: 极简模式 (Req Only)
// -------------------------------------------------------
// (Req) -> (Res, error)
func (s *MyStructService) ReqOnly(req *dto.Req) (*dto.Res, error) {
	fmt.Printf("[Struct] ReqOnly Called: Req=%v\n", req)
	return &dto.Res{Msg: "OK from Struct.ReqOnly"}, nil
}

// -------------------------------------------------------
// 场景 5: 返回值变体
// -------------------------------------------------------
// 只返回结果: (Req) -> (Res)
// 注意：如果发生错误框架层无法捕获，仅用于必定成功的逻辑
func (s *MyStructService) ResOnly(req *dto.Req) *dto.Res {
	fmt.Printf("[Struct] ResOnly Called: Req=%v,%p\n", req, req)
	return &dto.Res{Msg: "OK from Struct.ResOnly"}
}

// 只返回错误: (Req) -> (error)
func (s *MyStructService) ErrOnly(req *dto.Req) error {
	fmt.Printf("[Struct] ErrOnly Called: Req=%v\n", req)
	return nil
}

// 无返回值: (Req) -> ()
func (s *MyStructService) NoReturn(req *dto.Req) {
	fmt.Printf("[Struct] NoReturn Called: Req=%v\n", req)
}

// -------------------------------------------------------
// 场景 6: 特殊边界
// -------------------------------------------------------
// 无参无返: () -> ()
func (s *MyStructService) Empty() {
	fmt.Println("[Struct] Empty Called")
}

// 仅 Context: (Ctx) -> ()
func (s *MyStructService) CtxOnly(ctx context.Context) {
	fmt.Println("[Struct] CtxOnly Called")
}

// ==========================================
// 3. Main 测试入口
// ==========================================
func main() {
	// 1. 初始化 Client
	c, err := client.Dial(context.Background(), "client3", ":8080")
	if err != nil {
		fmt.Println("dial error:", err)
		return
	}
	comm.Default_lient = c
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

	time.AfterFunc(5*time.Second, run)

	// 方式 B: 使用 Register 自动推导服务名 (通常是结构体名 "MyStructService")
	// if err := c.Register(svc); err != nil { ... }

	// 阻塞以保持服务运行
	select {}
}

func run() {
	// ctx := context.Background()
	// var meta = dto.Meta{Source: "client2"}
	var req = &dto.Req{Name: "ll"}
	// var req = "ddd"
	// var res Res
	fmt.Printf("req pointer:%p", req)
	res := comm.ResOnly(req)
	fmt.Println(res)
	// err := c.Call(context.Background(), "client1", "cc.FnCtxOnly", req, &res, client.Options().SetMeta(meta).SetTraceID("traceid-client2-001"))
	// fmt.Println("client2:", " call result:", res, err)

}
