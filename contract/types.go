// Package contract contains the stable, framework-independent API contract.
package contract

import (
	"encoding/json"
	"time"
)

type RuntimeType string

const (
	RuntimeChatModel  RuntimeType = "chat_model"
	RuntimeDeepAgent  RuntimeType = "deep_agent"
	RuntimeSupervisor RuntimeType = "supervisor"
	RuntimeWorkflow   RuntimeType = "workflow"
)

type MemoryMode string

const (
	MemoryDisabled  MemoryMode = "disabled"
	MemoryReadOnly  MemoryMode = "read_only"
	MemoryWriteOnly MemoryMode = "write_only"
	MemoryReadWrite MemoryMode = "read_write"
)

func (m MemoryMode) CanRead() bool  { return m == MemoryReadOnly || m == MemoryReadWrite }
func (m MemoryMode) CanWrite() bool { return m == MemoryWriteOnly || m == MemoryReadWrite }

type AgentSpec struct {
	ID          string            `json:"id"`
	TenantID    string            `json:"tenant_id"`
	ProductID   string            `json:"product_id"`
	Key         string            `json:"key"`
	Version     int64             `json:"version"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Instruction string            `json:"instruction,omitempty"`
	ModelRef    string            `json:"model_ref"`
	ToolRefs    []string          `json:"tool_refs,omitempty"`
	Runtime     RuntimeSpec       `json:"runtime"`
	Memory      MemoryPolicy      `json:"memory"`
	Limits      RunLimits         `json:"limits"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}

type RuntimeSpec struct {
	Type      RuntimeType    `json:"type"`
	PresetRef string         `json:"preset_ref,omitempty"`
	Settings  map[string]any `json:"settings,omitempty"`
}

type MemoryPolicy struct {
	Mode            MemoryMode `json:"mode"`
	Scope           string     `json:"scope,omitempty"`
	MaxItems        int        `json:"max_items,omitempty"`
	MaxCandidates   int        `json:"max_candidates,omitempty"`
	MinConfidence   float64    `json:"min_confidence,omitempty"`
	TTL             string     `json:"ttl,omitempty"`
	RequireExplicit bool       `json:"require_explicit"`
	WindowMessages  int        `json:"window_messages,omitempty"`
}

type RunLimits struct {
	MaxSteps      int           `json:"max_steps,omitempty"`
	Timeout       time.Duration `json:"timeout,omitempty"`
	MaxInputBytes int           `json:"max_input_bytes,omitempty"`
}

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Message struct {
	ID         string            `json:"id,omitempty"`
	Role       Role              `json:"role"`
	Content    string            `json:"content"`
	ToolName   string            `json:"tool_name,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	CreatedAt  time.Time         `json:"created_at,omitempty"`
}

type RunRequest struct {
	TenantID       string            `json:"tenant_id"`
	ProductID      string            `json:"product_id"`
	UserID         string            `json:"user_id"`
	AgentKey       string            `json:"agent_key"`
	ConversationID string            `json:"conversation_id,omitempty"`
	Message        Message           `json:"message"`
	Variables      map[string]any    `json:"variables,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

type ResumeRequest struct {
	TenantID  string         `json:"tenant_id"`
	ProductID string         `json:"product_id"`
	UserID    string         `json:"user_id"`
	RunID     string         `json:"run_id"`
	Targets   map[string]any `json:"targets,omitempty"`
}

type RunStatus string

const (
	RunPending         RunStatus = "pending"
	RunRunning         RunStatus = "running"
	RunWaitingApproval RunStatus = "waiting_approval"
	RunCompleted       RunStatus = "completed"
	RunFailed          RunStatus = "failed"
	RunCancelled       RunStatus = "cancelled"
)

type Run struct {
	ID             string            `json:"id"`
	TenantID       string            `json:"tenant_id"`
	ProductID      string            `json:"product_id"`
	UserID         string            `json:"user_id"`
	AgentKey       string            `json:"agent_key"`
	AgentVersion   int64             `json:"agent_version"`
	ConversationID string            `json:"conversation_id"`
	CheckpointID   string            `json:"checkpoint_id,omitempty"`
	Status         RunStatus         `json:"status"`
	Error          string            `json:"error,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

type Conversation struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	ProductID string    `json:"product_id"`
	UserID    string    `json:"user_id"`
	AgentKey  string    `json:"agent_key"`
	Messages  []Message `json:"messages,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type EventType string

const (
	EventRunStarted       EventType = "run.started"
	EventMessageDelta     EventType = "message.delta"
	EventMessageCompleted EventType = "message.completed"
	EventToolStarted      EventType = "tool.started"
	EventToolCompleted    EventType = "tool.completed"
	EventApprovalRequired EventType = "approval.required"
	EventRunCompleted     EventType = "run.completed"
	EventRunFailed        EventType = "run.failed"
)

type RunEvent struct {
	ID        string          `json:"id"`
	RunID     string          `json:"run_id"`
	Sequence  int64           `json:"sequence"`
	Type      EventType       `json:"type"`
	AgentName string          `json:"agent_name,omitempty"`
	Role      Role            `json:"role,omitempty"`
	Content   string          `json:"content,omitempty"`
	ToolName  string          `json:"tool_name,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	Usage     *Usage          `json:"usage,omitempty"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type ApprovalPoint struct {
	ID   string `json:"id"`
	Info string `json:"info"`
}

type Memory struct {
	ID          string            `json:"id"`
	TenantID    string            `json:"tenant_id"`
	ProductID   string            `json:"product_id"`
	UserID      string            `json:"user_id"`
	AgentKey    string            `json:"agent_key"`
	Category    string            `json:"category,omitempty"`
	Content     string            `json:"content"`
	Confidence  float64           `json:"confidence"`
	Embedding   []float32         `json:"embedding,omitempty"`
	SourceRunID string            `json:"source_run_id,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type Checkpoint struct {
	ID               string    `json:"id"`
	Payload          []byte    `json:"-"`
	Framework        string    `json:"framework"`
	FrameworkVersion string    `json:"framework_version"`
	CodecVersion     string    `json:"codec_version"`
	ExpiresAt        time.Time `json:"expires_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
