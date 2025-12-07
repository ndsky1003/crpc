package client

import "github.com/ndsky1003/crpc/v3/comm/ut"

func Options() *Option {
	return &Option{}
}

type Option struct {
	Name   *string
	Weight *int
}

func (o *Option) SetName(name string) *Option {
	o.Name = &name
	return o
}

func (o *Option) SetWeight(weight int) *Option {
	o.Weight = &weight
	return o
}

// 自动生成merge方法
func (o *Option) merge(other *Option) *Option {
	if other == nil || o == nil {
		return o
	}
	ut.ResolveOption(&o.Name, other.Name)
	ut.ResolveOption(&o.Weight, other.Weight)
	return o
}

// NOTE: 不要返回指针
// 全局对象来merge,
// 先浅拷贝全局对象,然后再修改这个拷贝对象,最后返回这个拷贝对象,这是避免逃逸到堆上,性能最高
func (o Option) Merge(opts ...*Option) Option {
	for _, opt := range opts {
		o.merge(opt)
	}
	return o
}
