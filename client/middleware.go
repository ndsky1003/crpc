package client

import (
	"context"
	"math"
	"sync"
)

// HandlerFunc 定义中间件处理函数
type HandlerFunc func(c *Context)

// HandlersChain 中间件链
type HandlersChain []HandlerFunc

// Context 客户端请求上下文
// 使用 sync.Pool 复用，避免 GC 压力
type Context struct {
	// 基础信息
	Ctx context.Context // Go 标准库 Context，支持修改 (Timeout, Trace 等)

	// 请求参数 (中间件可读写)
	ServiceName string
	Method      string
	Args        any
	Reply       any
	Opts        []*Option

	// 结果信息
	Call *Call // 关联的 Call 对象
	Err  error // 发送错误或回包错误

	// 内部状态
	handlers HandlersChain
	index    int8

	// 异步回调钩子：用于在收到回包后执行逻辑 (统计耗时等)
	hooks []func(err error)
}

// Next 执行下一个中间件
func (c *Context) Next() {
	c.index++
	for c.index < int8(len(c.handlers)) {
		c.handlers[c.index](c)
		c.index++
	}
}

// Abort 终止处理链
func (c *Context) Abort() {
	c.index = int8(math.MaxInt8 / 2)
}

// IsAborted 检查是否已停止
func (c *Context) IsAborted() bool {
	return c.index >= int8(math.MaxInt8/2)
}

// OnResponse 注册回包后的回调函数 (支持异步)
// 这是解决异步中间件的关键
func (c *Context) OnResponse(fn func(err error)) {
	c.hooks = append(c.hooks, fn)
}

// invokeHooks 内部调用：当 Call 完成时触发
func (c *Context) invokeHooks(err error) {
	// 倒序执行，符合洋葱模型直觉 (后注册先执行)
	for i := len(c.hooks) - 1; i >= 0; i-- {
		c.hooks[i](err)
	}
}

// reset 重置状态以复用
func (c *Context) reset() {
	c.Ctx = nil
	c.ServiceName = ""
	c.Method = ""
	c.Args = nil
	c.Reply = nil
	c.Opts = nil
	c.Call = nil
	c.Err = nil
	c.handlers = nil
	c.index = -1
	// 复用切片底层数组，避免分配
	c.hooks = c.hooks[:0]
}

// releaseContext 释放自身回池
func (c *Context) releaseContext() {
	c.reset()
	contextPool.Put(c)
}

var contextPool = sync.Pool{
	New: func() any {
		return &Context{
			index: -1,
			hooks: make([]func(error), 0, 4), // 预分配一点空间
		}
	},
}

// obtainContext 包级私有获取方法
func obtainContext() *Context {
	return contextPool.Get().(*Context)
}
