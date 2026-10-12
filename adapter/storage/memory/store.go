// Package memory provides a concurrency-safe development storage implementation.
package memory

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/izhw/ai-agent/contract"
	storagespi "github.com/izhw/ai-agent/spi/storage"
)

type Store struct {
	mu            sync.RWMutex
	agents        map[string]contract.AgentSpec
	activeAgents  map[string]int64
	conversations map[string]contract.Conversation
	runs          map[string]contract.Run
	events        map[string][]contract.RunEvent
	checkpoints   map[string]contract.Checkpoint
	memories      map[string]contract.Memory
	audits        [][]byte
}

func New() *Store {
	return &Store{
		agents:        make(map[string]contract.AgentSpec),
		activeAgents:  make(map[string]int64),
		conversations: make(map[string]contract.Conversation),
		runs:          make(map[string]contract.Run),
		events:        make(map[string][]contract.RunEvent),
		checkpoints:   make(map[string]contract.Checkpoint),
		memories:      make(map[string]contract.Memory),
	}
}

func scope(parts ...string) string {
	return string(mustMarshal(parts))
}

func (s *Store) Agents() storagespi.AgentRepository { return s }

func (s *Store) Conversations() storagespi.ConversationRepository { return s.conversationRepository() }
func (s *Store) Runs() storagespi.RunRepository                   { return s.runRepository() }
func (s *Store) Events() storagespi.RunEventStore                 { return s }

func (s *Store) Checkpoints() storagespi.CheckpointStore { return s.checkpointRepository() }
func (s *Store) Memories() storagespi.MemoryStore        { return s }
func (s *Store) Audits() storagespi.AuditRepository      { return s.auditRepository() }
func (*Store) Capabilities() storagespi.Capabilities {
	return storagespi.Capabilities{BinaryCheckpoint: true, Transactions: true}
}

func (s *Store) InTx(ctx context.Context, fn func(storagespi.Storage) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx := New()
	tx.agents, tx.activeAgents = clone(s.agents), clone(s.activeAgents)
	tx.conversations, tx.runs, tx.events = clone(s.conversations), clone(s.runs), clone(s.events)
	tx.memories, tx.audits = clone(s.memories), clone(s.audits)
	for k, v := range s.checkpoints {
		v.Payload = append([]byte(nil), v.Payload...)
		tx.checkpoints[k] = v
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.agents, s.activeAgents = tx.agents, tx.activeAgents
	s.conversations, s.runs, s.events = tx.conversations, tx.runs, tx.events
	s.checkpoints, s.memories, s.audits = tx.checkpoints, tx.memories, tx.audits
	return nil
}

func (s *Store) SaveVersion(_ context.Context, spec contract.AgentSpec) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := scope(spec.TenantID, spec.ProductID, spec.Key, strconv.FormatInt(spec.Version, 10))
	if _, exists := s.agents[k]; exists {
		return storagespi.ErrConflict
	}
	s.agents[k] = clone(spec)
	return nil
}

func (s *Store) GetVersion(
	_ context.Context,
	tenantID, productID, key string,
	version int64,
) (contract.AgentSpec, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	spec, ok := s.agents[scope(tenantID, productID, key, strconv.FormatInt(version, 10))]
	if !ok {
		return contract.AgentSpec{}, storagespi.ErrNotFound
	}
	return clone(spec), nil
}

func (s *Store) GetActive(
	ctx context.Context,
	tenantID, productID, key string,
) (contract.AgentSpec, error) {
	s.mu.RLock()
	version, ok := s.activeAgents[scope(tenantID, productID, key)]
	s.mu.RUnlock()
	if !ok {
		return contract.AgentSpec{}, storagespi.ErrNotFound
	}
	return s.GetVersion(ctx, tenantID, productID, key, version)
}

func (s *Store) Activate(_ context.Context, tenantID, productID, key string, version int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.agents[scope(tenantID, productID, key, strconv.FormatInt(version, 10))]; !ok {
		return storagespi.ErrNotFound
	}
	s.activeAgents[scope(tenantID, productID, key)] = version
	return nil
}

func (s *Store) Create(_ context.Context, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch v := value.(type) {
	case contract.Conversation:
		if _, exists := s.conversations[scope(v.TenantID, v.ProductID, v.ID)]; exists {
			return storagespi.ErrConflict
		}
		s.conversations[scope(v.TenantID, v.ProductID, v.ID)] = clone(v)
	case contract.Run:
		s.runs[scope(v.TenantID, v.ProductID, v.ID)] = clone(v)
	default:
		return errors.New("memory: unsupported value type")
	}
	return nil
}

func (s *Store) Get(
	_ context.Context,
	tenantID, productID, id string,
) (contract.Conversation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.conversations[scope(tenantID, productID, id)]
	if !ok {
		return contract.Conversation{}, storagespi.ErrNotFound
	}
	return clone(v), nil
}

func (s *Store) AppendMessages(
	_ context.Context,
	tenantID, productID, id string,
	messages ...contract.Message,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := scope(tenantID, productID, id)
	v, ok := s.conversations[k]
	if !ok {
		return storagespi.ErrNotFound
	}
	v.Messages = append(v.Messages, clone(messages)...)
	v.UpdatedAt = time.Now().UTC()
	s.conversations[k] = v
	return nil
}

func (s *Store) CreateRun(_ context.Context, run contract.Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.runs {
		if existing.ID == run.ID {
			return storagespi.ErrConflict
		}
	}
	s.runs[scope(run.TenantID, run.ProductID, run.ID)] = clone(run)
	return nil
}

func (s *Store) Transition(
	_ context.Context,
	t, p, id string,
	expected, next contract.RunStatus,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := scope(t, p, id)
	run, ok := s.runs[k]
	if !ok {
		return storagespi.ErrNotFound
	}
	if run.Status != expected {
		return storagespi.ErrConflict
	}
	run.Status, run.UpdatedAt = next, time.Now().UTC()
	s.runs[k] = run
	return nil
}

func (s *Store) GetRun(_ context.Context, tenantID, productID, id string) (contract.Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.runs[scope(tenantID, productID, id)]
	if !ok {
		return contract.Run{}, storagespi.ErrNotFound
	}
	return clone(v), nil
}

func (s *Store) UpdateStatus(
	_ context.Context,
	tenantID, productID, id string,
	status contract.RunStatus,
	errText string,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := scope(tenantID, productID, id)
	v, ok := s.runs[k]
	if !ok {
		return storagespi.ErrNotFound
	}
	v.Status, v.Error, v.UpdatedAt = status, errText, time.Now().UTC()
	s.runs[k] = v
	return nil
}

func (s *Store) Append(_ context.Context, event contract.RunEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := scope(event.RunID)
	for _, old := range s.events[k] {
		if old.Sequence == event.Sequence {
			return storagespi.ErrConflict
		}
	}
	s.events[k] = append(s.events[k], clone(event))
	return nil
}

func (s *Store) ListAfter(
	_ context.Context,
	tenantID, productID, runID string,
	sequence int64,
	limit int,
) ([]contract.RunEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.runs[scope(tenantID, productID, runID)]; !ok {
		return nil, storagespi.ErrNotFound
	}
	if limit <= 0 {
		limit = 100
	}
	result := make([]contract.RunEvent, 0, limit)
	for _, event := range s.events[scope(runID)] {
		if event.Sequence > sequence {
			result = append(result, clone(event))
		}
		if len(result) == limit {
			break
		}
	}
	return result, nil
}

func (s *Store) LastSequence(_ context.Context, tenantID, productID, runID string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.runs[scope(tenantID, productID, runID)]; !ok {
		return 0, storagespi.ErrNotFound
	}
	events := s.events[scope(runID)]
	if len(events) == 0 {
		return 0, nil
	}
	return events[len(events)-1].Sequence, nil
}

func (s *Store) GetCheckpoint(_ context.Context, id string) (contract.Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.checkpoints[id]
	if !ok {
		return contract.Checkpoint{}, storagespi.ErrNotFound
	}
	v.Payload = append([]byte(nil), v.Payload...)
	return v, nil
}

func (s *Store) SetCheckpoint(_ context.Context, value contract.Checkpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	value.Payload = append([]byte(nil), value.Payload...)
	s.checkpoints[value.ID] = value
	return nil
}

func (s *Store) DeleteCheckpoint(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.checkpoints, id)
	return nil
}

func (s *Store) Save(_ context.Context, value contract.Memory) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.memories[value.ID] = clone(value)
	return nil
}

func (s *Store) Delete(_ context.Context, tenantID, productID, userID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.memories[id]
	if !ok || v.TenantID != tenantID || v.ProductID != productID || v.UserID != userID {
		return storagespi.ErrNotFound
	}
	delete(s.memories, id)
	return nil
}

func (s *Store) ListCandidates(
	_ context.Context,
	tenantID, productID, userID, agentKey string,
	limit int,
) ([]contract.Memory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 500
	}
	now := time.Now().UTC()
	result := make([]contract.Memory, 0)
	for _, value := range s.memories {
		if value.TenantID != tenantID || value.ProductID != productID || value.UserID != userID {
			continue
		}
		if value.AgentKey != "" && value.AgentKey != agentKey {
			continue
		}
		if value.ExpiresAt != nil && value.ExpiresAt.Before(now) {
			continue
		}
		result = append(result, clone(value))
	}
	slices.SortFunc(result, func(left, right contract.Memory) int {
		return right.UpdatedAt.Compare(left.UpdatedAt)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *Store) AppendAudit(
	_ context.Context,
	tenantID, productID, category, subjectID string,
	data []byte,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record := mustMarshal([]any{tenantID, productID, category, subjectID, json.RawMessage(data)})
	s.audits = append(s.audits, record)
	return nil
}

func clone[T any](value T) T {
	b := mustMarshal(value)
	var result T
	if err := json.Unmarshal(b, &result); err != nil {
		panic(err)
	}
	return result
}

func mustMarshal(value any) []byte {
	b, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return b
}

// Compile-time adapters avoid overloaded method names in Go.
type (
	conversationRepo struct{ *Store }
	runRepo          struct{ *Store }
	checkpointRepo   struct{ *Store }
	auditRepo        struct{ *Store }
)

func (s *Store) conversationRepository() storagespi.ConversationRepository {
	return conversationRepo{s}
}
func (s *Store) runRepository() storagespi.RunRepository          { return runRepo{s} }
func (s *Store) checkpointRepository() storagespi.CheckpointStore { return checkpointRepo{s} }
func (s *Store) auditRepository() storagespi.AuditRepository      { return auditRepo{s} }

func (r conversationRepo) Create(ctx context.Context, v contract.Conversation) error {
	return r.Store.Create(ctx, v)
}
func (r runRepo) Create(ctx context.Context, v contract.Run) error { return r.CreateRun(ctx, v) }
func (r runRepo) Get(ctx context.Context, t, p, id string) (contract.Run, error) {
	return r.GetRun(ctx, t, p, id)
}

func (r checkpointRepo) Get(ctx context.Context, id string) (contract.Checkpoint, error) {
	return r.GetCheckpoint(ctx, id)
}

func (r checkpointRepo) Set(ctx context.Context, v contract.Checkpoint) error {
	return r.SetCheckpoint(ctx, v)
}

func (r checkpointRepo) Delete(ctx context.Context, id string) error {
	return r.DeleteCheckpoint(ctx, id)
}

func (r auditRepo) Append(ctx context.Context, t, p, c, id string, data []byte) error {
	return r.AppendAudit(ctx, t, p, c, id, data)
}
