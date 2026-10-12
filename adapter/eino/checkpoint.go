package eino

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/izhw/ai-agent/contract"
	storagespi "github.com/izhw/ai-agent/spi/storage"
)

const frameworkVersion = "v0.9.21"

type checkpointAdapter struct {
	store storagespi.CheckpointStore
	ttl   time.Duration
}

func (a checkpointAdapter) Get(ctx context.Context, id string) ([]byte, bool, error) {
	value, err := a.store.Get(ctx, id)
	if errors.Is(err, storagespi.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if value.Framework != "eino" || value.FrameworkVersion != frameworkVersion ||
		value.CodecVersion != "gob-v1" {
		return nil, false, errors.New(
			"eino adapter: incompatible checkpoint version; migrate or resume with the original runtime",
		)
	}
	if !value.ExpiresAt.IsZero() && value.ExpiresAt.Before(time.Now().UTC()) {
		if err := a.store.Delete(ctx, id); err != nil {
			return nil, false, fmt.Errorf("delete expired checkpoint: %w", err)
		}
		return nil, false, nil
	}
	return value.Payload, true, nil
}

func (a checkpointAdapter) Set(ctx context.Context, id string, payload []byte) error {
	now := time.Now().UTC()
	expires := time.Time{}
	if a.ttl > 0 {
		expires = now.Add(a.ttl)
	}
	return a.store.Set(
		ctx,
		contract.Checkpoint{
			ID:               id,
			Payload:          payload,
			Framework:        "eino",
			FrameworkVersion: frameworkVersion,
			CodecVersion:     "gob-v1",
			ExpiresAt:        expires,
			UpdatedAt:        now,
		},
	)
}

func (a checkpointAdapter) Delete(ctx context.Context, id string) error {
	return a.store.Delete(ctx, id)
}
