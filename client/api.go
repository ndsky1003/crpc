package client

import (
	"context"

	"github.com/ndsky1003/crpc/v3/protocol/errors"
	"github.com/ndsky1003/crpc/v3/protocol/header/headertype"
)

func (c *Client) Call(ctx context.Context, service, method string, args, reply any, opts ...*Option) error {
	call := c.Go(ctx, service, method, args, reply, opts...)
	if call.Error != nil {
		return call.Error
	}
	select {
	case <-ctx.Done():
		err := ctx.Err()
		if err != nil {
			err = errors.New(errors.ClientInternal, err.Error())
		}
		if _, loaded := c.pending.LoadAndDelete(call.seq); loaded {
			call.Error = err
			call.done()
		}
		return err
	case call := <-call.Done:
		return call.Error
	}
}

func (c *Client) Go(ctx context.Context, service, method string, args, reply any, opts ...*Option) *Call {
	// return c._go(ctx, headertype.Req, serviceName, method, args, reply, opts...)
	return c.executeChain(ctx, headertype.Req, service, method, args, reply, opts...)
}

func (c *Client) Send(ctx context.Context, service, method string, args any, opts ...*Option) error {
	call := c.executeChain(ctx, headertype.Send, service, method, args, nil, opts...)
	if call.Error != nil {
		return call.Error
	}
	call.done()
	return nil
}

func (c *Client) Close() error {
	err := c.client.Close()
	c.pending.Clear()
	c.serviceMap.Clear()
	for i := range c.handlers {
		c.handlers[i] = nil
	}
	c.handlers = c.handlers[:0]
	return err
}
