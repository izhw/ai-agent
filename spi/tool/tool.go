// Package tool defines tools without exposing Eino types.
package tool

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
)

type Parameter struct {
	Type        string               `json:"type"`
	Description string               `json:"description,omitempty"`
	Required    bool                 `json:"required,omitempty"`
	Enum        []string             `json:"enum,omitempty"`
	Properties  map[string]Parameter `json:"properties,omitempty"`
	Items       *Parameter           `json:"items,omitempty"`
}

type Descriptor struct {
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Parameters  map[string]Parameter `json:"parameters,omitempty"`
	RiskLevel   string               `json:"risk_level,omitempty"`
	Idempotent  bool                 `json:"idempotent,omitempty"`
}

type Tool interface {
	Descriptor() Descriptor
	Invoke(context.Context, json.RawMessage) (json.RawMessage, error)
}

type Resolver interface {
	Resolve(context.Context, []string) ([]Tool, error)
}

type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry(tools ...Tool) *Registry {
	r := &Registry{tools: make(map[string]Tool)}
	for _, item := range tools {
		if err := r.Register(item); err != nil {
			panic(err)
		}
	}
	return r
}

func (r *Registry) Register(item Tool) error {
	if item == nil || item.Descriptor().Name == "" {
		return errors.New("tool: name is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[item.Descriptor().Name]; exists {
		return errors.New("tool: duplicate name")
	}
	r.tools[item.Descriptor().Name] = item
	return nil
}

func (r *Registry) Resolve(_ context.Context, refs []string) ([]Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Tool, 0, len(refs))
	for _, ref := range refs {
		item, ok := r.tools[ref]
		if !ok {
			return nil, errors.New("tool: not found: " + ref)
		}
		result = append(result, item)
	}
	return result, nil
}
