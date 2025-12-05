package client

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
	if other == nil {
		return o
	}
	if other.Name != nil {
		o.Name = other.Name
	}
	if other.Weight != nil {
		o.Weight = other.Weight
	}
	return o
}

func (o *Option) Merge(opts ...*Option) *Option {
	for _, opt := range opts {
		o.merge(opt)
	}
	return o
}
