package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ndsky1003/crpc/v3/client"
	"github.com/ndsky1003/crpc/v3/example/comm"
	"github.com/ndsky1003/crpc/v3/example/dto"
	"github.com/ndsky1003/crpc/v3/example/trace"
	"github.com/ndsky1003/log"
)

func main() {
	log.SetDefault(log.Options().SetExtractorAttr(func(ctx context.Context, r *slog.Record) {
		if tid := trace.ExtractorTraceID(ctx); tid != "" {
			r.Add("trace_id", tid)
		}
	}).SetAddSource(true))

	// 初始化 Client4 - 连接到server，准备调用client3的服务
	c, err := client.Dial(context.Background(), "client4", ":8080",
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

	// 设置全局客户端变量
	comm.Default_Client = c
	fmt.Println("Client4 started... connecting to server to call client3 services")

	// 等待连接建立
	time.Sleep(2 * time.Second)

	// 调用client3中MyService的所有方法
	callAllClient3Methods()

	// 保持程序运行
	select {}
}

func callAllClient3Methods() {
	ctx := context.Background()
	meta := &dto.Meta{Source: "client4"}
	req := &dto.Req{Name: "client4"}
	stringReq := "test_string_from_client4"

	fmt.Println("\n========== Client4 调用 Client3 (MyService) 的所有方法 ==========")

	// 1. Full - (Ctx, Meta, Req) -> (Res, error)
	fmt.Println("\n1. 调用 Full 方法:")
	res, err := comm.Full(ctx, meta, req)
	if err != nil {
		fmt.Printf("   错误: %v\n", err)
	} else {
		fmt.Printf("   结果: %v\n", res)
	}

	// 2. CtxReqPtr - (Ctx, Req) -> (Res, error)
	fmt.Println("\n2. 调用 CtxReqPtr 方法:")
	res, err = comm.CtxReqPtr(ctx, req)
	if err != nil {
		fmt.Printf("   错误: %v\n", err)
	} else {
		fmt.Printf("   结果: %v\n", res)
	}

	// 3. CtxReqVal - (Ctx, Req) -> (Res, error) - Req/Res为string
	fmt.Println("\n3. 调用 CtxReqVal 方法:")
	stringRes, err := comm.CtxReqVal(ctx, stringReq)
	if err != nil {
		fmt.Printf("   错误: %v\n", err)
	} else {
		fmt.Printf("   结果: %s\n", stringRes)
	}

	// 4. MetaReq - (Meta, Req) -> (Res, error)
	fmt.Println("\n4. 调用 MetaReq 方法:")
	res, err = comm.MetaReq(meta, req)
	if err != nil {
		fmt.Printf("   错误: %v\n", err)
	} else {
		fmt.Printf("   结果: %v\n", res)
	}

	// 5. ReqOnly - (Req) -> (Res, error)
	fmt.Println("\n5. 调用 ReqOnly 方法:")
	res, err = comm.ReqOnly(req)
	if err != nil {
		fmt.Printf("   错误: %v\n", err)
	} else {
		fmt.Printf("   结果: %v\n", res)
	}

	// 6. ResOnly - (Req) -> (Res) - 只返回结果
	fmt.Println("\n6. 调用 ResOnly 方法:")
	res, err = comm.ResOnly(req)
	if err != nil {
		fmt.Printf("   错误: %v\n", err)
	} else {
		fmt.Printf("   结果: %v\n", res)
	}

	// 7. ErrOnly - (Req) -> (error) - 只返回错误
	fmt.Println("\n7. 调用 ErrOnly 方法:")
	err = comm.ErrOnly(req)
	if err != nil {
		fmt.Printf("   错误: %v\n", err)
	} else {
		fmt.Printf("   成功: 无错误返回\n")
	}

	// 8. NoReturn - (Req) -> () - 无返回值
	fmt.Println("\n8. 调用 NoReturn 方法:")
	comm.NoReturn(req)
	fmt.Printf("   完成: 无返回值\n")

	// 9. Empty - () -> () - 无参无返
	fmt.Println("\n9. 调用 Empty 方法:")
	comm.Empty()
	fmt.Printf("   完成: 无参数无返回值\n")

	// 10. CtxOnly - (Ctx) -> () - 仅Context
	fmt.Println("\n10. 调用 CtxOnly 方法:")
	comm.CtxOnly(ctx)
	fmt.Printf("   完成: 仅Context参数\n")

	// 11. CtxOnlyResErr - (Ctx) -> (Res, error)
	fmt.Println("\n11. 调用 CtxOnlyResErr 方法:")
	res, err = comm.CtxOnlyResErr(ctx)
	if err != nil {
		fmt.Printf("   错误: %v\n", err)
	} else {
		fmt.Printf("   结果: %v\n", res)
	}

	// 12. CtxOnlyRes - (Ctx) -> (Res)
	fmt.Println("\n12. 调用 CtxOnlyRes 方法:")
	res, err = comm.CtxOnlyRes(ctx)
	if err != nil {
		fmt.Printf("   错误: %v\n", err)
	} else {
		fmt.Printf("   结果: %v\n", res)
	}

	// 13. CtxOnlyErr - (Ctx) -> (error)
	fmt.Println("\n13. 调用 CtxOnlyErr 方法:")
	err = comm.CtxOnlyErr(ctx)
	if err != nil {
		fmt.Printf("   错误: %v\n", err)
	} else {
		fmt.Printf("   成功: 无错误返回\n")
	}

	// 14. NoParamsResErr - () -> (Res, error)
	fmt.Println("\n14. 调用 NoParamsResErr 方法:")
	res, err = comm.NoParamsResErr()
	if err != nil {
		fmt.Printf("   错误: %v\n", err)
	} else {
		fmt.Printf("   结果: %v\n", res)
	}

	// 15. NoParamsRes - () -> (Res)
	fmt.Println("\n15. 调用 NoParamsRes 方法:")
	res, err = comm.NoParamsRes()
	if err != nil {
		fmt.Printf("   错误: %v\n", err)
	} else {
		fmt.Printf("   结果: %v\n", res)
	}

	// 16. NoParamsErr - () -> (error)
	fmt.Println("\n16. 调用 NoParamsErr 方法:")
	err = comm.NoParamsErr()
	if err != nil {
		fmt.Printf("   错误: %v\n", err)
	} else {
		fmt.Printf("   成功: 无错误返回\n")
	}

	// 17. CtxMetaReqResErr - (Ctx, Meta, Req) -> (Res, error)
	fmt.Println("\n17. 调用 CtxMetaReqResErr 方法:")
	res, err = comm.CtxMetaReqResErr(ctx, meta, req)
	if err != nil {
		fmt.Printf("   错误: %v\n", err)
	} else {
		fmt.Printf("   结果: %v\n", res)
	}

	// 18. CtxMetaReqRes - (Ctx, Meta, Req) -> (Res)
	fmt.Println("\n18. 调用 CtxMetaReqRes 方法:")
	res, err = comm.CtxMetaReqRes(ctx, meta, req)
	if err != nil {
		fmt.Printf("   错误: %v\n", err)
	} else {
		fmt.Printf("   结果: %v\n", res)
	}

	// 19. CtxMetaReqErr - (Ctx, Meta, Req) -> (error)
	fmt.Println("\n19. 调用 CtxMetaReqErr 方法:")
	err = comm.CtxMetaReqErr(ctx, meta, req)
	if err != nil {
		fmt.Printf("   错误: %v\n", err)
	} else {
		fmt.Printf("   成功: 无错误返回\n")
	}

	// 20. CtxMetaReq - (Ctx, Meta, Req) -> ()
	fmt.Println("\n20. 调用 CtxMetaReq 方法:")
	comm.CtxMetaReq(ctx, meta, req)
	fmt.Printf("   完成: 无返回值\n")

	fmt.Println("\n========== Client4 调用完成 ==========")
}