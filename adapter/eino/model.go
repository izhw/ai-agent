package eino

import (
	"context"
	"errors"
	"sync"

	"github.com/cloudwego/eino/components/model"
)

type ModelResolver interface {
	Resolve(context.Context, string) (model.BaseChatModel, error)
}

type ModelRegistry struct {
	mu     sync.RWMutex
	models map[string]model.BaseChatModel
}

func NewModelRegistry() *ModelRegistry {
	return &ModelRegistry{models: make(map[string]model.BaseChatModel)}
}

func (r *ModelRegistry) Register(ref string, value model.BaseChatModel) error {
	if ref == "" || value == nil {
		return errors.New("eino adapter: model ref and model are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.models[ref]; exists {
		return errors.New("eino adapter: duplicate model ref: " + ref)
	}
	r.models[ref] = value
	return nil
}

func (r *ModelRegistry) Resolve(_ context.Context, ref string) (model.BaseChatModel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.models[ref]
	if !ok {
		return nil, errors.New("eino adapter: model not found: " + ref)
	}
	return value, nil
}
