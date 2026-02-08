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
