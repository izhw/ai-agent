// Package secrets defines secret lookup without prescribing a vault product.
package secrets

import "context"

type Resolver interface {
	Resolve(context.Context, string) (string, error)
}

type ResolverFunc func(context.Context, string) (string, error)

func (f ResolverFunc) Resolve(
	ctx context.Context,
	ref string,
) (string, error) {
	return f(ctx, ref)
}
