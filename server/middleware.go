// 放弃池化
package server

import (
	"maps"
	"math"
	"sync"

	"github.com/ndsky1003/crpc/v3/protocol/header"
	"github.com/ndsky1003/net/v2/server"
)

type HandlerFunc func(c *Context)
type HandlersChain []HandlerFunc

// Context 服务端上下文
type Context struct {
	Sess server.Session

	wg *sync.WaitGroup
	// 原始数据信息
	Header *header.Header //这个是池化的
	Data   []byte         //整体的数据

	// 错误处理
	err error

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

func (c *Context) SetError(err error) {
	c.err = err
}

func (c *Context) Err() error {
	return c.err
}

func (c *Context) AbortWithError(err error) {
	c.SetError(err)
	c.Abort()
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

func (c *Context) Clone() *Context {
	// 1. 浅拷贝 (复制 index, err, Sess 等值类型/接口)
	ctx := *c

	// 2. 【关键】深拷贝 Header (解耦 Header 池化)
	if c.Header != nil {
		ctx.Header = c.Header.Clone()
	}

	// 3. 【关键】深拷贝 Keys (防止并发读写 Map Panic)
	if c.Keys != nil {
		newKeys := make(map[string]any, len(c.Keys))
		maps.Copy(newKeys, c.Keys)
		ctx.Keys = newKeys
	} else {
		ctx.Keys = nil // 确保新对象是干净的
	}

	// 4. 【关键】深拷贝 Body/Meta (防止 Use-After-Free)
	// 因为原 Context 的 Bytes 指向的是 buffer pool，请求结束会被回收。
	// 异步任务必须拥有自己独立的内存副本。
	if len(c.Data) > 0 {
		ctx.Data = make([]byte, len(c.Data))
		copy(ctx.Data, c.Data)
	}

	// 注意：handlers 链不需要拷贝，因为它是只读的函数切片，且长期存在。
	// Sess (Session) 也不需要拷贝，因为 net.Conn 是线程安全的。

	return &ctx
}
