package main

import (
	"context"
	"fmt"

	"github.com/ndsky1003/crpc/v3/client"
)

// 定义测试用的结构体
type Req struct {
	Name string
}
type Res struct {
	Msg string
}
type Meta struct {
	Source string
}

func main1() {
	// 假设服务端监听在 :8080
	c, err := client.Dial(context.Background(), "client1", ":8080")
	if err != nil {
		fmt.Println("dial error:", err)
		return
	}
	fmt.Println("client1 started")

	// ==========================================
	// 场景 1: 标准全量 (Ctx, Meta, Req) -> (Res, error)
	// ==========================================
	c.RegisterFunc("cc", FnFull, "FnFull")

	// ==========================================
	// 场景 2: 常用组合 (Ctx, Req) -> (Res, error)
	// ==========================================
	// 指针类型 Req/Res
	c.RegisterFunc("cc", FnCtxReqPtr, "FnCtxReqPtr")
	// 值类型 Req/Res (如你的 string 示例)
	c.RegisterFunc("cc", FnCtxReqVal, "FnCtxReqVal")

	// ==========================================
	// 场景 3: 省略 Context (Meta, Req) 或 (Req)
	// ==========================================
	// 带 Meta 无 Ctx
	c.RegisterFunc("cc", FnMetaReq, "FnMetaReq")
	// 极简模式：只有 Req
	c.RegisterFunc("cc", FnReqOnly, "FnReqOnly")

	// ==========================================
	// 场景 4: 返回值变体
	// ==========================================
	// 只返回结果 (无 error)
	c.RegisterFunc("cc", FnResOnly, "FnResOnly")
	// 只返回错误 (无 Res)
	c.RegisterFunc("cc", FnErrOnly, "FnErrOnly")
	// 无返回值 (Fire-and-Forget)
	c.RegisterFunc("cc", FnNoReturn, "FnNoReturn")

	// ==========================================
	// 场景 5: 特殊边界
	// ==========================================
	// 无参无返
	c.RegisterFunc("cc", FnEmpty, "FnEmpty")
	// 仅 Context
	c.RegisterFunc("cc", FnCtxOnly, "FnCtxOnly")

	// 阻塞主程
	select {}
}

// -------------------------------------------------------
// 具体实现
// -------------------------------------------------------

// 1. 全量: (Ctx, Meta, Req) -> (Res, error)
func FnFull(ctx context.Context, meta *Meta, req *Req) (*Res, error) {
	fmt.Printf("[FnFull] Meta=%v, Req=%v\n", meta, req)
	return &Res{Msg: "OK from Full"}, nil
}

// 2. 常用: (Ctx, Req) -> (Res, error)
func FnCtxReqPtr(ctx context.Context, req *Req) (*Res, error) {
	fmt.Printf("[FnCtxReqPtr] Req=%v\n", req)
	return &Res{Msg: "OK from CtxReqPtr"}, nil
}

// 对应你的 string 示例
func FnCtxReqVal(ctx context.Context, req string) (string, error) {
	fmt.Printf("[FnCtxReqVal] Req=%s\n", req)
	return "playerinfo:" + req, nil
}

// 3. 无 Ctx: (Meta, Req) -> (Res, error)
func FnMetaReq(meta *Meta, req *Req) (*Res, error) {
	fmt.Printf("[FnMetaReq] Meta=%v, Req=%v\n", meta, req)
	return &Res{Msg: "OK from MetaReq"}, nil
}

// 4. 极简: (Req) -> (Res, error)
func FnReqOnly(req *Req) (*Res, error) {
	fmt.Printf("[FnReqOnly] Req=%v\n", req)
	return &Res{Msg: "OK from ReqOnly"}, nil
}

// 5. 只返回结果: (Req) -> (Res)
func FnResOnly(req *Req) *Res {
	fmt.Printf("[FnResOnly] Req=%v\n", req)
	return &Res{Msg: "OK from ResOnly"}
}

// 6. 只返回错误: (Req) -> (error)
func FnErrOnly(req *Req) error {
	fmt.Printf("[FnErrOnly] Req=%v\n", req)
	return nil
}

// 7. 无返回值: (Req) -> ()
func FnNoReturn(req *Req) {
	fmt.Printf("[FnNoReturn] Req=%v\n", req)
}

// 8. 无参无返: () -> ()
func FnEmpty() {
	fmt.Println("[FnEmpty] Called")
}

// 9. 仅 Context: (Ctx) -> ()
func FnCtxOnly(ctx context.Context) {
	fmt.Println("[FnCtxOnly] Called with context")
}
