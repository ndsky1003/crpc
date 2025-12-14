package client

import (
	"context"

	"github.com/ndsky1003/crpc/v3/protocol/errors"
	"github.com/ndsky1003/crpc/v3/protocol/header/headertype"
)

// Use 插入中间件
// 默认插入策略：为了保证业务逻辑生效，我们把用户中间件插在 Codec 之前 (倒数第2个位置之前)
// 这样用户可以修改 Args, Opts, Context 等，但不会破坏 Init/Header 的基础环境
func (c *Client) Use(middleware ...HandlerFunc) {
	n := len(c.handlers)
	insertIdx := max(n-2, 0)
	c.UseByIndex(insertIdx, middleware...)
}

func (c *Client) UseByIndex(index int, middleware ...HandlerFunc) {
	n := len(c.handlers)
	if index < 0 {
		index = 0
	}
	if index >= n {
		index = n
	}
	newHandlers := make(HandlersChain, 0, n+len(middleware))
	newHandlers = append(newHandlers, c.handlers[:index]...)
	newHandlers = append(newHandlers, middleware...)
	newHandlers = append(newHandlers, c.handlers[index:]...)
	c.handlers = newHandlers
}

// ResetHandlers 允许完全重置中间件链 (给高级用户)
func (c *Client) ResetMiddleware(chain HandlersChain) {
	c.handlers = chain
}

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

// Send 模式在 Transport 中间件里已经完成，且没有 pending 等待
func (c *Client) Send(ctx context.Context, service, method string, args any, opts ...*Option) error {
	call := c.executeChain(ctx, headertype.Send, service, method, args, nil, opts...)
	call.done()
	return call.Error
}

func (c *Client) Close() error {
	err := c.client.Close()
	c.serviceMap.Clear()
	c.handlers = nil //只清空引用就可以了
	return err
}
