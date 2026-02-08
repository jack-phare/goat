package agent

import (
	"context"
	"time"

	"github.com/jg-phare/goat/pkg/llm"
	"github.com/jg-phare/goat/pkg/types"
)

// TokenBudget describes token utilization for context window management.
type TokenBudget struct {
	ContextLimit     int // model's total context window
	SystemPromptTkns int // estimated system prompt tokens
	MaxOutputTkns    int // reserved for output (default 16384)
	MessageTkns      int // current message history tokens
}

// IsOverflow returns true if current usage exceeds the context limit.
func (b TokenBudget) IsOverflow() bool {
	return b.MessageTkns+b.SystemPromptTkns+b.MaxOutputTkns > b.ContextLimit
}

// UtilizationPct returns the fraction of the context window currently used.
func (b TokenBudget) UtilizationPct() float64 {
	if b.ContextLimit <= 0 {
		return 1.0
	}
	used := b.SystemPromptTkns + b.MessageTkns + b.MaxOutputTkns
	return float64(used) / float64(b.ContextLimit)
}

// Available returns the number of tokens remaining for new content.
func (b TokenBudget) Available() int {
	avail := b.ContextLimit - b.SystemPromptTkns - b.MessageTkns - b.MaxOutputTkns
	if avail < 0 {
		return 0
	}
	return avail
}

// CompactRequest is the input to a Compact call.
type CompactRequest struct {
	Messages  []llm.ChatMessage
	Model     string
	Budget    TokenBudget
	Trigger   string // "auto" | "manual"
	SessionID string
	EmitCh    chan<- types.SDKMessage
}

// SystemPromptAssembler builds the system prompt for an LLM call.
type SystemPromptAssembler interface {
	Assemble(config *AgentConfig) string
}

// PermissionChecker gates tool execution.
type PermissionChecker interface {
	Check(ctx context.Context, toolName string, input map[string]any) (PermissionResult, error)
}

// PermissionResult is the outcome of a permission check.
type PermissionResult struct {
	Behavior           string             // "allow"|"deny"|"ask"
	UpdatedInput       map[string]any     // nil if unchanged
	UpdatedPermissions []types.PermissionUpdate // rule changes to persist
	Message            string             // deny reason
	Interrupt          bool               // stop the loop entirely
	ToolUseID          string             // for correlation
}

// HookRunner fires lifecycle hooks.
type HookRunner interface {
	Fire(ctx context.Context, event types.HookEvent, input any) ([]HookResult, error)
}

// HookResult is the outcome of a hook invocation.
type HookResult struct {
	Decision           string // "allow"|"deny"|""
	Message            string
	Continue           *bool
	SystemMessage      string
	SuppressOutput     *bool
	StopReason         string
	Reason             string
	HookSpecificOutput any // typed per-event output
}

// ContextCompactor handles context overflow.
type ContextCompactor interface {
	ShouldCompact(budget TokenBudget) bool
	Compact(ctx context.Context, req CompactRequest) ([]llm.ChatMessage, error)
}

// SessionStore manages session persistence.
type SessionStore interface {
	// Lifecycle
	Create(meta SessionMetadata) error
	Load(sessionID string) (*SessionState, error)
	LoadLatest(cwd string) (*SessionState, error)
	Delete(sessionID string) error
	List() ([]SessionMetadata, error)
	Fork(sourceID, newID string) (*SessionState, error)

	// Message persistence (async-safe)
	AppendMessage(sessionID string, entry MessageEntry) error
	AppendSDKMessage(sessionID string, msg types.SDKMessage) error
	LoadMessages(sessionID string) ([]MessageEntry, error)
	LoadMessagesUpTo(sessionID string, messageUUID string) ([]MessageEntry, error)

	// Metadata updates
	UpdateMetadata(sessionID string, fn func(*SessionMetadata)) error

	// Checkpoint management
	CreateCheckpoint(sessionID, userMsgUUID string, filePaths []string) error
	RewindFiles(sessionID, userMsgUUID string, dryRun bool) (*RewindFilesResult, error)

	// Lifecycle
	Close() error // flush async writer, close files
}

// SessionMetadata holds the identity and stats for a session.
type SessionMetadata struct {
	ID               string    `json:"id"`
	CWD              string    `json:"cwd"`
	Model            string    `json:"model"`
	ParentSessionID  string    `json:"parent_session_id,omitempty"`
	ForkPointUUID    string    `json:"fork_point_uuid,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	MessageCount     int       `json:"message_count"`
	TurnCount        int       `json:"turn_count"`
	TotalCostUSD     float64   `json:"total_cost_usd"`
	LeafTitle        string    `json:"leaf_title,omitempty"`
}

// SessionState is the loaded form of a session: metadata + messages.
type SessionState struct {
	Metadata SessionMetadata
	Messages []MessageEntry
}

// MessageEntry wraps a ChatMessage with a UUID and timestamp for JSONL persistence.
type MessageEntry struct {
	UUID      string          `json:"uuid"`
	Timestamp time.Time       `json:"timestamp"`
	Message   llm.ChatMessage `json:"message"`
}

// RewindFilesResult describes the outcome of a file rewind operation.
type RewindFilesResult struct {
	CanRewind    bool     `json:"can_rewind"`
	Error        string   `json:"error,omitempty"`
	FilesChanged []string `json:"files_changed,omitempty"`
	Insertions   int      `json:"insertions"`
	Deletions    int      `json:"deletions"`
}
