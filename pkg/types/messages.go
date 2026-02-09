package types

import "github.com/google/uuid"

// MessageType is the top-level discriminator for SDKMessage variants.
type MessageType string

const (
	MessageTypeAssistant      MessageType = "assistant"
	MessageTypeUser           MessageType = "user"
	MessageTypeResult         MessageType = "result"
	MessageTypeSystem         MessageType = "system"
	MessageTypeStreamEvent    MessageType = "stream_event"
	MessageTypeToolProgress   MessageType = "tool_progress"
	MessageTypeAuthStatus     MessageType = "auth_status"
	MessageTypeToolUseSummary MessageType = "tool_use_summary"
)

// SystemSubtype disambiguates system-typed messages.
type SystemSubtype string

const (
	SystemSubtypeInit             SystemSubtype = "init"
	SystemSubtypeStatus           SystemSubtype = "status"
	SystemSubtypeCompactBoundary  SystemSubtype = "compact_boundary"
	SystemSubtypeHookStarted      SystemSubtype = "hook_started"
	SystemSubtypeHookProgress     SystemSubtype = "hook_progress"
	SystemSubtypeHookResponse     SystemSubtype = "hook_response"
	SystemSubtypeFilesPersisted   SystemSubtype = "files_persisted"
	SystemSubtypeTaskNotification SystemSubtype = "task_notification"
)

// ResultSubtype disambiguates result message variants.
type ResultSubtype string

const (
	ResultSubtypeSuccess                   ResultSubtype = "success"
	ResultSubtypeSuccessTurn               ResultSubtype = "success_turn" // per-turn result in multi-turn mode
	ResultSubtypeErrorDuringExecution      ResultSubtype = "error_during_execution"
	ResultSubtypeErrorMaxTurns             ResultSubtype = "error_max_turns"
	ResultSubtypeErrorMaxBudget            ResultSubtype = "error_max_budget_usd"
	ResultSubtypeErrorMaxStructuredRetries ResultSubtype = "error_max_structured_output_retries"
)

// SDKMessage is implemented by all message types in the protocol.
type SDKMessage interface {
	GetType() MessageType
	GetUUID() uuid.UUID
	GetSessionID() string
}

// BaseMessage provides the common fields shared by all SDKMessage variants.
type BaseMessage struct {
	UUID      uuid.UUID `json:"uuid"`
	SessionID string    `json:"session_id"`
}

func (b BaseMessage) GetUUID() uuid.UUID  { return b.UUID }
func (b BaseMessage) GetSessionID() string { return b.SessionID }

// AssistantError classifies LLM-level failures.
type AssistantError string

const (
	ErrAuthenticationFailed AssistantError = "authentication_failed"
	ErrBillingError         AssistantError = "billing_error"
	ErrRateLimit            AssistantError = "rate_limit"
	ErrInvalidRequest       AssistantError = "invalid_request"
	ErrServerError          AssistantError = "server_error"
	ErrUnknown              AssistantError = "unknown"
)

// AssistantMessage is a complete model response wrapping the accumulated BetaMessage.
type AssistantMessage struct {
	BaseMessage
	Type            MessageType     `json:"type"`
	Message         BetaMessage     `json:"message"`
	ParentToolUseID *string         `json:"parent_tool_use_id"`
	Error           *AssistantError `json:"error,omitempty"`
}

func (m AssistantMessage) GetType() MessageType { return MessageTypeAssistant }

// PartialAssistantMessage wraps a StreamChunk as a streaming delta.
type PartialAssistantMessage struct {
	BaseMessage
	Type            MessageType `json:"type"`
	Event           any         `json:"event"`
	ParentToolUseID *string     `json:"parent_tool_use_id"`
}

func (m PartialAssistantMessage) GetType() MessageType { return MessageTypeStreamEvent }
