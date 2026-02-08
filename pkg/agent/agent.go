package agent

import (
	"github.com/jg-phare/goat/pkg/llm"
	"github.com/jg-phare/goat/pkg/tools"
	"github.com/jg-phare/goat/pkg/types"
)

// Option configures an AgentConfig.
type Option func(*AgentConfig)

// WithModel sets the model name.
func WithModel(model string) Option {
	return func(c *AgentConfig) { c.Model = model }
}

// WithMaxTurns sets the maximum number of LLM round-trips.
func WithMaxTurns(n int) Option {
	return func(c *AgentConfig) { c.MaxTurns = n }
}

// WithMaxBudget sets the maximum USD budget.
func WithMaxBudget(usd float64) Option {
	return func(c *AgentConfig) { c.MaxBudgetUSD = usd }
}

// WithCWD sets the working directory.
func WithCWD(dir string) Option {
	return func(c *AgentConfig) { c.CWD = dir }
}

// WithPermissions sets the permission checker.
func WithPermissions(checker PermissionChecker) Option {
	return func(c *AgentConfig) { c.Permissions = checker }
}

// WithHooks sets the hook runner.
func WithHooks(runner HookRunner) Option {
	return func(c *AgentConfig) { c.Hooks = runner }
}

// WithSystemPrompt sets a custom system prompt string.
func WithSystemPrompt(prompt string) Option {
	return func(c *AgentConfig) {
		c.Prompter = &StaticPromptAssembler{Prompt: prompt}
	}
}

// WithIncludePartial enables streaming of partial messages (stream_event).
func WithIncludePartial(include bool) Option {
	return func(c *AgentConfig) { c.IncludePartial = include }
}

// WithPermissionMode sets the permission mode.
func WithPermissionMode(mode types.PermissionMode) Option {
	return func(c *AgentConfig) { c.PermissionMode = mode }
}

// WithPrompter sets the system prompt assembler.
func WithPrompter(p SystemPromptAssembler) Option {
	return func(c *AgentConfig) { c.Prompter = p }
}

// WithOS sets the operating system identifier.
func WithOS(os string) Option {
	return func(c *AgentConfig) { c.OS = os }
}

// WithShell sets the shell path.
func WithShell(shell string) Option {
	return func(c *AgentConfig) { c.Shell = shell }
}

// WithClaudeMD sets the pre-loaded CLAUDE.md content.
func WithClaudeMD(content string) Option {
	return func(c *AgentConfig) { c.ClaudeMDContent = content }
}

// WithOutputStyle sets the output style configuration.
func WithOutputStyle(style string) Option {
	return func(c *AgentConfig) { c.OutputStyle = style }
}

// WithMCPServers sets the MCP server configurations.
func WithMCPServers(servers map[string]types.McpServerConfig) Option {
	return func(c *AgentConfig) { c.MCPServers = servers }
}

// WithSessionsEnabled enables/disables past sessions access.
func WithSessionsEnabled(enabled bool) Option {
	return func(c *AgentConfig) { c.SessionsEnabled = enabled }
}

// WithMemoryEnabled enables/disables agent memory.
func WithMemoryEnabled(enabled bool) Option {
	return func(c *AgentConfig) { c.MemoryEnabled = enabled }
}

// WithAgentType sets the agent type identifier.
func WithAgentType(agentType string) Option {
	return func(c *AgentConfig) { c.AgentType = agentType }
}

// WithAllowedTools sets tool names that are auto-allowed (no permission prompt).
func WithAllowedTools(names ...string) Option {
	return func(c *AgentConfig) { c.AllowedTools = names }
}

// WithDisallowedTools sets tool names that are explicitly disabled.
func WithDisallowedTools(names ...string) Option {
	return func(c *AgentConfig) { c.DisallowedTools = names }
}

// WithCanUseTool sets the custom permission callback.
func WithCanUseTool(fn types.CanUseToolFunc) Option {
	return func(c *AgentConfig) { c.CanUseTool = fn }
}

// WithAllowDangerouslySkipPermissions enables bypassPermissions mode.
func WithAllowDangerouslySkipPermissions(allow bool) Option {
	return func(c *AgentConfig) { c.AllowDangerouslySkipPermissions = allow }
}

// New creates a fully wired AgentConfig with sensible defaults.
func New(llmClient llm.Client, registry *tools.Registry, opts ...Option) AgentConfig {
	config := DefaultConfig()
	config.LLMClient = llmClient
	config.ToolRegistry = registry

	for _, opt := range opts {
		opt(&config)
	}

	return config
}
