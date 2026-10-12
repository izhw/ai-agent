// Package storage defines persistence ports. Implementations must enforce tenant isolation.
package storage

import (
	"context"
	"errors"

	"github.com/izhw/ai-agent/contract"
)

var (
	ErrNotFound = errors.New("storage: not found")
	ErrConflict = errors.New("storage: conflict")
)

type Capabilities struct {
	Transactions       bool
	RowLocking         bool
	BinaryCheckpoint   bool
	NativeJSON         bool
	NativeVectorSearch bool
}

type AgentRepository interface {
	SaveVersion(context.Context, contract.AgentSpec) error
	GetVersion(
		ctx context.Context,
		tenantID, productID, key string,
		version int64,
	) (contract.AgentSpec, error)
	GetActive(ctx context.Context, tenantID, productID, key string) (contract.AgentSpec, error)
	Activate(ctx context.Context, tenantID, productID, key string, version int64) error
}

type ConversationRepository interface {
	Create(context.Context, contract.Conversation) error
	Get(ctx context.Context, tenantID, productID, id string) (contract.Conversation, error)
	AppendMessages(
		ctx context.Context,
		tenantID, productID, id string,
		messages ...contract.Message,
	) error
}

type RunRepository interface {
	Create(context.Context, contract.Run) error
	Get(ctx context.Context, tenantID, productID, id string) (contract.Run, error)
	UpdateStatus(
		ctx context.Context,
		tenantID, productID, id string,
		status contract.RunStatus,
		errText string,
	) error
	// Transition atomically claims a state change. Returns ErrConflict if expected differs.
	Transition(
		ctx context.Context,
		tenantID, productID, id string,
		expected, next contract.RunStatus,
	) error
}

type RunEventStore interface {
	Append(context.Context, contract.RunEvent) error
	ListAfter(
		ctx context.Context,
		tenantID, productID, runID string,
		sequence int64,
		limit int,
	) ([]contract.RunEvent, error)
	LastSequence(ctx context.Context, tenantID, productID, runID string) (int64, error)
}

type CheckpointStore interface {
	Get(context.Context, string) (contract.Checkpoint, error)
	Set(context.Context, contract.Checkpoint) error
	Delete(context.Context, string) error
}

type MemoryStore interface {
	Save(context.Context, contract.Memory) error
	Delete(ctx context.Context, tenantID, productID, userID, id string) error
	ListCandidates(
		ctx context.Context,
		tenantID, productID, userID, agentKey string,
		limit int,
	) ([]contract.Memory, error)
}

type AuditRepository interface {
	Append(ctx context.Context, tenantID, productID, category, subjectID string, data []byte) error
}

type Storage interface {
	Agents() AgentRepository
	Conversations() ConversationRepository
	Runs() RunRepository
	Events() RunEventStore
	Checkpoints() CheckpointStore
	Memories() MemoryStore
	Audits() AuditRepository
	InTx(ctx context.Context, fn func(Storage) error) error
	Capabilities() Capabilities
}
