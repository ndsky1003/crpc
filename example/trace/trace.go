package trace

import "context"

type tracekey struct {
}

func WithTraceID(ctx context.Context, tid string) context.Context {
	if tid != "" {
		ctx = context.WithValue(ctx, tracekey{}, tid)
	}
	return ctx
}

func ExtractorTraceID(ctx context.Context) string {
	if v := ctx.Value(tracekey{}); v != nil {
		if vv, ok := v.(string); ok {
			return vv
		}
	}
	return ""
}
