package types

import (
	"context"
	"encoding/json"
)

// QueryOptions is the complete configuration surface for a query() invocation.
type QueryOptions struct {
	// Agent Configuration
	Agent  string                     `json:"agent,omitempty"`
	Agents map[string]AgentDefinition `json:"agents,omitempty"`

	// Tool Configuration
	AllowedTools    []string `json:"allowedTools,omitempty"`
	DisallowedTools []string `json:"disallowedTools,omitempty"`
	Tools           any      `json:"tools,omitempty"`

	// Model Configuration
	Model             string   `json:"model,omitempty"`
	FallbackModel     string   `json:"fallbackModel,omitempty"`
	MaxThinkingTokens *int     `json:"maxThinkingTokens,omitempty"`
	MaxTurns          *int     `json:"maxTurns,omitempty"`
	MaxBudgetUSD      *float64 `json:"maxBudgetUsd,omitempty"`

	// Session Configuration
	CWD                     string   `json:"cwd,omitempty"`
	Continue                bool     `json:"continue,omitempty"`
	Resume                  string   `json:"resume,omitempty"`
	ResumeSessionAt         string   `json:"resumeSessionAt,omitempty"`
	SessionID               string   `json:"sessionId,omitempty"`
	PersistSession          *bool    `json:"persistSession,omitempty"`
	ForkSession             bool     `json:"forkSession,omitempty"`
	EnableFileCheckpointing bool     `json:"enableFileCheckpointing,omitempty"`
	AdditionalDirectories   []string `json:"additionalDirectories,omitempty"`

	// Permission Configuration
	PermissionMode                  PermissionMode `json:"permissionMode,omitempty"`
	AllowDangerouslySkipPermissions bool           `json:"allowDangerouslySkipPermissions,omitempty"`
	PermissionPromptToolName        string         `json:"permissionPromptToolName,omitempty"`

	// Prompt Configuration
	SystemPrompt SystemPromptConfig `json:"systemPrompt,omitempty"`

	// MCP Servers
	McpServers      map[string]McpServerConfig `json:"mcpServers,omitempty"`
	StrictMcpConfig bool                       `json:"strictMcpConfig,omitempty"`

	// Hooks
	Hooks map[HookEvent][]HookCallbackMatcher `json:"hooks,omitempty"`

	// Plugins
	Plugins []PluginConfig `json:"plugins,omitempty"`

	// Streaming
	IncludePartialMessages bool `json:"includePartialMessages,omitempty"`

	// Feature Flags
	Betas        []string      `json:"betas,omitempty"`
	OutputFormat *OutputFormat `json:"outputFormat,omitempty"`

	// Settings Sources
	SettingSources []SettingSource `json:"settingSources,omitempty"`

	// Sandbox
	Sandbox *SandboxSettings `json:"sandbox,omitempty"`

	// Debug
	Debug     bool   `json:"debug,omitempty"`
	DebugFile string `json:"debugFile,omitempty"`

	// Callbacks (Go function types, not serializable)
	CanUseTool  CanUseToolFunc  `json:"-"`
	AbortSignal context.Context `json:"-"`
	Stderr      func(string)    `json:"-"`
}

// SettingSource identifies which settings files to load.
type SettingSource string

const (
	SettingSourceUser    SettingSource = "user"
	SettingSourceProject SettingSource = "project"
	SettingSourceLocal   SettingSource = "local"
)

// OutputFormat for structured response schemas.
type OutputFormat struct {
	Type   string         `json:"type"`
	Schema map[string]any `json:"schema"`
}

// SystemPromptConfig allows custom or preset system prompts.
type SystemPromptConfig struct {
	Raw    string `json:"-"`
	Preset string `json:"preset"`
	Append string `json:"append"`
}

// MarshalJSON handles the string | object union for SystemPromptConfig.
func (s SystemPromptConfig) MarshalJSON() ([]byte, error) {
	if s.Raw != "" {
		return json.Marshal(s.Raw)
	}
	type alias struct {
		Type   string `json:"type"`
		Preset string `json:"preset"`
		Append string `json:"append,omitempty"`
	}
	return json.Marshal(alias{Type: "preset", Preset: s.Preset, Append: s.Append})
}

// ExitReason describes why a session ended.
type ExitReason string

const (
	ExitReasonClear                     ExitReason = "clear"
	ExitReasonLogout                    ExitReason = "logout"
	ExitReasonPromptInputExit           ExitReason = "prompt_input_exit"
	ExitReasonOther                     ExitReason = "other"
	ExitReasonBypassPermissionsDisabled ExitReason = "bypass_permissions_disabled"
)

// ModelInfo describes an available model.
type ModelInfo struct {
	Value       string `json:"value"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
}

// HookEvent enumerates all hook event names.
type HookEvent string

const (
	HookEventPreToolUse         HookEvent = "PreToolUse"
	HookEventPostToolUse        HookEvent = "PostToolUse"
	HookEventPostToolUseFailure HookEvent = "PostToolUseFailure"
	HookEventNotification       HookEvent = "Notification"
	HookEventUserPromptSubmit   HookEvent = "UserPromptSubmit"
	HookEventSessionStart       HookEvent = "SessionStart"
	HookEventSessionEnd         HookEvent = "SessionEnd"
	HookEventStop               HookEvent = "Stop"
	HookEventSubagentStart      HookEvent = "SubagentStart"
	HookEventSubagentStop       HookEvent = "SubagentStop"
	HookEventPreCompact         HookEvent = "PreCompact"
	HookEventPermissionRequest  HookEvent = "PermissionRequest"
	HookEventSetup              HookEvent = "Setup"
	HookEventTeammateIdle       HookEvent = "TeammateIdle"
	HookEventTaskCompleted      HookEvent = "TaskCompleted"
)

// HookCallbackMatcher routes hook events to specific callback handlers.
type HookCallbackMatcher struct {
	Matcher         string   `json:"matcher,omitempty"`
	HookCallbackIDs []string `json:"hookCallbackIds"`
	Timeout         *int     `json:"timeout,omitempty"`
}

// CanUseToolFunc is the Go callback type for custom permission handling.
type CanUseToolFunc func(toolName string, input map[string]any) (*PermissionResult, error)

// PermissionResult from a CanUseTool callback.
type PermissionResult struct {
	Behavior     string             `json:"behavior"`
	UpdatedInput map[string]any     `json:"updatedInput,omitempty"`
	Message      string             `json:"message,omitempty"`
	Permissions  []PermissionUpdate `json:"updatedPermissions,omitempty"`
}

// PermissionUpdate describes a permission rule change.
type PermissionUpdate struct {
	Type        string               `json:"type"`
	Destination string               `json:"destination"`
	Rule        *PermissionRuleValue `json:"rule,omitempty"`
	Mode        *PermissionMode      `json:"mode,omitempty"`
	Directories []string             `json:"directories,omitempty"`
}

// PermissionRuleValue describes a permission rule.
type PermissionRuleValue struct {
	ToolName    string `json:"tool_name"`
	RuleContent string `json:"rule_content"`
}

// AgentDefinition describes a custom subagent type.
type AgentDefinition struct {
	Description     string   `json:"description"`
	Tools           []string `json:"tools,omitempty"`
	DisallowedTools []string `json:"disallowedTools,omitempty"`
	Prompt          string   `json:"prompt"`
	Model           string   `json:"model,omitempty"`
	MCPServers      []string `json:"mcpServers,omitempty"`
	MaxTurns        *int     `json:"maxTurns,omitempty"`
}
