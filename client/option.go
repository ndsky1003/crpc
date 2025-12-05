package client

type CallOptions struct {
	TargetSid string
	Broadcast bool
}

type CallOption func(*CallOptions)

// WithTargetSid 指定调用 (需求 4)
func WithTargetSid(sid string) CallOption {
	return func(o *CallOptions) {
		o.TargetSid = sid
	}
}

// WithBroadcast 广播调用 (需求 4)
func WithBroadcast() CallOption {
	return func(o *CallOptions) {
		o.Broadcast = true
	}
}
