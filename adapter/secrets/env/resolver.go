// Package env resolves secret references from environment variables.
package env

import (
	"context"
	"errors"
	"os"
)

type Resolver struct{}

func (Resolver) Resolve(_ context.Context, ref string) (string, error) {
	value, ok := os.LookupEnv(ref)
	if !ok || value == "" {
		return "", errors.New("secret environment variable not found: " + ref)
	}
	return value, nil
}
