package types

// ControlRequest is an out-of-band command from client to agent.
type ControlRequest struct {
	Type      string              `json:"type"`       // "control_request"
	RequestID string              `json:"request_id"`
	Request   ControlRequestInner `json:"request"`
}

// ControlRequestInner is the discriminated union of control commands.
type ControlRequestInner struct {
	Subtype string `json:"subtype"` // discriminator

	// can_use_tool
	ToolName              string             `json:"tool_name,omitempty"`
	Input                 map[string]any     `json:"input,omitempty"`
	PermissionSuggestions []PermissionUpdate `json:"permission_suggestions,omitempty"`
	BlockedPath           string             `json:"blocked_path,omitempty"`
	DecisionReason        string             `json:"decision_reason,omitempty"`
	ToolUseID             string             `json:"tool_use_id,omitempty"`
	AgentID               string             `json:"agent_id,omitempty"`
	Description           string             `json:"description,omitempty"`

	// set_permission_mode
	Mode PermissionMode `json:"mode,omitempty"`

	// set_model
	Model string `json:"model,omitempty"`

	// set_max_thinking_tokens
	MaxThinkingTokens *int `json:"max_thinking_tokens,omitempty"`

	// mcp_set_servers
	Servers map[string]McpServerConfig `json:"servers,omitempty"`

	// mcp_reconnect, mcp_toggle
	ServerName string `json:"serverName,omitempty"`
	Enabled    *bool  `json:"enabled,omitempty"`

	// rewind_files
	UserMessageID string `json:"user_message_id,omitempty"`
	DryRun        bool   `json:"dry_run,omitempty"`

	// hook_callback
	CallbackID string `json:"callback_id,omitempty"`
	HookInput  any    `json:"hook_input,omitempty"`
}

// ControlRequestSubtype enumerates valid control command subtypes.
const (
	ControlSubtypeInterrupt            = "interrupt"
	ControlSubtypeCanUseTool           = "can_use_tool"
	ControlSubtypeSetPermissionMode    = "set_permission_mode"
	ControlSubtypeSetModel             = "set_model"
	ControlSubtypeSetMaxThinkingTokens = "set_max_thinking_tokens"
	ControlSubtypeMcpStatus            = "mcp_status"
	ControlSubtypeMcpReconnect         = "mcp_reconnect"
	ControlSubtypeMcpToggle            = "mcp_toggle"
	ControlSubtypeMcpSetServers        = "mcp_set_servers"
	ControlSubtypeMcpMessage           = "mcp_message"
	ControlSubtypeRewindFiles          = "rewind_files"
	ControlSubtypeHookCallback         = "hook_callback"
	ControlSubtypeInitialize           = "initialize"
)

// ControlResponse is the agent's reply to a ControlRequest.
type ControlResponse struct {
	Type     string `json:"type"` // "control_response"
	Response any    `json:"response"`
}

// ControlSuccessResponse is a successful control response.
type ControlSuccessResponse struct {
	RequestID string `json:"request_id"`
	Result    any    `json:"result,omitempty"`
}

// ControlErrorResponse is an error control response.
type ControlErrorResponse struct {
	RequestID string `json:"request_id"`
	Error     string `json:"error"`
}

// ControlCancelRequest cancels a pending control request.
type ControlCancelRequest struct {
	Type      string `json:"type"`       // "control_cancel_request"
	RequestID string `json:"request_id"`
}
