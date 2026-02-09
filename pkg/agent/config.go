package agent

import (
	"os"
	"runtime"

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

	// Multi-turn mode
	MultiTurn bool // if true, loop waits for more input after end_turn instead of exiting

	// Streaming
	IncludePartial bool // emit stream_event messages for each SSE chunk

	// Debug
	Debug     bool
	DebugFile string // path for debug output

	// Model control
	FallbackModel   string // for automatic model fallback
	MaxThinkingTkns *int   // thinking token limit (wire to LLM request)

	// Additional directories for prompt assembly
	AdditionalDirs []string

	// Beta features
	Betas []string

	// Environment
	OS          string // runtime.GOOS
	OSVersion   string // e.g., "Darwin 25.2.0"
	CurrentDate string // e.g., "2026-02-09"
	Shell       string // $SHELL

	// Feature toggles
	SessionsEnabled bool
	MemoryEnabled   bool
	LearningMode    bool
	ScratchpadDir   string

	// Content
	ClaudeMDContent string   // pre-loaded CLAUDE.md content
	OutputStyle     string   // output style config (empty = default)
	SlashCommands   []string // registered slash command names

	// Git state (snapshot at session start)
	GitBranch        string
	GitMainBranch    string
	GitStatus        string
	GitRecentCommits string

	// MCP
	MCPServers map[string]types.McpServerConfig

	// Permission configuration
	AllowedTools                    []string
	DisallowedTools                 []string
	AllowDangerouslySkipPermissions bool
	CanUseTool                      types.CanUseToolFunc

	// Agent identity
	AgentType string // "" for main agent, "explore", "plan", "task", etc.

	// Subagent behavior
	BackgroundMode    bool // auto-deny unpermitted tools, disable AskUser, no MCP
	CanSpawnSubagents bool // false = Agent tool filtered from registry

	// Team context (set when running as a teammate)
	TeamName  string // non-empty when part of a team
	AgentName string // this agent's name within the team

	// Prompt version
	PromptVersion string // e.g., "2.1.37"

	// Dependencies (injected)
	LLMClient    llm.Client
	ToolRegistry *tools.Registry
	Prompter     SystemPromptAssembler
	Permissions  PermissionChecker
	Hooks        HookRunner
	Compactor    ContextCompactor
	CostTracker  *llm.CostTracker
	SessionStore SessionStore // nil = no persistence (default)
}

// DefaultConfig returns an AgentConfig with sensible defaults.
// The caller must still provide LLMClient and ToolRegistry.
func DefaultConfig() AgentConfig {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	return AgentConfig{
		Model:          "claude-sonnet-4-5-20250929",
		MaxTurns:       100,
		PermissionMode: types.PermissionModeDefault,
		OS:             runtime.GOOS,
		Shell:          shell,
		PromptVersion:  "2.1.37",
		Prompter:       &StaticPromptAssembler{Prompt: "You are a helpful assistant."},
		Permissions:    &AllowAllChecker{},
		Hooks:          &NoOpHookRunner{},
		Compactor:      &NoOpCompactor{},
		CostTracker:    llm.NewCostTracker(),
	}
}
