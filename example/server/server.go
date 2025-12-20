package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ndsky1003/crpc/v3/server"
)

func main() {
	fmt.Println("start")
	s, err := server.New(context.Background(), server.Options().SetSecret("ddddd"))
	if err != nil {
		slog.Error("e", "err", err)
		return
	}

	s.Listen(":8080")
	fmt.Println("dd")
}
