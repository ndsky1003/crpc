// 放弃池化，这复杂度，特别是Go调用后面必须记得放回池子，增加心智负担
package client

import (
	"context"
	"math"
)

// HandlerFunc 定义中间件处理函数
type HandlerFunc func(c *Context)

// HandlersChain 中间件链
type HandlersChain []HandlerFunc

type responseHook func(ret any, err error)

// Context 客户端请求上下文
// 使用 sync.Pool 复用，避免 GC 压力
type Context struct {
	// 基础信息 ,透传给_go真正的调用 start
	Ctx context.Context // Go 标准库 Context，支持修改 (Timeout, Trace 等)
	// 请求参数 (中间件可读写)
	Service string
	Method  string
	Args    any
	Reply   any
	Opts    []*Option
	// 基础信息 ,透传给_go真正的调用 end

	// 结果信息
	Call *Call // 关联的 Call 对象
	err  error // 发送错误或回包错误

	// 内部状态 --start
	handlers HandlersChain
	index    int8
	// 异步回调钩子：用于在收到回包后执行逻辑 (统计耗时等)
	hooks []responseHook
	// 内部状态 --end
}

// Next 执行下一个中间件
func (c *Context) Next() {
	c.index++
	for c.index < int8(len(c.handlers)) {
		c.handlers[c.index](c)
		c.index++
	}
}

//NOTE: Abort 停止后续中间件的执行 ,具体的错误有c.Err来传递
// 很好理解，缓存击中，就不需要往下执行了，但是有没有报错
/*NOTE:
func CacheMiddleware(c *Context) {
    if cached, ok := getCache(c.Method); ok {
        c.Reply = cached
        c.Abort() // 语义清晰：请求处理结束，但这依然是一个"成功"的请求
    }
}
*/
func (c *Context) Abort() {
	c.index = int8(math.MaxInt8 / 2)
}

// SetError 设置上下文错误
// 这是一个覆盖操作，通常用于中间件记录遇到的错误
func (c *Context) SetError(err error) {
	c.err = err
}

// Err 获取当前请求的错误
// 统一了错误视图。
// 如果 Call 对象已经产生（说明请求已发出），优先返回 Call 的错误（因为它通常包含更底层的网络或业务错误）。
// 否则返回 Context 自身积累的错误（如中间件拦截错误）。
func (c *Context) Err() error {
	if c.Call != nil && c.Call.Error != nil {
		return c.Call.Error
	}
	return c.err
}

// AbortWithError 语法糖：设置错误并终止
func (c *Context) AbortWithError(err error) {
	c.SetError(err)
	c.Abort()
}

// IsAborted 检查是否已停止
func (c *Context) IsAborted() bool {
	return c.index >= int8(math.MaxInt8/2)
}

// OnResponse 注册回包后的回调函数 (支持异步)
// 这是解决异步中间件的关键
func (c *Context) OnResponse(fn func(ret any, err error)) {
	c.hooks = append(c.hooks, fn)
}

// invokeHooks 内部调用：当 Call 完成时触发
func (c *Context) invokeHooks(ret any, err error) {
	// 倒序执行，符合洋葱模型直觉 (后注册先执行)
	for i := len(c.hooks) - 1; i >= 0; i-- {
		c.hooks[i](ret, err)
	}
}
