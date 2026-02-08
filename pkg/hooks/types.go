package hooks

import "context"

// BaseHookInput is embedded in all hook inputs.
type BaseHookInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	CWD            string `json:"cwd"`
	PermissionMode string `json:"permission_mode,omitempty"`
}

// PreToolUseHookInput is the input for PreToolUse hooks.
type PreToolUseHookInput struct {
	BaseHookInput
	HookEventName string `json:"hook_event_name"`
	ToolName      string `json:"tool_name"`
	ToolInput     any    `json:"tool_input"`
	ToolUseID     string `json:"tool_use_id"`
}

// PostToolUseHookInput is the input for PostToolUse hooks.
type PostToolUseHookInput struct {
	BaseHookInput
	HookEventName string `json:"hook_event_name"`
	ToolName      string `json:"tool_name"`
	ToolInput     any    `json:"tool_input"`
	ToolResponse  any    `json:"tool_response"`
	ToolUseID     string `json:"tool_use_id"`
}

// PostToolUseFailureHookInput is the input for PostToolUseFailure hooks.
type PostToolUseFailureHookInput struct {
	BaseHookInput
	HookEventName string `json:"hook_event_name"`
	ToolName      string `json:"tool_name"`
	ToolInput     any    `json:"tool_input"`
	ToolUseID     string `json:"tool_use_id"`
	Error         string `json:"error"`
	IsInterrupt   bool   `json:"is_interrupt,omitempty"`
}

// SessionStartHookInput is the input for SessionStart hooks.
type SessionStartHookInput struct {
	BaseHookInput
	HookEventName string `json:"hook_event_name"`
	Source        string `json:"source"`
	AgentType     string `json:"agent_type,omitempty"`
	Model         string `json:"model,omitempty"`
}

// SessionEndHookInput is the input for SessionEnd hooks.
type SessionEndHookInput struct {
	BaseHookInput
	HookEventName string `json:"hook_event_name"`
	Reason        string `json:"reason"`
}

// StopHookInput is the input for Stop hooks.
type StopHookInput struct {
	BaseHookInput
	HookEventName  string `json:"hook_event_name"`
	StopHookActive bool   `json:"stop_hook_active"`
}

// SubagentStartHookInput is the input for SubagentStart hooks.
type SubagentStartHookInput struct {
	BaseHookInput
	HookEventName string `json:"hook_event_name"`
	AgentID       string `json:"agent_id"`
	AgentType     string `json:"agent_type"`
}

// SubagentStopHookInput is the input for SubagentStop hooks.
type SubagentStopHookInput struct {
	BaseHookInput
	HookEventName       string `json:"hook_event_name"`
	StopHookActive      bool   `json:"stop_hook_active"`
	AgentID             string `json:"agent_id"`
	AgentTranscriptPath string `json:"agent_transcript_path"`
	AgentType           string `json:"agent_type"`
}

// PreCompactHookInput is the input for PreCompact hooks.
type PreCompactHookInput struct {
	BaseHookInput
	HookEventName      string  `json:"hook_event_name"`
	Trigger            string  `json:"trigger"`
	CustomInstructions *string `json:"custom_instructions"`
}

// NotificationHookInput is the input for Notification hooks.
type NotificationHookInput struct {
	BaseHookInput
	HookEventName    string `json:"hook_event_name"`
	Message          string `json:"message"`
	Title            string `json:"title,omitempty"`
	NotificationType string `json:"notification_type"`
}

// UserPromptSubmitHookInput is the input for UserPromptSubmit hooks.
type UserPromptSubmitHookInput struct {
	BaseHookInput
	HookEventName string `json:"hook_event_name"`
	Prompt        string `json:"prompt"`
}

// SetupHookInput is the input for Setup hooks.
type SetupHookInput struct {
	BaseHookInput
	HookEventName string `json:"hook_event_name"`
	Trigger       string `json:"trigger"`
}

// TaskCompletedHookInput is the input for TaskCompleted hooks.
type TaskCompletedHookInput struct {
	BaseHookInput
	HookEventName   string `json:"hook_event_name"`
	TaskID          string `json:"task_id"`
	TaskSubject     string `json:"task_subject"`
	TaskDescription string `json:"task_description,omitempty"`
	TeammateName    string `json:"teammate_name,omitempty"`
	TeamName        string `json:"team_name,omitempty"`
}

// TeammateIdleHookInput is the input for TeammateIdle hooks.
type TeammateIdleHookInput struct {
	BaseHookInput
	HookEventName string `json:"hook_event_name"`
	TeammateName  string `json:"teammate_name"`
	TeamName      string `json:"team_name"`
}

// PermissionRequestHookInput is the input for PermissionRequest hooks.
type PermissionRequestHookInput struct {
	BaseHookInput
	HookEventName string         `json:"hook_event_name"`
	ToolName      string         `json:"tool_name"`
	ToolInput     map[string]any `json:"tool_input"`
}

// --- Output Types ---

// SyncHookJSONOutput is the synchronous return value from hooks.
type SyncHookJSONOutput struct {
	Continue           *bool  `json:"continue,omitempty"`
	SuppressOutput     *bool  `json:"suppressOutput,omitempty"`
	StopReason         string `json:"stopReason,omitempty"`
	Decision           string `json:"decision,omitempty"`
	SystemMessage      string `json:"systemMessage,omitempty"`
	Reason             string `json:"reason,omitempty"`
	HookSpecificOutput any    `json:"hookSpecificOutput,omitempty"`
}

// AsyncHookJSONOutput signals that the hook will complete asynchronously.
type AsyncHookJSONOutput struct {
	Async        bool `json:"async"`
	AsyncTimeout int  `json:"asyncTimeout,omitempty"`
}

// HookJSONOutput is the union of sync and async outputs.
type HookJSONOutput struct {
	Sync  *SyncHookJSONOutput
	Async *AsyncHookJSONOutput
}

// --- Event-Specific Output Types ---

// PreToolUseSpecificOutput is the hook-specific output for PreToolUse.
type PreToolUseSpecificOutput struct {
	HookEventName            string         `json:"hookEventName"`
	PermissionDecision       string         `json:"permissionDecision,omitempty"`
	PermissionDecisionReason string         `json:"permissionDecisionReason,omitempty"`
	UpdatedInput             map[string]any `json:"updatedInput,omitempty"`
	AdditionalContext        string         `json:"additionalContext,omitempty"`
}

// PostToolUseSpecificOutput is the hook-specific output for PostToolUse.
type PostToolUseSpecificOutput struct {
	HookEventName        string `json:"hookEventName"`
	AdditionalContext    string `json:"additionalContext,omitempty"`
	UpdatedMCPToolOutput any    `json:"updatedMCPToolOutput,omitempty"`
}

// PostToolUseFailureSpecificOutput is the hook-specific output for PostToolUseFailure.
type PostToolUseFailureSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext,omitempty"`
}

// SessionStartSpecificOutput is the hook-specific output for SessionStart.
type SessionStartSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext,omitempty"`
}

// SubagentStartSpecificOutput is the hook-specific output for SubagentStart.
type SubagentStartSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext,omitempty"`
}

// SetupSpecificOutput is the hook-specific output for Setup.
type SetupSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext,omitempty"`
}

// NotificationSpecificOutput is the hook-specific output for Notification.
type NotificationSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext,omitempty"`
}

// UserPromptSubmitSpecificOutput is the hook-specific output for UserPromptSubmit.
type UserPromptSubmitSpecificOutput struct {
	HookEventName string `json:"hookEventName"`
}

// HookCallback is the Go function type for hook implementations.
type HookCallback func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error)

// CallbackMatcher groups callbacks with an optional tool name matcher and timeout.
type CallbackMatcher struct {
	Matcher  string         // tool name pattern (glob or exact), empty = match all
	Hooks    []HookCallback // Go function callbacks
	Commands []string       // shell command hooks
	Timeout  int            // seconds, 0 = no timeout
}
