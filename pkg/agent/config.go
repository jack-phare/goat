package agent

import (
	"fmt"
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
	MaxTurns     int                // 0 = unlimited
	MaxBudgetUSD float64            // 0 = unlimited
	ModelBudgets map[string]float64 // per-model USD budget limits (model ID → max USD)

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
	FallbackModel            string  // for automatic model fallback
	CompactorModel           string  // model to use for context compaction (default: haiku)
	MaxThinkingTkns          *int    // thinking token limit (wire to LLM request)
	BudgetDowngradeThreshold float64 // fraction of MaxBudgetUSD (0.0-1.0) to trigger downgrade
	BudgetDowngradeModel     string  // model to switch to when threshold is exceeded

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

	// Auto-memory (main agent persistent project-scoped memory)
	AutoMemoryDir     string // resolved auto-memory directory path
	AutoMemoryContent string // loaded MEMORY.md content (first 200 lines)

	// Session memory extraction
	SessionMemoryEnabled bool   // enable background session memory extraction
	SessionMemoryModel   string // lightweight model for extractions (default: haiku)
	SessionDir           string // session directory for memory storage

	// Session cleanup
	CleanupRetentionDays int // sessions older than this are deleted (default: 30)

	// Content
	ManagedPolicyContent string   // OS-level managed CLAUDE.md policy (highest priority)
	ClaudeMDContent      string   // pre-loaded CLAUDE.md content
	OutputStyle          string   // output style config (empty = default)
	SlashCommands        []string // registered slash command names

	// Rules (.claude/rules/ directory)
	ProjectRules    []RuleEntry // rules loaded from .claude/rules/
	UserRules       []RuleEntry // rules loaded from ~/.claude/rules/
	ActiveFilePaths []string    // files currently being worked on (for conditional injection)

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

	// Parallel tool execution
	MaxParallelTools int // max concurrency for side-effect-free tools (0 = default 5)

	// Team context (set when running as a teammate)
	TeamName  string // non-empty when part of a team
	AgentName string // this agent's name within the team

	// Prompt version
	PromptVersion string // e.g., "2.1.37"

	// Context window resolution (injected to avoid import cycle with pkg/context)
	ContextLimitFunc func(model string, betas []string) int

	// Dynamic model selection: automatically choose model based on estimated task complexity.
	DynamicModelConfig *DynamicModelConfig

	// Dependencies (injected)
	LLMClient    llm.Client
	ToolRegistry *tools.Registry
	Prompter     SystemPromptAssembler
	Permissions  PermissionChecker
	Hooks        HookRunner
	Compactor    ContextCompactor
	CostTracker  *llm.CostTracker
	SessionStore SessionStore // nil = no persistence (default)
	Skills       SkillProvider // nil = no skills
}

// RuleEntry is a rule loaded from .claude/rules/ for injection into the system prompt.
type RuleEntry struct {
	Content      string   // markdown body
	PathPatterns []string // glob patterns from frontmatter (empty = unconditional)
}

// DynamicModelConfig configures automatic model selection based on estimated prompt complexity.
type DynamicModelConfig struct {
	// SimpleModel is used when prompt tokens < SimpleThresholdTokens.
	SimpleModel           string
	SimpleThresholdTokens int // default: 1000

	// DefaultModel uses config.Model (no field needed).

	// ComplexModel is used when prompt tokens > ComplexThresholdTokens.
	ComplexModel           string
	ComplexThresholdTokens int // default: 10000
}

// ValidateModel checks if the configured model has pricing data.
// Returns a warning string if the model is unknown (cost tracking will report $0).
// Returns empty string if the model is valid.
func (c *AgentConfig) ValidateModel() string {
	if c.Model == "" {
		return "no model configured"
	}
	if !llm.IsKnownModel(c.Model) {
		return fmt.Sprintf("model %q not found in pricing data; cost tracking will report $0", c.Model)
	}
	return ""
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
