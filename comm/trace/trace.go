package trace

import (
	"context"
)

type traceKey struct{}

func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, traceKey{}, id)
}

func GetTraceID(ctx context.Context) string {
	if val, ok := ctx.Value(traceKey{}).(string); ok {
		return val
	}
	return ""
}
