package server

import (
	"math"
	"sync"

	"github.com/ndsky1003/crpc/v3/protocol/header"
	"github.com/ndsky1003/net/server"
)

type HandlerFunc func(c *Context)
type HandlersChain []HandlerFunc

// Context 服务端上下文
type Context struct {
	Sess server.Session

	// 原始数据信息
	Header    *header.Header
	MetaBytes []byte // 原始 Meta 数据
	BodyBytes []byte // 原始 Body 数据

	// 错误处理
	Err error

	// 内部控制
	handlers HandlersChain
	index    int8

	// 允许中间件存储一些 K-V 信息 (比如 UserID)
	Keys map[string]any
}

func (c *Context) Next() {
	c.index++
	for c.index < int8(len(c.handlers)) {
		c.handlers[c.index](c)
		c.index++
	}
}

func (c *Context) Abort() {
	c.index = int8(math.MaxInt8 / 2)
}

func (c *Context) IsAborted() bool {
	return c.index >= int8(math.MaxInt8/2)
}

// Set 存储数据
func (c *Context) Set(key string, value any) {
	if c.Keys == nil {
		c.Keys = make(map[string]any)
	}
	c.Keys[key] = value
}

// Get 获取数据
func (c *Context) Get(key string) (any, bool) {
	if c.Keys == nil {
		return nil, false
	}
	val, ok := c.Keys[key]
	return val, ok
}

func (c *Context) reset() {
	c.Sess = nil
	c.Header = nil
	c.MetaBytes = nil
	c.BodyBytes = nil
	c.Err = nil
	c.handlers = nil
	c.index = -1
	c.Keys = nil
}

// releaseContext 释放 Context
func (c *Context) releaseContext() {
	c.reset()
	serverContextPool.Put(c)
}

var serverContextPool = sync.Pool{
	New: func() any {
		return &Context{index: -1}
	},
}

func obtainContext() *Context {
	return serverContextPool.Get().(*Context)
}
