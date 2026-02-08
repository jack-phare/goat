package types

// ResultMessage is the final message emitted when a query completes.
type ResultMessage struct {
	BaseMessage
	Type              MessageType           `json:"type"`
	Subtype           ResultSubtype         `json:"subtype"`
	DurationMs        int64                 `json:"duration_ms"`
	DurationAPIMs     int64                 `json:"duration_api_ms"`
	IsError           bool                  `json:"is_error"`
	NumTurns          int                   `json:"num_turns"`
	StopReason        *string               `json:"stop_reason"`
	TotalCostUSD      float64               `json:"total_cost_usd"`
	Usage             BetaUsage             `json:"usage"`
	ModelUsage        map[string]ModelUsage `json:"modelUsage"`
	PermissionDenials []PermissionDenial    `json:"permission_denials"`

	// Success-only fields
	Result           string `json:"result,omitempty"`
	StructuredOutput any    `json:"structured_output,omitempty"`

	// Error-only fields
	Errors []string `json:"errors,omitempty"`
}

func (m ResultMessage) GetType() MessageType { return MessageTypeResult }

// ModelUsage tracks per-model token consumption and cost.
type ModelUsage struct {
	InputTokens              int     `json:"inputTokens"`
	OutputTokens             int     `json:"outputTokens"`
	CacheReadInputTokens     int     `json:"cacheReadInputTokens"`
	CacheCreationInputTokens int     `json:"cacheCreationInputTokens"`
	WebSearchRequests        int     `json:"webSearchRequests"`
	CostUSD                  float64 `json:"costUSD"`
	ContextWindow            int     `json:"contextWindow"`
	MaxOutputTokens          int     `json:"maxOutputTokens"`
}

// PermissionDenial records a tool invocation that was denied by the permission system.
type PermissionDenial struct {
	ToolName  string         `json:"tool_name"`
	ToolUseID string         `json:"tool_use_id"`
	ToolInput map[string]any `json:"tool_input"`
}
