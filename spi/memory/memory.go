// Package memory defines long-term memory extraction and retrieval ports.
package memory

import (
	"context"

	"github.com/izhw/ai-agent/contract"
)

type Query struct {
	TenantID      string
	ProductID     string
	UserID        string
	AgentKey      string
	Text          string
	Limit         int
	MaxCandidates int
}

type ExtractInput struct {
	Run      contract.Run
	Messages []contract.Message
	Policy   contract.MemoryPolicy
}

type Retriever interface {
	Retrieve(context.Context, Query) ([]contract.Memory, error)
}

type Extractor interface {
	Extract(context.Context, ExtractInput) ([]contract.Memory, error)
}

type Service interface {
	Recall(context.Context, Query) ([]contract.Memory, error)
	Remember(context.Context, ExtractInput) error
}
