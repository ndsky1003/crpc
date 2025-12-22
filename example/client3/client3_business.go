//go:generate gencrpcserverv3
//go:generate gencrpcclientv3 --out_dir ../comm --package comm --client Default_Client --service client3 --module  BusinessService
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

// ==========================================
// 业务服务结构体
// ==========================================
type BusinessService struct{}

// ==========================================
// 用户相关服务
// ==========================================

// 查询用户信息
// @crpc:CallType:Call,Send,Go
func (s *BusinessService) QueryUser(ctx context.Context, req *dto.BusinessReq) (*dto.BusinessRes, error) {
	// // fmt.Printf("[BusinessService] QueryUser Called: RequestID=%s\n", req.RequestID)

	// 模拟业务处理
	userID := req.UserInfo.UserID
	if userID == "" {
		// 如果请求中没有用户信息，使用RequestID模拟
		userID = fmt.Sprintf("user_%s", req.RequestID[:8])
	}

	// 返回用户信息
	user := dto.CreateTestUser(int(time.Now().UnixNano() % 10000))
	user.UserID = userID

	return &dto.BusinessRes{
		UserInfo:   user,
		Code:       0,
		Message:    "查询成功",
		Success:    true,
		RequestID:  req.RequestID,
		TraceID:    req.TraceID,
		Timestamp:  time.Now().Unix(),
	}, nil
}

// 更新用户信息
// @crpc:CallType:Call,Send,Go
func (s *BusinessService) UpdateUser(ctx context.Context, req *dto.BusinessReq) (*dto.BusinessRes, error) {
	// // fmt.Printf("[BusinessService] UpdateUser Called: RequestID=%s\n", req.RequestID)

	// 模拟更新用户
	if req.UserInfo != nil {
		// 实际业务中这里会更新数据库
	}

	return &dto.BusinessRes{
		Code:       0,
		Message:    "更新成功",
		Success:    true,
		RequestID:  req.RequestID,
		TraceID:    req.TraceID,
		Timestamp:  time.Now().Unix(),
	}, nil
}

// ==========================================
// 订单相关服务
// ==========================================

// 查询订单列表
// @crpc:CallType:Call,Send,Go
func (s *BusinessService) QueryOrders(ctx context.Context, req *dto.BusinessReq) (*dto.BusinessRes, error) {
	// // fmt.Printf("[BusinessService] QueryOrders Called: RequestID=%s\n", req.RequestID)

	// 模拟查询订单列表
	orders := make([]*dto.OrderInfo, 10)
	pageSize := int32(10)
	if req.QueryParam != nil && req.QueryParam.PageSize > 0 {
		pageSize = req.QueryParam.PageSize
	}

	for i := 0; i < int(pageSize); i++ {
		orders[i] = dto.CreateTestOrder(int(time.Now().UnixNano() + int64(i*1000)))
		if req.QueryParam != nil {
			orders[i].UserID = req.QueryParam.Filters["user_id"]
		}
	}

	return &dto.BusinessRes{
		Orders:     orders,
		TotalCount: 1000, // 模拟总数
		PageNum:    1,
		PageSize:   pageSize,
		Code:       0,
		Message:    "查询成功",
		Success:    true,
		RequestID:  req.RequestID,
		TraceID:    req.TraceID,
		Timestamp:  time.Now().Unix(),
	}, nil
}

// 创建订单
// @crpc:CallType:Call,Send,Go
func (s *BusinessService) CreateOrder(ctx context.Context, req *dto.BusinessReq) (*dto.BusinessRes, error) {
	// // fmt.Printf("[BusinessService] CreateOrder Called: RequestID=%s\n", req.RequestID)

	var order *dto.OrderInfo
	if req.OrderInfo != nil {
		// 使用请求中的订单信息
		order = req.OrderInfo
		order.CreateTime = time.Now()
		order.UpdateTime = time.Now()
	} else {
		// 创建新订单
		order = dto.CreateTestOrder(int(time.Now().UnixNano()))
	}

	return &dto.BusinessRes{
		OrderInfo:  order,
		Code:       0,
		Message:    "创建成功",
		Success:    true,
		RequestID:  req.RequestID,
		TraceID:    req.TraceID,
		Timestamp:  time.Now().Unix(),
	}, nil
}

// ==========================================
// 商品相关服务
// ==========================================

// 搜索商品
// @crpc:CallType:Call,Send,Go
func (s *BusinessService) SearchProducts(ctx context.Context, req *dto.BusinessReq) (*dto.BusinessRes, error) {
	// // fmt.Printf("[BusinessService] SearchProducts Called: RequestID=%s\n", req.RequestID)

	// 模拟商品搜索
	products := make([]*dto.ProductInfo, 20)
	pageSize := int32(20)
	if req.QueryParam != nil && req.QueryParam.PageSize > 0 {
		pageSize = req.QueryParam.PageSize
	}

	for i := 0; i < int(pageSize); i++ {
		products[i] = dto.CreateTestProduct(int(time.Now().UnixNano() + int64(i*1000)))
		if req.QueryParam != nil {
			products[i].Name = req.QueryParam.Keywords + fmt.Sprintf(" %d", i)
			if req.QueryParam.CategoryID != "" {
				products[i].CategoryID = req.QueryParam.CategoryID
			}
			if req.QueryParam.PriceMin > 0 {
				products[i].Price = req.QueryParam.PriceMin + float64(i)*10
			}
		}
	}

	return &dto.BusinessRes{
		Products:   products,
		TotalCount: 5000, // 模拟总数
		PageNum:    1,
		PageSize:   pageSize,
		Code:       0,
		Message:    "搜索成功",
		Success:    true,
		RequestID:  req.RequestID,
		TraceID:    req.TraceID,
		Timestamp:  time.Now().Unix(),
	}, nil
}

// 获取商品详情
// @crpc:CallType:Call,Send,Go
func (s *BusinessService) GetProductDetail(ctx context.Context, req *dto.BusinessReq) (*dto.BusinessRes, error) {
	// // fmt.Printf("[BusinessService] GetProductDetail Called: RequestID=%s\n", req.RequestID)

	var product *dto.ProductInfo
	if req.Products != nil && len(req.Products) > 0 {
		product = req.Products[0]
	} else {
		// 根据请求ID生成商品
		product = dto.CreateTestProduct(int(time.Now().UnixNano()))
	}

	return &dto.BusinessRes{
		Products:   []*dto.ProductInfo{product},
		Code:       0,
		Message:    "获取成功",
		Success:    true,
		RequestID:  req.RequestID,
		TraceID:    req.TraceID,
		Timestamp:  time.Now().Unix(),
	}, nil
}

// ==========================================
// 购物车相关服务
// ==========================================

// 获取购物车
// @crpc:CallType:Call,Send,Go
func (s *BusinessService) GetCart(ctx context.Context, req *dto.BusinessReq) (*dto.BusinessRes, error) {
	// // fmt.Printf("[BusinessService] GetCart Called: RequestID=%s\n", req.RequestID)

	cart := req.CartInfo
	if cart == nil {
		// 创建空购物车
		cart = &dto.CartInfo{
			CartID:     fmt.Sprintf("cart_%s", req.RequestID[:8]),
			UserID:     fmt.Sprintf("user_%s", req.RequestID[:8]),
			Items:      make([]dto.CartItemInfo, 0),
			UpdateTime: time.Now(),
		}
	}

	return &dto.BusinessRes{
		CartInfo:   cart,
		Code:       0,
		Message:    "获取成功",
		Success:    true,
		RequestID:  req.RequestID,
		TraceID:    req.TraceID,
		Timestamp:  time.Now().Unix(),
	}, nil
}

// 更新购物车
// @crpc:CallType:Call,Send,Go
func (s *BusinessService) UpdateCart(ctx context.Context, req *dto.BusinessReq) (*dto.BusinessRes, error) {
	// // fmt.Printf("[BusinessService] UpdateCart Called: RequestID=%s\n", req.RequestID)

	cart := req.CartInfo
	if cart != nil {
		// 计算总金额
		totalAmount := 0.0
		for _, item := range cart.Items {
			totalAmount += item.Price * float64(item.Quantity)
		}
		cart.TotalAmount = totalAmount
		cart.UpdateTime = time.Now()
	}

	return &dto.BusinessRes{
		CartInfo:   cart,
		Code:       0,
		Message:    "更新成功",
		Success:    true,
		RequestID:  req.RequestID,
		TraceID:    req.TraceID,
		Timestamp:  time.Now().Unix(),
	}, nil
}

// ==========================================
// 综合服务
// ==========================================

// 批量查询 - 模拟复杂业务场景
// @crpc:CallType:Call,Send,Go
func (s *BusinessService) BatchQuery(ctx context.Context, req *dto.BusinessReq) (*dto.BusinessRes, error) {
	// // fmt.Printf("[BusinessService] BatchQuery Called: RequestID=%s\n", req.RequestID)

	// 返回综合数据
	res := &dto.BusinessRes{
		Code:       0,
		Message:    "批量查询成功",
		Success:    true,
		RequestID:  req.RequestID,
		TraceID:    req.TraceID,
		Timestamp:  time.Now().Unix(),
	}

	// 根据请求参数返回不同数据
	if req.QueryParam != nil {
		// 查询用户
		user := dto.CreateTestUser(1)
		user.UserID = req.QueryParam.Filters["user_id"]
		res.UserInfo = user

		// 查询订单
		orders := make([]*dto.OrderInfo, 5)
		for i := 0; i < 5; i++ {
			orders[i] = dto.CreateTestOrder(i)
			orders[i].UserID = user.UserID
		}
		res.Orders = orders
		res.TotalCount = 100

		// 查询商品
		products := make([]*dto.ProductInfo, 10)
		for i := 0; i < 10; i++ {
			products[i] = dto.CreateTestProduct(i)
		}
		res.Products = products
	}

	return res, nil
}

// ==========================================
// Main 函数
// ==========================================
func main() {
	log.SetDefault(log.Options().SetExtractorAttr(func(ctx context.Context, r *slog.Record) {
		if tid := trace.ExtractorTraceID(ctx); tid != "" {
			r.Add("trace_id", tid)
		}
	}).SetAddSource(true))

	// 初始化 Client
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
	fmt.Println("Client3 (Business Service) started...")

	// 实例化服务对象
	svc := &BusinessService{}

	// 注册服务
	if err := c.RegisterName("BusinessService", svc); err != nil {
		fmt.Printf("RegisterName error: %v\n", err)
	} else {
		fmt.Println("Registered 'BusinessService' successfully")
	}

	// 等待程序退出
	select {}
}