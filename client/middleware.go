// 放弃池化，这复杂度，特别是Go调用后面必须记得放回池子，增加心智负担
package client

import (
	"context"
	"math"

	"github.com/ndsky1003/crpc/v3/protocol/header"
	"github.com/ndsky1003/crpc/v3/protocol/header/headertype"
)

// HandlerFunc 定义中间件处理函数
type HandlerFunc func(c *Context)

// HandlersChain 中间件链
type HandlersChain []HandlerFunc

type responseHook func(ret any, err error)

type Context struct {
	// --- 基础输入 (由 Call/Go/Send 传入) ---
	Ctx      context.Context
	Service  string
	Method   string
	CallType headertype.T // 新增: 记录是 Req, Res 还是 Send
	Args     any
	Reply    any

	// --- 中间状态 (由中间件产生) ---
	MergedOpt *Option        // 合并后的 Option
	Seq       uint64         // 请求序列号
	Header    *header.Header // 协议头
	Module    string         // 解析出的模块名
	Func      string         // 解析出的方法名
	MetaBytes []byte         // 序列化后的 Meta
	BodyBytes []byte         // 序列化后的 Body

	// --- 结果信息 ---
	Call *Call // 关联的 Call 对象
	err  error // 发送错误或回包错误

	// --- 内部状态 ---
	handlers HandlersChain
	index    int8
	//NOTE:避免池化，将这个控制权移交到Call上面，共用call的声明周期，
	//又要保证，这个回调里没有使用Context的代码，因为Context大概率已经放回池子了。
	//这是用户调用了的，没法保证。
	hooks []responseHook
}

// Next 执行下一个中间件 ,这里的for是容错率更高
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
