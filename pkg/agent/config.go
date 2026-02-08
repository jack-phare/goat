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

	// Streaming
	IncludePartial bool // emit stream_event messages for each SSE chunk

	// Debug
	Debug bool

	// Environment
	OS    string // runtime.GOOS
	Shell string // $SHELL

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
