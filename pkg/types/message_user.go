package types

// UserMessage represents a user input submitted to the agent.
type UserMessage struct {
	BaseMessage
	Type            MessageType  `json:"type"`
	Message         MessageParam `json:"message"`
	ParentToolUseID *string      `json:"parent_tool_use_id"`
	IsSynthetic     bool         `json:"isSynthetic,omitempty"`
	ToolUseResult   any          `json:"tool_use_result,omitempty"`
}

func (m UserMessage) GetType() MessageType { return MessageTypeUser }

// UserMessageReplay wraps a UserMessage replayed during session resume.
type UserMessageReplay struct {
	UserMessage
	IsReplay bool `json:"isReplay"` // always true
}

// MessageParam is the Anthropic API message format used in request construction.
type MessageParam struct {
	Role    string `json:"role"`    // "user" | "assistant"
	Content any    `json:"content"` // string | []ContentBlock
}
