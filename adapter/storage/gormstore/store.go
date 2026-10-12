// Package gormstore provides portable GORM-backed storage.
package gormstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/izhw/ai-agent/contract"
	"github.com/izhw/ai-agent/internal/idgen"
	storagespi "github.com/izhw/ai-agent/spi/storage"
)

type Dialect string

const (
	DialectMySQL      Dialect = "mysql"
	DialectPostgres   Dialect = "postgres"
	DialectDameng     Dialect = "dameng"
	DialectCompatible Dialect = "compatible"
)

type Option func(*Store)

func WithDialect(dialect Dialect) Option { return func(s *Store) { s.dialect = dialect } }

type Store struct {
	db      *gorm.DB
	dialect Dialect
}

func New(db *gorm.DB, options ...Option) (*Store, error) {
	if db == nil {
		return nil, errors.New("gormstore: db is required")
	}
	s := &Store{db: db, dialect: Dialect(db.Name())}
	for _, option := range options {
		option(s)
	}
	return s, nil
}

func (s *Store) DB() *gorm.DB { return s.db }

func (s *Store) Migrate(ctx context.Context) error {
	return s.db.WithContext(ctx).AutoMigrate(
		&agentVersionModel{}, &activeAgentModel{}, &conversationModel{}, &runModel{},
		&runEventModel{}, &checkpointModel{}, &memoryModel{}, &auditModel{},
	)
}

func (s *Store) Agents() storagespi.AgentRepository { return agentRepository{s.db} }
func (s *Store) Conversations() storagespi.ConversationRepository {
	return conversationRepository{s.db}
}
func (s *Store) Runs() storagespi.RunRepository          { return runRepository{s.db} }
func (s *Store) Events() storagespi.RunEventStore        { return eventRepository{s.db} }
func (s *Store) Checkpoints() storagespi.CheckpointStore { return checkpointRepository{s.db} }
func (s *Store) Memories() storagespi.MemoryStore        { return memoryRepository{s.db} }
func (s *Store) Audits() storagespi.AuditRepository      { return auditRepository{s.db} }

func (s *Store) InTx(ctx context.Context, fn func(storagespi.Storage) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&Store{db: tx, dialect: s.dialect})
	})
}

func (s *Store) Capabilities() storagespi.Capabilities {
	caps := storagespi.Capabilities{Transactions: true, BinaryCheckpoint: true}
	switch s.dialect {
	case DialectMySQL, DialectPostgres, DialectDameng:
		caps.RowLocking = true
	default:
		caps.RowLocking = false
	}
	if s.dialect == DialectMySQL || s.dialect == DialectPostgres {
		caps.NativeJSON = true
	}
	return caps
}

type agentVersionModel struct {
	ID        string    `gorm:"primaryKey;size:64"`
	TenantID  string    `gorm:"uniqueIndex:uk_agent_version,priority:1;index;size:128;not null"`
	ProductID string    `gorm:"uniqueIndex:uk_agent_version,priority:2;index;size:128;not null"`
	AgentKey  string    `gorm:"uniqueIndex:uk_agent_version,priority:3;size:128;not null"`
	Version   int64     `gorm:"uniqueIndex:uk_agent_version,priority:4;not null"`
	Spec      []byte    `gorm:"not null"`
	CreatedAt time.Time `gorm:"not null"`
}

func (agentVersionModel) TableName() string { return "agent_versions" }

type activeAgentModel struct {
	TenantID  string    `gorm:"primaryKey;size:128"`
	ProductID string    `gorm:"primaryKey;size:128"`
	AgentKey  string    `gorm:"primaryKey;size:128"`
	Version   int64     `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

func (activeAgentModel) TableName() string { return "active_agents" }

type conversationModel struct {
	ID        string `gorm:"primaryKey;size:64"`
	TenantID  string `gorm:"index:idx_conversation_scope,priority:1;size:128;not null"`
	ProductID string `gorm:"index:idx_conversation_scope,priority:2;size:128;not null"`
	UserID    string `gorm:"index:idx_conversation_scope,priority:3;size:128;not null"`
	AgentKey  string `gorm:"size:128;not null"`
	Messages  []byte
	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

func (conversationModel) TableName() string { return "agent_conversations" }

type runModel struct {
	ID             string `gorm:"primaryKey;size:64"`
	TenantID       string `gorm:"index:idx_run_scope,priority:1;size:128;not null"`
	ProductID      string `gorm:"index:idx_run_scope,priority:2;size:128;not null"`
	UserID         string `gorm:"size:128;not null"`
	AgentKey       string `gorm:"size:128;not null"`
	AgentVersion   int64  `gorm:"not null"`
	ConversationID string `gorm:"index;size:64;not null"`
	CheckpointID   string `gorm:"size:128"`
	Status         string `gorm:"index;size:32;not null"`
	ErrorText      string
	Metadata       []byte
	CreatedAt      time.Time `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"not null"`
}

func (runModel) TableName() string { return "agent_runs" }

type runEventModel struct {
	ID        string    `gorm:"primaryKey;size:64"`
	TenantID  string    `gorm:"uniqueIndex:uk_run_event_seq,priority:1;size:128;not null"`
	ProductID string    `gorm:"uniqueIndex:uk_run_event_seq,priority:2;size:128;not null"`
	RunID     string    `gorm:"uniqueIndex:uk_run_event_seq,priority:3;index;size:64;not null"`
	Sequence  int64     `gorm:"uniqueIndex:uk_run_event_seq,priority:4;not null"`
	Payload   []byte    `gorm:"not null"`
	CreatedAt time.Time `gorm:"not null"`
}

func (runEventModel) TableName() string { return "agent_run_events" }

type checkpointModel struct {
	ID               string    `gorm:"primaryKey;size:128"`
	Payload          []byte    `gorm:"not null"`
	Framework        string    `gorm:"size:32;not null"`
	FrameworkVersion string    `gorm:"size:32;not null"`
	CodecVersion     string    `gorm:"size:32;not null"`
	ExpiresAt        time.Time `gorm:"index"`
	UpdatedAt        time.Time `gorm:"not null"`
}

func (checkpointModel) TableName() string { return "agent_checkpoints" }

type memoryModel struct {
	ID          string `gorm:"primaryKey;size:64"`
	TenantID    string `gorm:"index:idx_memory_scope,priority:1;size:128;not null"`
	ProductID   string `gorm:"index:idx_memory_scope,priority:2;size:128;not null"`
	UserID      string `gorm:"index:idx_memory_scope,priority:3;size:128;not null"`
	AgentKey    string `gorm:"index:idx_memory_scope,priority:4;size:128"`
	Category    string `gorm:"size:64"`
	Content     string
	Confidence  float64
	Embedding   []byte
	SourceRunID string `gorm:"size:64"`
	Metadata    []byte
	ExpiresAt   *time.Time `gorm:"index"`
	CreatedAt   time.Time  `gorm:"not null"`
	UpdatedAt   time.Time  `gorm:"not null"`
}

func (memoryModel) TableName() string { return "agent_memories" }

type auditModel struct {
	ID        string `gorm:"primaryKey;size:64"`
	TenantID  string `gorm:"index:idx_audit_scope,priority:1;size:128;not null"`
	ProductID string `gorm:"index:idx_audit_scope,priority:2;size:128;not null"`
	Category  string `gorm:"index;size:64;not null"`
	SubjectID string `gorm:"index;size:128;not null"`
	Payload   []byte
	CreatedAt time.Time `gorm:"index;not null"`
}

func (auditModel) TableName() string { return "agent_audits" }

type agentRepository struct{ db *gorm.DB }

func (r agentRepository) SaveVersion(ctx context.Context, spec contract.AgentSpec) error {
	payload, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	row := agentVersionModel{
		ID:        spec.ID,
		TenantID:  spec.TenantID,
		ProductID: spec.ProductID,
		AgentKey:  spec.Key,
		Version:   spec.Version,
		Spec:      payload,
		CreatedAt: spec.CreatedAt,
	}
	return mapError(r.db.WithContext(ctx).Create(&row).Error)
}

func (r agentRepository) GetVersion(
	ctx context.Context,
	tenantID, productID, key string,
	version int64,
) (contract.AgentSpec, error) {
	var row agentVersionModel
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND product_id = ? AND agent_key = ? AND version = ?", tenantID, productID, key, version).
		Take(&row).
		Error
	if err != nil {
		return contract.AgentSpec{}, mapError(err)
	}
	var spec contract.AgentSpec
	if err = json.Unmarshal(row.Spec, &spec); err != nil {
		return contract.AgentSpec{}, fmt.Errorf("gormstore: decode agent spec: %w", err)
	}
	return spec, nil
}

func (r agentRepository) GetActive(
	ctx context.Context,
	tenantID, productID, key string,
) (contract.AgentSpec, error) {
	var active activeAgentModel
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND product_id = ? AND agent_key = ?", tenantID, productID, key).
		Take(&active).
		Error
	if err != nil {
		return contract.AgentSpec{}, mapError(err)
	}
	return r.GetVersion(ctx, tenantID, productID, key, active.Version)
}

func (r agentRepository) Activate(
	ctx context.Context,
	tenantID, productID, key string,
	version int64,
) error {
	if _, err := r.GetVersion(ctx, tenantID, productID, key, version); err != nil {
		return err
	}
	row := activeAgentModel{
		TenantID:  tenantID,
		ProductID: productID,
		AgentKey:  key,
		Version:   version,
		UpdatedAt: time.Now().UTC(),
	}
	return portableSave(
		r.db.WithContext(ctx),
		&row,
		"tenant_id = ? AND product_id = ? AND agent_key = ?",
		tenantID,
		productID,
		key,
	)
}

type conversationRepository struct{ db *gorm.DB }

func (r conversationRepository) Create(ctx context.Context, value contract.Conversation) error {
	messages, err := json.Marshal(value.Messages)
	if err != nil {
		return err
	}
	row := conversationModel{
		ID:        value.ID,
		TenantID:  value.TenantID,
		ProductID: value.ProductID,
		UserID:    value.UserID,
		AgentKey:  value.AgentKey,
		Messages:  messages,
		CreatedAt: value.CreatedAt,
		UpdatedAt: value.UpdatedAt,
	}
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r conversationRepository) Get(
	ctx context.Context,
	tenantID, productID, id string,
) (contract.Conversation, error) {
	var row conversationModel
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND product_id = ? AND id = ?", tenantID, productID, id).
		Take(&row).
		Error
	if err != nil {
		return contract.Conversation{}, mapError(err)
	}
	var messages []contract.Message
	if len(row.Messages) > 0 {
		if err = json.Unmarshal(row.Messages, &messages); err != nil {
			return contract.Conversation{}, err
		}
	}
	return contract.Conversation{
		ID:        row.ID,
		TenantID:  row.TenantID,
		ProductID: row.ProductID,
		UserID:    row.UserID,
		AgentKey:  row.AgentKey,
		Messages:  messages,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}, nil
}

func (r conversationRepository) AppendMessages(
	ctx context.Context,
	tenantID, productID, id string,
	messages ...contract.Message,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row conversationModel
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("tenant_id = ? AND product_id = ? AND id = ?", tenantID, productID, id).
			Take(&row).
			Error
		if err != nil {
			return mapError(err)
		}
		current := make([]contract.Message, 0, len(messages))
		if len(row.Messages) > 0 {
			if err = json.Unmarshal(row.Messages, &current); err != nil {
				return err
			}
		}
		current = append(current, messages...)
		payload, err := json.Marshal(current)
		if err != nil {
			return err
		}
		return tx.Model(&conversationModel{}).
			Where("tenant_id = ? AND product_id = ? AND id = ?", tenantID, productID, id).
			Updates(map[string]any{"messages": payload, "updated_at": time.Now().UTC()}).
			Error
	})
}

type runRepository struct{ db *gorm.DB }

func (r runRepository) Create(ctx context.Context, value contract.Run) error {
	metadata, err := json.Marshal(value.Metadata)
	if err != nil {
		return err
	}
	row := runModel{
		ID:             value.ID,
		TenantID:       value.TenantID,
		ProductID:      value.ProductID,
		UserID:         value.UserID,
		AgentKey:       value.AgentKey,
		AgentVersion:   value.AgentVersion,
		ConversationID: value.ConversationID,
		CheckpointID:   value.CheckpointID,
		Status:         string(value.Status),
		ErrorText:      value.Error,
		Metadata:       metadata,
		CreatedAt:      value.CreatedAt,
		UpdatedAt:      value.UpdatedAt,
	}
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r runRepository) Get(
	ctx context.Context,
	tenantID, productID, id string,
) (contract.Run, error) {
	var row runModel
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND product_id = ? AND id = ?", tenantID, productID, id).
		Take(&row).
		Error
	if err != nil {
		return contract.Run{}, mapError(err)
	}
	var metadata map[string]string
	if len(row.Metadata) > 0 {
		if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
			return contract.Run{}, fmt.Errorf("decode run metadata: %w", err)
		}
	}
	return contract.Run{
		ID:             row.ID,
		TenantID:       row.TenantID,
		ProductID:      row.ProductID,
		UserID:         row.UserID,
		AgentKey:       row.AgentKey,
		AgentVersion:   row.AgentVersion,
		ConversationID: row.ConversationID,
		CheckpointID:   row.CheckpointID,
		Status:         contract.RunStatus(row.Status),
		Error:          row.ErrorText,
		Metadata:       metadata,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}, nil
}

func (r runRepository) UpdateStatus(
	ctx context.Context,
	tenantID, productID, id string,
	status contract.RunStatus,
	errText string,
) error {
	result := r.db.WithContext(ctx).
		Model(&runModel{}).
		Where("tenant_id = ? AND product_id = ? AND id = ?", tenantID, productID, id).
		Updates(map[string]any{"status": string(status), "error_text": errText, "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return storagespi.ErrNotFound
	}
	return nil
}

type eventRepository struct{ db *gorm.DB }

func (r runRepository) Transition(
	ctx context.Context,
	t, p, id string,
	expected, next contract.RunStatus,
) error {
	result := r.db.WithContext(ctx).
		Model(&runModel{}).
		Where("tenant_id = ? AND product_id = ? AND id = ? AND status = ?", t, p, id, string(expected)).
		Updates(map[string]any{"status": string(next), "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return storagespi.ErrConflict
	}
	return nil
}

func (r eventRepository) Append(ctx context.Context, value contract.RunEvent) error {
	var run runModel
	if err := r.db.WithContext(ctx).
		Select("tenant_id", "product_id").
		Where("id = ?", value.RunID).
		Take(&run).
		Error; err != nil {
		return mapError(err)
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	row := runEventModel{
		ID:        value.ID,
		TenantID:  run.TenantID,
		ProductID: run.ProductID,
		RunID:     value.RunID,
		Sequence:  value.Sequence,
		Payload:   payload,
		CreatedAt: value.CreatedAt,
	}
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r eventRepository) ListAfter(
	ctx context.Context,
	tenantID, productID, runID string,
	sequence int64,
	limit int,
) ([]contract.RunEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	var rows []runEventModel
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND product_id = ? AND run_id = ? AND sequence > ?", tenantID, productID, runID, sequence).
		Order("sequence ASC").
		Limit(limit).
		Find(&rows).
		Error
	if err != nil {
		return nil, err
	}
	result := make([]contract.RunEvent, 0, len(rows))
	for _, row := range rows {
		var event contract.RunEvent
		if err = json.Unmarshal(row.Payload, &event); err != nil {
			return nil, err
		}
		result = append(result, event)
	}
	return result, nil
}

func (r eventRepository) LastSequence(
	ctx context.Context,
	tenantID, productID, runID string,
) (int64, error) {
	var value struct{ Sequence int64 }
	err := r.db.WithContext(ctx).Model(&runEventModel{}).
		Select("COALESCE(MAX(sequence), 0) AS sequence").
		Where("tenant_id = ? AND product_id = ? AND run_id = ?", tenantID, productID, runID).
		Scan(&value).Error
	return value.Sequence, err
}

type checkpointRepository struct{ db *gorm.DB }

func (r checkpointRepository) Get(ctx context.Context, id string) (contract.Checkpoint, error) {
	var row checkpointModel
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&row).Error
	if err != nil {
		return contract.Checkpoint{}, mapError(err)
	}
	return contract.Checkpoint{
		ID:               row.ID,
		Payload:          row.Payload,
		Framework:        row.Framework,
		FrameworkVersion: row.FrameworkVersion,
		CodecVersion:     row.CodecVersion,
		ExpiresAt:        row.ExpiresAt,
		UpdatedAt:        row.UpdatedAt,
	}, nil
}

func (r checkpointRepository) Set(ctx context.Context, v contract.Checkpoint) error {
	row := checkpointModel{
		ID:               v.ID,
		Payload:          v.Payload,
		Framework:        v.Framework,
		FrameworkVersion: v.FrameworkVersion,
		CodecVersion:     v.CodecVersion,
		ExpiresAt:        v.ExpiresAt,
		UpdatedAt:        v.UpdatedAt,
	}
	return portableSave(r.db.WithContext(ctx), &row, "id = ?", row.ID)
}

func (r checkpointRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&checkpointModel{}, "id = ?", id).Error
}

type memoryRepository struct{ db *gorm.DB }

func (r memoryRepository) Save(ctx context.Context, v contract.Memory) error {
	embedding, err := json.Marshal(v.Embedding)
	if err != nil {
		return err
	}
	metadata, err := json.Marshal(v.Metadata)
	if err != nil {
		return err
	}
	row := memoryModel{
		ID:          v.ID,
		TenantID:    v.TenantID,
		ProductID:   v.ProductID,
		UserID:      v.UserID,
		AgentKey:    v.AgentKey,
		Category:    v.Category,
		Content:     v.Content,
		Confidence:  v.Confidence,
		Embedding:   embedding,
		SourceRunID: v.SourceRunID,
		Metadata:    metadata,
		ExpiresAt:   v.ExpiresAt,
		CreatedAt:   v.CreatedAt,
		UpdatedAt:   v.UpdatedAt,
	}
	return portableSave(
		r.db.WithContext(ctx),
		&row,
		"id = ? AND tenant_id = ? AND product_id = ? AND user_id = ?",
		row.ID,
		row.TenantID,
		row.ProductID,
		row.UserID,
	)
}

func (r memoryRepository) Delete(ctx context.Context, t, p, u, id string) error {
	result := r.db.WithContext(ctx).
		Where("tenant_id = ? AND product_id = ? AND user_id = ? AND id = ?", t, p, u, id).
		Delete(&memoryModel{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return storagespi.ErrNotFound
	}
	return nil
}

func (r memoryRepository) ListCandidates(
	ctx context.Context,
	t, p, u, agentKey string,
	limit int,
) ([]contract.Memory, error) {
	if limit <= 0 {
		limit = 500
	}
	var rows []memoryModel
	now := time.Now().UTC()
	const scopeQuery = "tenant_id = ? AND product_id = ? AND user_id = ? " +
		"AND (agent_key = ? OR agent_key = '') " +
		"AND (expires_at IS NULL OR expires_at > ?)"
	query := r.db.WithContext(ctx).
		Where(scopeQuery, t, p, u, agentKey, now).
		Order("updated_at DESC").
		Limit(limit)
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]contract.Memory, 0, len(rows))
	for _, row := range rows {
		var embedding []float32
		var metadata map[string]string
		if len(row.Embedding) > 0 {
			if err := json.Unmarshal(row.Embedding, &embedding); err != nil {
				return nil, fmt.Errorf("decode memory embedding: %w", err)
			}
		}
		if len(row.Metadata) > 0 {
			if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
				return nil, fmt.Errorf("decode memory metadata: %w", err)
			}
		}
		result = append(
			result,
			contract.Memory{
				ID:          row.ID,
				TenantID:    row.TenantID,
				ProductID:   row.ProductID,
				UserID:      row.UserID,
				AgentKey:    row.AgentKey,
				Category:    row.Category,
				Content:     row.Content,
				Confidence:  row.Confidence,
				Embedding:   embedding,
				SourceRunID: row.SourceRunID,
				Metadata:    metadata,
				ExpiresAt:   row.ExpiresAt,
				CreatedAt:   row.CreatedAt,
				UpdatedAt:   row.UpdatedAt,
			},
		)
	}
	return result, nil
}

type auditRepository struct{ db *gorm.DB }

func (r auditRepository) Append(
	ctx context.Context,
	t, p, category, subjectID string,
	data []byte,
) error {
	row := auditModel{
		ID:        idgen.New(),
		TenantID:  t,
		ProductID: p,
		Category:  category,
		SubjectID: subjectID,
		Payload:   data,
		CreatedAt: time.Now().UTC(),
	}
	return r.db.WithContext(ctx).Create(&row).Error
}

func mapError(err error) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return storagespi.ErrConflict
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return storagespi.ErrNotFound
	}
	return err
}

// portableSave uses only SELECT/UPDATE/INSERT so an injected dialect does not
// need an ON CONFLICT implementation. Concurrent first inserts may conflict;
// callers must retry the whole transaction, never an external tool side effect.
func portableSave(db *gorm.DB, row any, query string, args ...any) error {
	var count int64
	if err := db.Model(row).Where(query, args...).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return mapError(db.Create(row).Error)
	}
	return db.Model(row).Where(query, args...).Select("*").Updates(row).Error
}
