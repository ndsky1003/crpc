//go:generate msgp
package dto

import (
	"fmt"
	"time"
)

// 用户信息
type UserInfo struct {
	UserID      string    `msg:"user_id"`
	Username    string    `msg:"username"`
	Email       string    `msg:"email"`
	Phone       string    `msg:"phone"`
	Avatar      string    `msg:"avatar"`
	CreateTime  time.Time `msg:"create_time"`
	UpdateTime  time.Time `msg:"update_time"`
	Status      int32     `msg:"status"`
	IsVIP       bool      `msg:"is_vip"`
}

// 订单信息
type OrderInfo struct {
	OrderID        string    `msg:"order_id"`
	UserID         string    `msg:"user_id"`
	OrderNo        string    `msg:"order_no"`
	TotalAmount    float64   `msg:"total_amount"`
	PayAmount      float64   `msg:"pay_amount"`
	DiscountAmount float64   `msg:"discount_amount"`
	Status         int32     `msg:"status"`
	PayMethod      string    `msg:"pay_method"`
	PayTime        time.Time `msg:"pay_time"`
	DeliveryAddr   string    `msg:"delivery_addr"`
	Remark         string    `msg:"remark"`
	CreateTime     time.Time `msg:"create_time"`
	UpdateTime     time.Time `msg:"update_time"`
}

// 商品信息
type ProductInfo struct {
	ProductID    string    `msg:"product_id"`
	CategoryID   string    `msg:"category_id"`
	Name         string    `msg:"name"`
	Description  string    `msg:"description"`
	Price        float64   `msg:"price"`
	Stock        int32     `msg:"stock"`
	Sales        int32     `msg:"sales"`
	Images       []string  `msg:"images"`
	Tags         []string  `msg:"tags"`
	CreateTime   time.Time `msg:"create_time"`
	UpdateTime   time.Time `msg:"update_time"`
	IsActive     bool      `msg:"is_active"`
}

// 购物车信息
type CartInfo struct {
	CartID      string           `msg:"cart_id"`
	UserID      string           `msg:"user_id"`
	Items       []CartItemInfo   `msg:"items"`
	TotalAmount float64          `msg:"total_amount"`
	UpdateTime  time.Time        `msg:"update_time"`
}

type CartItemInfo struct {
	ProductID string  `msg:"product_id"`
	Name      string  `msg:"name"`
	Price     float64 `msg:"price"`
	Quantity  int32   `msg:"quantity"`
}

// 业务请求结构体
type BusinessReq struct {
	// 用户相关
	UserInfo *UserInfo `msg:"user_info,omitempty"`

	// 订单相关
	OrderInfo *OrderInfo `msg:"order_info,omitempty"`

	// 商品相关
	Products  []*ProductInfo `msg:"products,omitempty"`
	CartInfo  *CartInfo      `msg:"cart_info,omitempty"`

	// 查询条件
	QueryParam *QueryParam `msg:"query_param,omitempty"`

	// 元数据
	RequestID  string `msg:"request_id"`
	TraceID    string `msg:"trace_id"`
	ClientType string `msg:"client_type"`
	Version    string `msg:"version"`
	Timestamp  int64  `msg:"timestamp"`
}

// 查询参数
type QueryParam struct {
	PageNum     int32             `msg:"page_num"`
	PageSize    int32             `msg:"page_size"`
	SortBy      string            `msg:"sort_by"`
	SortOrder   string            `msg:"sort_order"`
	Filters     map[string]string `msg:"filters"`
	Keywords    string            `msg:"keywords"`
	CategoryID  string            `msg:"category_id"`
	PriceMin    float64           `msg:"price_min"`
	PriceMax    float64           `msg:"price_max"`
}

// 业务响应结构体
type BusinessRes struct {
	// 响应数据
	UserInfo   *UserInfo        `msg:"user_info,omitempty"`
	OrderInfo  *OrderInfo       `msg:"order_info,omitempty"`
	Products   []*ProductInfo   `msg:"products,omitempty"`
	CartInfo   *CartInfo        `msg:"cart_info,omitempty"`
	Orders     []*OrderInfo     `msg:"orders,omitempty"`

	// 分页信息
	TotalCount int64  `msg:"total_count"`
	PageNum    int32  `msg:"page_num"`
	PageSize   int32  `msg:"page_size"`

	// 状态信息
	Code       int32  `msg:"code"`
	Message    string `msg:"message"`
	Success    bool   `msg:"success"`
	RequestID  string `msg:"request_id"`
	Timestamp  int64  `msg:"timestamp"`
	TraceID    string `msg:"trace_id"`
}

// 创建一些工厂方法来生成测试数据

func CreateTestUser(id int) *UserInfo {
	return &UserInfo{
		UserID:     fmt.Sprintf("user_%d", id),
		Username:   fmt.Sprintf("username_%d", id),
		Email:      fmt.Sprintf("user%d@example.com", id),
		Phone:      fmt.Sprintf("138%08d", id%100000000),
		Avatar:     fmt.Sprintf("https://avatar.example.com/%d.jpg", id),
		CreateTime: time.Now().Add(-time.Hour * 24 * time.Duration(id%365)),
		UpdateTime: time.Now(),
		Status:     1,
		IsVIP:      id%10 == 0,
	}
}

func CreateTestOrder(id int) *OrderInfo {
	return &OrderInfo{
		OrderID:        fmt.Sprintf("order_%d", id),
		UserID:         fmt.Sprintf("user_%d", id%10000),
		OrderNo:        fmt.Sprintf("ORD%010d", id),
		TotalAmount:    float64(id%1000) + 99.99,
		PayAmount:      float64(id%800) + 79.99,
		DiscountAmount: 20.00,
		Status:         int32(id % 5),
		PayMethod:      []string{"alipay", "wechat", "card", "paypal"}[id%4],
		PayTime:        time.Now().Add(-time.Hour * time.Duration(id%24)),
		DeliveryAddr:   fmt.Sprintf("Address %d, Street %d, City %d", id%100, id%1000, id%100),
		Remark:         fmt.Sprintf("Remark for order %d", id),
		CreateTime:     time.Now().Add(-time.Hour * 24 * time.Duration(id%30)),
		UpdateTime:     time.Now(),
	}
}

func CreateTestProduct(id int) *ProductInfo {
	tags := []string{"hot", "new", "sale", "popular", "recommend"}
	images := make([]string, 3)
	for i := 0; i < 3; i++ {
		images[i] = fmt.Sprintf("https://img.example.com/product_%d_%d.jpg", id, i)
	}

	return &ProductInfo{
		ProductID:   fmt.Sprintf("product_%d", id),
		CategoryID:  fmt.Sprintf("cat_%d", id%100),
		Name:        fmt.Sprintf("Product Name %d", id),
		Description: fmt.Sprintf("This is a detailed description for product %d. It contains many details about the product including features, specifications, and other important information that customers need to know.", id),
		Price:       float64(id%1000) + 9.99,
		Stock:       int32(id % 10000),
		Sales:       int32(id % 5000),
		Images:      images,
		Tags:        []string{tags[id%len(tags)], tags[(id+1)%len(tags)]},
		CreateTime:  time.Now().Add(-time.Hour * 24 * time.Duration(id%365)),
		UpdateTime:  time.Now(),
		IsActive:    id%100 != 0,
	}
}