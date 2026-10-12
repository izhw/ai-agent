// Package observability defines optional run and event hooks.
package observability

import (
	"context"

	"github.com/izhw/ai-agent/contract"
)

type Observer interface {
	OnRunStart(context.Context, contract.Run, contract.AgentSpec)
	OnEvent(context.Context, contract.Run, contract.RunEvent)
	OnRunEnd(context.Context, contract.Run, contract.RunStatus, error)
}

type Nop struct{}

func (Nop) OnRunStart(context.Context, contract.Run, contract.AgentSpec)      {}
func (Nop) OnEvent(context.Context, contract.Run, contract.RunEvent)          {}
func (Nop) OnRunEnd(context.Context, contract.Run, contract.RunStatus, error) {}
