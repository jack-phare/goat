package agent

import (
	"github.com/jg-phare/goat/pkg/llm"
	"github.com/jg-phare/goat/pkg/tools"
	"github.com/jg-phare/goat/pkg/types"
)

// AgentConfig holds the full configuration for an agentic loop.
type AgentConfig struct {
	// Model and prompt
	Model        string
	SystemPrompt types.SystemPromptConfig

	// Execution limits
	MaxTurns     int     // 0 = unlimited
	MaxBudgetUSD float64 // 0 = unlimited

	// Session
	CWD            string
	SessionID      string
	PermissionMode types.PermissionMode

	// Streaming
	IncludePartial bool // emit stream_event messages for each SSE chunk

	// Debug
	Debug bool

	// Dependencies (injected)
	LLMClient    llm.Client
	ToolRegistry *tools.Registry
	Prompter     SystemPromptAssembler
	Permissions  PermissionChecker
	Hooks        HookRunner
	Compactor    ContextCompactor
	CostTracker  *llm.CostTracker
}

// DefaultConfig returns an AgentConfig with sensible defaults.
// The caller must still provide LLMClient and ToolRegistry.
func DefaultConfig() AgentConfig {
	return AgentConfig{
		Model:          "claude-sonnet-4-5-20250929",
		MaxTurns:       100,
		PermissionMode: types.PermissionModeDefault,
		Prompter:       &StaticPromptAssembler{Prompt: "You are a helpful assistant."},
		Permissions:    &AllowAllChecker{},
		Hooks:          &NoOpHookRunner{},
		Compactor:      &NoOpCompactor{},
		CostTracker:    llm.NewCostTracker(),
	}
}
