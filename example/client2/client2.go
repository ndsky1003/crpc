package main

import (
	"context"
	"fmt"
	"time"

	"github.com/ndsky1003/crpc/v3/client"
	"github.com/ndsky1003/crpc/v3/example/comm"
	"github.com/ndsky1003/crpc/v3/example/dto"
)

var c *client.Client

func main() {
	c, _ = client.Dial(context.Background(), "client2", ":8080")
	comm.Default_lient = c
	c.UseByIndex(0, func(c *client.Context) {
		now := time.Now()
		c.Next()
		fmt.Println("usetime:", time.Since(now))
	})
	time.AfterFunc(2e9, run)
	select {}
}

func run() {
	// ctx := context.Background()
	// var meta = dto.Meta{Source: "client2"}
	var req = &dto.Req{Name: "llll"}
	// var req = "ddd"
	// var res Res
	res, _ := comm.ResOnly(req)
	fmt.Println(res)
	// err := c.Call(context.Background(), "client1", "cc.FnCtxOnly", req, &res, client.Options().SetMeta(meta).SetTraceID("traceid-client2-001"))
	// fmt.Println("client2:", " call result:", res, err)

}
