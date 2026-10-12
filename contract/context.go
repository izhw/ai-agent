package contract

import "context"

type runKey struct{}

// WithRun exposes trusted scope to tools; callers must never derive it from model arguments.
func WithRun(ctx context.Context, run Run) context.Context {
	return context.WithValue(ctx, runKey{}, run)
}

func RunFromContext(ctx context.Context) (Run, bool) {
	v, ok := ctx.Value(runKey{}).(Run)
	return v, ok
}
