# crpc
中心服务的rpc，采用注册机制


#### 支持的方法签名
##### 结构体
```golang
// 这里是伪代码,用于代码生成示例
//
//go:generate gencrpcserverv3
//go:generate gencrpcclientv3 --out_dir=./db --package=db
package client

import (
	"context"
)

type msg_game struct{}

// @crpc:CallType:Call,Send,Go
// @crpc:Client: ccclient
// @crpc:Module: crpc
// @crpc:Service: db
// @crpc:FuncName:PlayerInfo1_gai
// @crpc:IsSkip:true
func (*msg_game) PlayerInfo1(meta *Meta, req *PlayerInfoReq) (*PlayerInfoRes, error) {
	return nil, nil
}

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
	fmt.Printf("[Struct] ResOnly Called: Req=%v\n", req)
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


```
##### 函数
> 除了**context.Context**,**error**,其他的均支持值与指针,除了meta,与req没办法辨别以外,其他的均可任意组合
```golang
// 1. 全量: (Ctx, Meta, Req) -> (Res, error)
func FnFull(ctx context.Context, meta *Meta, req *Req) (*Res, error) {
	traceid := trace.GetTraceID(ctx)
	fmt.Printf("[FnFull] Meta=%v, Req=%v,traceid=%v\n", meta, req, traceid)
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

```



