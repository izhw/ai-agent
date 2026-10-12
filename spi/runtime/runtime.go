// Package runtime defines the framework-independent agent runtime extension point.
//
//nolint:revive // The public SPI package is intentionally named runtime.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/izhw/ai-agent/contract"
)

var ErrRuntimeNotRegistered = errors.New("runtime: factory not registered")

type BuildInput struct {
	Spec contract.AgentSpec
}

type RunInput struct {
	Run          contract.Run
	Spec         contract.AgentSpec
	Messages     []contract.Message
	Variables    map[string]any
	CheckpointID string
}

type ResumeInput struct {
	Run          contract.Run
	Spec         contract.AgentSpec
	CheckpointID string
	Targets      map[string]any
}

type EventStream interface {
	Next(context.Context) (contract.RunEvent, error)
	Close() error
}

type Runtime interface {
	Run(context.Context, RunInput) (EventStream, error)
	Resume(context.Context, ResumeInput) (EventStream, error)
	Close() error
}

type Factory interface {
	Type() contract.RuntimeType
	Validate(context.Context, contract.AgentSpec) error
	Build(context.Context, BuildInput) (Runtime, error)
}

type Registry struct {
	mu        sync.RWMutex
	factories map[contract.RuntimeType]Factory
}

func NewRegistry(factories ...Factory) (*Registry, error) {
	r := &Registry{factories: make(map[contract.RuntimeType]Factory)}
	for _, factory := range factories {
		if err := r.Register(factory); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *Registry) Register(factory Factory) error {
	if factory == nil || factory.Type() == "" {
		return errors.New("runtime: factory and type are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.factories[factory.Type()]; exists {
		return fmt.Errorf("runtime: duplicate factory %q", factory.Type())
	}
	r.factories[factory.Type()] = factory
	return nil
}

func (r *Registry) Factory(runtimeType contract.RuntimeType) (Factory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	factory, ok := r.factories[runtimeType]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrRuntimeNotRegistered, runtimeType)
	}
	return factory, nil
}

type SliceStream struct {
	Events []contract.RunEvent
	index  int
}

func (s *SliceStream) Next(context.Context) (contract.RunEvent, error) {
	if s.index >= len(s.Events) {
		return contract.RunEvent{}, io.EOF
	}
	event := s.Events[s.index]
	s.index++
	return event, nil
}

func (*SliceStream) Close() error { return nil }
