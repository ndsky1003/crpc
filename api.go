package crpc

import (
	"context"

	"github.com/ndsky1003/crpc/v3/client"
	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/crpc/v3/protocol/errors"
	"github.com/ndsky1003/crpc/v3/server"
)

// --------------coder------------------
func RegisterCoder(t coder.T, c coder.Coder) {
	coder.RegisterCoder(t, c)
}

// --------------error------------------
type Error = errors.Error

func NewError(code uint16, msg string) *Error {
	return errors.New(code, msg)
}

func NewErrorf(code uint16, msg string, args ...any) *Error {
	return errors.Newf(code, msg, args...)
}

// --------------client------------------
type Client = client.Client

type ClientOption = client.Option

func ClientOptions() *ClientOption {
	return client.Options()
}

func Dial(ctx context.Context, name string, addr string, opts ...*client.Option) (c *Client, err error) {
	return client.New(ctx, name, addr, opts...)
}

// --------------server------------------
type Server = server.Server

type ServerOption = server.Option

func ServerOptions() *ServerOption {
	return server.Options()
}

func NewServer(ctx context.Context, opts ...*server.Option) *Server {
	return server.New(ctx, opts...)
}
