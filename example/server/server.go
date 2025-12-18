package main

import (
	"context"
	"fmt"

	"github.com/ndsky1003/crpc/v3/server"
)

func main() {
	fmt.Println("start")
	s, _ := server.New(context.Background())
	s.Listen(":8080")
	fmt.Println("dd")
}
