package client

import (
	"context"

	"github.com/ndsky1003/crpc/v3/protocol/errors"
	"github.com/ndsky1003/crpc/v3/protocol/header/headertype"
)

func (c *Client) Call(ctx context.Context, serviceName, method string, args, reply any, opts ...*Option) error {
	call := c.Go(ctx, serviceName, method, args, reply, opts...)
	if call.Error != nil {
		return call.Error
	}
	select {
	case <-ctx.Done():
		call.Error = errors.New(errors.ClientInternal, ctx.Err().Error())
		c.pending.Delete(call.seq)
		return ctx.Err()
	case call := <-call.Done:
		return call.Error
	}
}

func (c *Client) Go(ctx context.Context, serviceName, method string, args, reply any, opts ...*Option) (call *Call) {
	return c._go(ctx, headertype.Req, serviceName, method, args, reply, opts...)
}

func (c *Client) Send(ctx context.Context, serviceName, method string, args any, opts ...*Option) error {
	call := c._go(ctx, headertype.Send, serviceName, method, args, nil, opts...)
	return call.Error
}
