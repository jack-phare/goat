package types

import "github.com/google/uuid"

// NewAssistantMessage creates an AssistantMessage with a fresh UUID.
func NewAssistantMessage(msg BetaMessage, parentToolUseID *string, sessionID string) *AssistantMessage {
	return &AssistantMessage{
		BaseMessage:     BaseMessage{UUID: uuid.New(), SessionID: sessionID},
		Type:            MessageTypeAssistant,
		Message:         msg,
		ParentToolUseID: parentToolUseID,
	}
}

// NewUserMessage creates a UserMessage with a fresh UUID.
func NewUserMessage(content string, sessionID string) *UserMessage {
	return &UserMessage{
		BaseMessage: BaseMessage{UUID: uuid.New(), SessionID: sessionID},
		Type:        MessageTypeUser,
		Message:     MessageParam{Role: "user", Content: content},
	}
}

// NewResultSuccess creates a successful ResultMessage with a fresh UUID.
func NewResultSuccess(result string, numTurns int, totalCost float64, usage BetaUsage,
	modelUsage map[string]ModelUsage, duration, apiDuration int64, sessionID string) *ResultMessage {
	return &ResultMessage{
		BaseMessage:   BaseMessage{UUID: uuid.New(), SessionID: sessionID},
		Type:          MessageTypeResult,
		Subtype:       ResultSubtypeSuccess,
		IsError:       false,
		Result:        result,
		NumTurns:      numTurns,
		TotalCostUSD:  totalCost,
		Usage:         usage,
		ModelUsage:    modelUsage,
		DurationMs:    duration,
		DurationAPIMs: apiDuration,
	}
}

// NewResultError creates an error ResultMessage with a fresh UUID.
func NewResultError(subtype ResultSubtype, errors []string, numTurns int, totalCost float64,
	usage BetaUsage, modelUsage map[string]ModelUsage, duration, apiDuration int64, sessionID string) *ResultMessage {
	return &ResultMessage{
		BaseMessage:   BaseMessage{UUID: uuid.New(), SessionID: sessionID},
		Type:          MessageTypeResult,
		Subtype:       subtype,
		IsError:       true,
		Errors:        errors,
		NumTurns:      numTurns,
		TotalCostUSD:  totalCost,
		Usage:         usage,
		ModelUsage:    modelUsage,
		DurationMs:    duration,
		DurationAPIMs: apiDuration,
	}
}

// NewSystemInit creates a SystemInitMessage with a fresh UUID.
func NewSystemInit(model string, version string, cwd string, permissionMode PermissionMode, sessionID string) *SystemInitMessage {
	return &SystemInitMessage{
		BaseMessage:       BaseMessage{UUID: uuid.New(), SessionID: sessionID},
		Type:              MessageTypeSystem,
		Subtype:           SystemSubtypeInit,
		Model:             model,
		ClaudeCodeVersion: version,
		CWD:               cwd,
		PermissionMode:    permissionMode,
	}
}

// NewCompactBoundary creates a CompactBoundaryMessage with a fresh UUID.
func NewCompactBoundary(trigger string, preTokens int, sessionID string) *CompactBoundaryMessage {
	return &CompactBoundaryMessage{
		BaseMessage: BaseMessage{UUID: uuid.New(), SessionID: sessionID},
		Type:        MessageTypeSystem,
		Subtype:     SystemSubtypeCompactBoundary,
		CompactMetadata: CompactMetadata{
			Trigger:   trigger,
			PreTokens: preTokens,
		},
	}
}
