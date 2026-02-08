package agent

import (
	"context"

	"github.com/jg-phare/goat/pkg/llm"
	"github.com/jg-phare/goat/pkg/types"
)

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
	ShouldCompact(messages []llm.ChatMessage, model string) bool
	Compact(ctx context.Context, messages []llm.ChatMessage) ([]llm.ChatMessage, error)
}
