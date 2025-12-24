package main

//go:generate msgp

// OrderReq 订单请求
type OrderReq struct {
	OrderID      string   `msg:"oid"`
	UserID       string   `msg:"uid"`
	ProductIDs   []string `msg:"pids"`
	Amount       int64    `msg:"amt"`
	Status       string   `msg:"status"`
	PaymentType  string   `msg:"pay"`
	ReceiverInfo string   `msg:"recv"`
	Timestamp    int64    `msg:"ts"`
	Metadata     []string `msg:"meta"`
}
