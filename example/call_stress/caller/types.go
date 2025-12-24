package main

//go:generate msgp

// OrderReq 订单请求(真实业务场景 - 电商订单处理)
type OrderReq struct {
	OrderID      string   `msg:"oid"`      // 订单ID
	UserID       string   `msg:"uid"`      // 用户ID
	ProductIDs   []string `msg:"pids"`     // 商品ID列表(多个商品)
	Amount       int64    `msg:"amt"`      // 订单金额(分)
	Status       string   `msg:"status"`   // 订单状态: PENDING/PAID/SHIPPED
	PaymentType  string   `msg:"pay"`      // 支付方式: alipay/wechat/card
	ReceiverInfo string   `msg:"recv"`     // 收货信息JSON
	Timestamp    int64    `msg:"ts"`       // 下单时间戳(微秒)
	Metadata     []string `msg:"meta"`     // 扩展元数据(渠道、来源等)
}

// OrderRes 订单响应
type OrderRes struct {
	OrderID     string `msg:"oid"`    // 订单ID
	Status      string `msg:"status"` // 处理后状态
	OrderAmount int64  `msg:"amt"`    // 订单金额
	ProcessedAt int64  `msg:"pat"`    // 处理时间戳
	Message     string `msg:"msg"`    // 处理消息
}
