package main

import (
	"context"
	"fmt"

	"github.com/ndsky1003/crpc/v3/server"
)

func main() {
	fmt.Println("start")
	server.New(context.Background()).Listen(":8080")
	fmt.Println("dd")
}
