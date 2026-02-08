# Spec 02: Message Types & Protocol

**Go Package**: `pkg/types/`
**Source References**:
- `sdk.d.ts:1371` — `SDKMessage` union type (16 variants)
- `sdk.d.ts:1140-1598` — All individual message type definitions
- `sdk.d.ts:449-818` — `Options` type (full query configuration surface)
- `sdk.d.ts:1173-1280` — `SDKControlRequest`/`SDKControlResponse` (transport control protocol)
- `sdk.d.ts:400-447` — `ModelInfo`, `ModelUsage`, `NonNullableUsage`
- `sdk.d.ts:250-252` — `ExitReason`
- `sdk.d.ts:820-822` — `OutputFormat`
- `sdk.d.ts:829` — `PermissionMode`
- `sdk.d.ts:1621` — `SettingSource`
- `sdk-tools.d.ts:1-1570` — Tool input schemas (canonical definitions in Spec 04)

---

## 1. Purpose

This spec defines the complete message taxonomy for the system. Every message flowing through the agentic loop, session storage, transport layer, and hook system is an instance of the `SDKMessage` union. These types form the canonical contract between all components.

Additionally, this spec defines the `Options` configuration type and the control protocol types used by the transport layer.

---

## 2. Message Lifecycle

```
    User Input
        │
        ▼
  ┌─────────────┐    ┌──────────────────┐
  │ SDKSystem    │───▶│ Transport Layer  │
  │ Message      │    │ (emits to client)│
  │ (subtype:    │    └────────┬─────────┘
  │  init)       │             │
  └─────────────┘             │
        │                      │
        ▼                      ▼
  ┌─────────────┐    ┌──────────────────┐
  │ SDKUser      │    │ SDKPartial       │──── streaming deltas
  │ Message      │    │ AssistantMessage │     (if includePartial)
  └──────┬──────┘    └──────────────────┘
         │
         ▼
  ┌─────────────┐    ┌──────────────────┐
  │ SDKAssistant │───▶│ SDKToolProgress  │──── long tool exec
  │ Message      │    │ Message          │
  └──────┬──────┘    └──────────────────┘
         │
         │  (if tool_use)
         ▼
  ┌─────────────┐    ┌──────────────────┐
  │ SDKHook*     │───▶│ Pre/PostToolUse  │──── hook lifecycle
  │ Messages     │    │ Started/Progress │
  └──────┬──────┘    │ /Response        │
         │           └──────────────────┘
         │
         │  (loop continues until terminal)
         ▼
  ┌─────────────┐    ┌──────────────────┐
  │ SDKResult    │    │ SDKCompact       │──── if context overflow
  │ Message      │    │ BoundaryMessage  │
  │ (success or  │    └──────────────────┘
  │  error)      │
  └─────────────┘
```

---

## 3. SDKMessage Union — The Complete Protocol

Source: `sdk.d.ts:1371`

```typescript
export declare type SDKMessage =
    | SDKAssistantMessage        // Complete model response with BetaMessage
    | SDKUserMessage             // User input (real or synthetic)
    | SDKUserMessageReplay       // Replayed user message on session resume
    | SDKResultMessage           // Terminal message (success or error variant)
    | SDKSystemMessage           // Session init with metadata
    | SDKPartialAssistantMessage // Streaming delta wrapping BetaRawMessageStreamEvent
    | SDKCompactBoundaryMessage  // Context compaction marker
    | SDKStatusMessage           // Status updates (compacting, mode change)
    | SDKHookStartedMessage      // Hook execution started
    | SDKHookProgressMessage     // Hook stdout/stderr streaming
    | SDKHookResponseMessage     // Hook execution completed
    | SDKToolProgressMessage     // Long-running tool heartbeat
    | SDKAuthStatusMessage       // OAuth/auth flow status
    | SDKTaskNotificationMessage // Background subagent task completion
    | SDKFilesPersistedEvent     // File persistence acknowledgment
    | SDKToolUseSummaryMessage;  // Tool use summary for compacted context
```

---

## 4. Go Type Definitions

### 4.1 Discriminator Types

```go
package types

import (
    "encoding/json"
    "github.com/google/uuid"
)

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
    ResultSubtypeErrorDuringExecution      ResultSubtype = "error_during_execution"
    ResultSubtypeErrorMaxTurns             ResultSubtype = "error_max_turns"
    ResultSubtypeErrorMaxBudget            ResultSubtype = "error_max_budget_usd"
    ResultSubtypeErrorMaxStructuredRetries ResultSubtype = "error_max_structured_output_retries"
)
```

### 4.2 SDKMessage Interface

All message types implement this interface for polymorphic handling in channels, session storage, and transport.

```go
// SDKMessage is implemented by all message types in the protocol.
type SDKMessage interface {
    GetType() MessageType
    GetUUID() uuid.UUID
    GetSessionID() string
}

// BaseMessage provides the common fields shared by all SDKMessage variants.
// Embedded in every concrete message type.
type BaseMessage struct {
    UUID      uuid.UUID `json:"uuid"`
    SessionID string    `json:"session_id"`
}

func (b BaseMessage) GetUUID() uuid.UUID   { return b.UUID }
func (b BaseMessage) GetSessionID() string  { return b.SessionID }
```

### 4.3 AssistantMessage (sdk.d.ts:1140-1147)

A complete model response wrapping the accumulated `BetaMessage`.

```go
type AssistantMessage struct {
    BaseMessage
    Type            MessageType      `json:"type"`               // "assistant"
    Message         BetaMessage      `json:"message"`            // Full Anthropic message
    ParentToolUseID *string          `json:"parent_tool_use_id"` // nil for main agent, set for subagents
    Error           *AssistantError  `json:"error,omitempty"`    // Set on LLM errors
}

func (m AssistantMessage) GetType() MessageType { return MessageTypeAssistant }

// AssistantError classifies LLM-level failures.
// Source: sdk.d.ts:1149
type AssistantError string

const (
    ErrAuthenticationFailed AssistantError = "authentication_failed"
    ErrBillingError         AssistantError = "billing_error"
    ErrRateLimit            AssistantError = "rate_limit"
    ErrInvalidRequest       AssistantError = "invalid_request"
    ErrServerError          AssistantError = "server_error"
    ErrUnknown              AssistantError = "unknown"
)
```

### 4.4 BetaMessage & ContentBlock (Anthropic API Mirror)

These types mirror the Anthropic Messages API response format. They are the canonical internal representation used throughout the system (session storage, hook inputs, transport).

```go
// BetaMessage mirrors the Anthropic Messages API response object.
// Referenced by AssistantMessage.Message.
type BetaMessage struct {
    ID           string         `json:"id"`             // "msg_xxx"
    Type         string         `json:"type"`           // "message"
    Role         string         `json:"role"`           // "assistant"
    Content      []ContentBlock `json:"content"`        // Ordered: thinking → text → tool_use
    Model        string         `json:"model"`
    StopReason   *string        `json:"stop_reason"`    // "end_turn"|"tool_use"|"max_tokens"|"stop_sequence"
    StopSequence *string        `json:"stop_sequence"`
    Usage        BetaUsage      `json:"usage"`
}

// ContentBlock is a discriminated union for message content.
// The Type field determines which other fields are populated.
//
// Invariants:
//   - type="text":     Text is set
//   - type="tool_use": ID, Name, Input are set
//   - type="thinking": Thinking is set
type ContentBlock struct {
    Type     string         `json:"type"`               // "text" | "tool_use" | "thinking"
    Text     string         `json:"text,omitempty"`     // type="text"
    ID       string         `json:"id,omitempty"`       // type="tool_use" — "toolu_xxx" or "call_xxx"
    Name     string         `json:"name,omitempty"`     // type="tool_use" — tool name
    Input    map[string]any `json:"input,omitempty"`    // type="tool_use" — parsed JSON arguments
    Thinking string         `json:"thinking,omitempty"` // type="thinking"
}

// BetaUsage mirrors Anthropic's usage fields.
// All fields are non-optional (zero-valued if absent from API response).
// Maps to NonNullableUsage (sdk.d.ts:429-430).
type BetaUsage struct {
    InputTokens              int `json:"input_tokens"`
    OutputTokens             int `json:"output_tokens"`
    CacheReadInputTokens     int `json:"cache_read_input_tokens"`
    CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}
```

### 4.5 PartialAssistantMessage (sdk.d.ts:1373-1379)

Streaming deltas emitted during LLM response streaming. Only emitted when `Options.includePartialMessages` is true.

```go
type PartialAssistantMessage struct {
    BaseMessage
    Type            MessageType `json:"type"`               // "stream_event"
    Event           any         `json:"event"`              // BetaRawMessageStreamEvent equivalent
    ParentToolUseID *string     `json:"parent_tool_use_id"`
}

func (m PartialAssistantMessage) GetType() MessageType { return MessageTypeStreamEvent }
```

### 4.6 UserMessage (sdk.d.ts:1580-1586) & UserMessageReplay (sdk.d.ts:1590-1598)

```go
// UserMessage represents a user input submitted to the agent.
type UserMessage struct {
    BaseMessage
    Type            MessageType  `json:"type"`               // "user"
    Message         MessageParam `json:"message"`            // Anthropic MessageParam
    ParentToolUseID *string      `json:"parent_tool_use_id"` // nil for main agent
    IsSynthetic     bool         `json:"isSynthetic,omitempty"`
    ToolUseResult   any          `json:"tool_use_result,omitempty"`
}

func (m UserMessage) GetType() MessageType { return MessageTypeUser }

// UserMessageReplay wraps a UserMessage replayed during session resume.
type UserMessageReplay struct {
    UserMessage
    IsReplay bool `json:"isReplay"` // always true
}

// MessageParam is the Anthropic API message format used in request construction
// and stored in UserMessage.
type MessageParam struct {
    Role    string `json:"role"`    // "user" | "assistant"
    Content any    `json:"content"` // string | []ContentBlock
}
```

### 4.7 ResultMessage (sdk.d.ts:1401-1436)

The terminal message for a query. Two variants share a single Go struct, discriminated by `Subtype` and `IsError`.

```go
// ResultMessage is the final message emitted when a query completes.
// Source: sdk.d.ts:1418 — SDKResultMessage = SDKResultSuccess | SDKResultError
type ResultMessage struct {
    BaseMessage
    Type             MessageType                `json:"type"`                        // "result"
    Subtype          ResultSubtype              `json:"subtype"`                     // "success" | "error_*"
    DurationMs       int64                      `json:"duration_ms"`                 // Wall-clock time
    DurationAPIMs    int64                      `json:"duration_api_ms"`             // Time in LLM API calls only
    IsError          bool                       `json:"is_error"`
    NumTurns         int                        `json:"num_turns"`
    StopReason       *string                    `json:"stop_reason"`                 // Anthropic stop_reason
    TotalCostUSD     float64                    `json:"total_cost_usd"`
    Usage            BetaUsage                  `json:"usage"`                       // Aggregate across all turns
    ModelUsage       map[string]ModelUsage      `json:"modelUsage"`                  // Per-model breakdown
    PermissionDenials []PermissionDenial        `json:"permission_denials"`

    // Success-only fields
    Result           string                     `json:"result,omitempty"`            // Final text output
    StructuredOutput any                        `json:"structured_output,omitempty"` // When OutputFormat is set

    // Error-only fields
    Errors           []string                   `json:"errors,omitempty"`
}

func (m ResultMessage) GetType() MessageType { return MessageTypeResult }

// ModelUsage tracks per-model token consumption and cost.
// Source: sdk.d.ts:418-427
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
// Source: sdk.d.ts:1381-1385
type PermissionDenial struct {
    ToolName  string         `json:"tool_name"`
    ToolUseID string         `json:"tool_use_id"`
    ToolInput map[string]any `json:"tool_input"`
}
```

### 4.8 SystemMessage — Init (sdk.d.ts:1522-1549)

The first message emitted on session start, containing full session metadata.

```go
type SystemInitMessage struct {
    BaseMessage
    Type              MessageType    `json:"type"`              // "system"
    Subtype           SystemSubtype  `json:"subtype"`           // "init"
    Agents            []string       `json:"agents,omitempty"`
    APIKeySource      string         `json:"apiKeySource"`      // e.g. "env_var", "config_file"
    Betas             []string       `json:"betas,omitempty"`
    ClaudeCodeVersion string         `json:"claude_code_version"`
    CWD               string         `json:"cwd"`
    Tools             []string       `json:"tools"`             // Enabled tool names
    McpServers        []McpServerInfo `json:"mcp_servers"`
    Model             string         `json:"model"`
    PermissionMode    PermissionMode `json:"permissionMode"`
    SlashCommands     []string       `json:"slash_commands"`
    OutputStyle       string         `json:"output_style"`
    Skills            []string       `json:"skills"`
    Plugins           []PluginInfo   `json:"plugins"`
}

func (m SystemInitMessage) GetType() MessageType { return MessageTypeSystem }

type McpServerInfo struct {
    Name   string `json:"name"`
    Status string `json:"status"` // "connected"|"failed"|"needs-auth"|"pending"|"disabled"
}

type PluginInfo struct {
    Name string `json:"name"`
    Path string `json:"path"`
}
```

### 4.9 SystemMessage — Status (sdk.d.ts:1511-1520)

Emitted during status transitions (e.g. entering compaction, mode change).

```go
type StatusMessage struct {
    BaseMessage
    Type           MessageType     `json:"type"`              // "system"
    Subtype        SystemSubtype   `json:"subtype"`           // "status"
    Status         *string         `json:"status"`            // "compacting" | null
    PermissionMode *PermissionMode `json:"permissionMode,omitempty"`
}

func (m StatusMessage) GetType() MessageType { return MessageTypeSystem }
```

### 4.10 SystemMessage — CompactBoundary (sdk.d.ts:1162-1171)

Marker inserted into the message history when context compaction occurs. Messages before this boundary were summarized.

```go
type CompactBoundaryMessage struct {
    BaseMessage
    Type            MessageType     `json:"type"`              // "system"
    Subtype         SystemSubtype   `json:"subtype"`           // "compact_boundary"
    CompactMetadata CompactMetadata `json:"compact_metadata"`
}

func (m CompactBoundaryMessage) GetType() MessageType { return MessageTypeSystem }

type CompactMetadata struct {
    Trigger   string `json:"trigger"`    // "manual" | "auto"
    PreTokens int    `json:"pre_tokens"` // Token count before compaction
}
```

### 4.11 Hook Lifecycle Messages (sdk.d.ts:1341, 1313, 1326)

Three message types track hook execution progress.

```go
// HookStartedMessage — emitted when a hook begins execution.
// Source: sdk.d.ts:1341-1353
type HookStartedMessage struct {
    BaseMessage
    Type      MessageType   `json:"type"`       // "system"
    Subtype   SystemSubtype `json:"subtype"`    // "hook_started"
    HookID    string        `json:"hook_id"`
    HookName  string        `json:"hook_name"`
    HookEvent string        `json:"hook_event"` // e.g. "PreToolUse", "PostToolUse"
}

func (m HookStartedMessage) GetType() MessageType { return MessageTypeSystem }

// HookProgressMessage — streams hook stdout/stderr during execution.
// Source: sdk.d.ts:1313-1324
type HookProgressMessage struct {
    BaseMessage
    Type      MessageType   `json:"type"`       // "system"
    Subtype   SystemSubtype `json:"subtype"`    // "hook_progress"
    HookID    string        `json:"hook_id"`
    HookName  string        `json:"hook_name"`
    HookEvent string        `json:"hook_event"`
    Stdout    string        `json:"stdout"`
    Stderr    string        `json:"stderr"`
    Output    string        `json:"output"`     // Combined output
}

func (m HookProgressMessage) GetType() MessageType { return MessageTypeSystem }

// HookResponseMessage — emitted when a hook completes.
// Source: sdk.d.ts:1326-1339
type HookResponseMessage struct {
    BaseMessage
    Type      MessageType   `json:"type"`       // "system"
    Subtype   SystemSubtype `json:"subtype"`    // "hook_response"
    HookID    string        `json:"hook_id"`
    HookName  string        `json:"hook_name"`
    HookEvent string        `json:"hook_event"`
    Output    string        `json:"output"`
    Stdout    string        `json:"stdout"`
    Stderr    string        `json:"stderr"`
    ExitCode  *int          `json:"exit_code,omitempty"`
    Outcome   string        `json:"outcome"`    // "success" | "error" | "cancelled"
}

func (m HookResponseMessage) GetType() MessageType { return MessageTypeSystem }
```

### 4.12 ToolProgressMessage (sdk.d.ts:1562-1570)

Heartbeat for long-running tool executions, allowing the client to display elapsed time.

```go
type ToolProgressMessage struct {
    BaseMessage
    Type               MessageType `json:"type"`               // "tool_progress"
    ToolUseID          string      `json:"tool_use_id"`
    ToolName           string      `json:"tool_name"`
    ParentToolUseID    *string     `json:"parent_tool_use_id"`
    ElapsedTimeSeconds float64     `json:"elapsed_time_seconds"`
}

func (m ToolProgressMessage) GetType() MessageType { return MessageTypeToolProgress }
```

### 4.13 AuthStatusMessage (sdk.d.ts:1151-1158)

Tracks OAuth/authentication flow status for MCP servers requiring auth.

```go
type AuthStatusMessage struct {
    BaseMessage
    Type             MessageType `json:"type"`             // "auth_status"
    IsAuthenticating bool        `json:"isAuthenticating"`
    Output           []string    `json:"output"`
    Error            string      `json:"error,omitempty"`
}

func (m AuthStatusMessage) GetType() MessageType { return MessageTypeAuthStatus }
```

### 4.14 TaskNotificationMessage (sdk.d.ts:1551-1560)

Emitted when a background subagent task completes, fails, or is stopped.

```go
type TaskNotificationMessage struct {
    BaseMessage
    Type       MessageType   `json:"type"`       // "system"
    Subtype    SystemSubtype `json:"subtype"`    // "task_notification"
    TaskID     string        `json:"task_id"`    // AgentID from subagent manager
    Status     string        `json:"status"`     // "completed" | "failed" | "stopped"
    OutputFile string        `json:"output_file"`
    Summary    string        `json:"summary"`
}

func (m TaskNotificationMessage) GetType() MessageType { return MessageTypeSystem }
```

### 4.15 FilesPersistedEvent (sdk.d.ts:1281-1293)

Emitted after session files are persisted to disk.

```go
type FilesPersistedEvent struct {
    BaseMessage
    Type        MessageType   `json:"type"`         // "system"
    Subtype     SystemSubtype `json:"subtype"`      // "files_persisted"
    Files       []PersistedFile `json:"files"`
    Failed      []FailedFile    `json:"failed"`
    ProcessedAt string          `json:"processed_at"` // ISO 8601
}

func (m FilesPersistedEvent) GetType() MessageType { return MessageTypeSystem }

type PersistedFile struct {
    Filename string `json:"filename"`
    FileID   string `json:"file_id"`
}

type FailedFile struct {
    Filename string `json:"filename"`
    Error    string `json:"error"`
}
```

### 4.16 ToolUseSummaryMessage (sdk.d.ts:1572-1578)

Injected during compaction to summarize tool use blocks that were removed.

```go
type ToolUseSummaryMessage struct {
    BaseMessage
    Type                MessageType `json:"type"` // "tool_use_summary"
    Summary             string      `json:"summary"`
    PrecedingToolUseIDs []string    `json:"preceding_tool_use_ids"`
}

func (m ToolUseSummaryMessage) GetType() MessageType { return MessageTypeToolUseSummary }
```

---

## 5. JSON Serialization

### 5.1 SDKMessage Marshaling/Unmarshaling

Since Go doesn't have union types, we use a two-pass strategy for deserialization (e.g., loading from session JSONL).

```go
// RawSDKMessage is used for first-pass deserialization to extract the discriminator.
type RawSDKMessage struct {
    Type    MessageType    `json:"type"`
    Subtype *SystemSubtype `json:"subtype,omitempty"`
}

// UnmarshalSDKMessage deserializes a JSON blob into the correct concrete SDKMessage type.
func UnmarshalSDKMessage(data []byte) (SDKMessage, error) {
    var raw RawSDKMessage
    if err := json.Unmarshal(data, &raw); err != nil {
        return nil, fmt.Errorf("unmarshal discriminator: %w", err)
    }

    switch raw.Type {
    case MessageTypeAssistant:
        var msg AssistantMessage
        return &msg, json.Unmarshal(data, &msg)

    case MessageTypeUser:
        // Check for isReplay field
        var probe struct{ IsReplay bool `json:"isReplay"` }
        json.Unmarshal(data, &probe)
        if probe.IsReplay {
            var msg UserMessageReplay
            return &msg, json.Unmarshal(data, &msg)
        }
        var msg UserMessage
        return &msg, json.Unmarshal(data, &msg)

    case MessageTypeResult:
        var msg ResultMessage
        return &msg, json.Unmarshal(data, &msg)

    case MessageTypeSystem:
        return unmarshalSystemMessage(data, raw.Subtype)

    case MessageTypeStreamEvent:
        var msg PartialAssistantMessage
        return &msg, json.Unmarshal(data, &msg)

    case MessageTypeToolProgress:
        var msg ToolProgressMessage
        return &msg, json.Unmarshal(data, &msg)

    case MessageTypeAuthStatus:
        var msg AuthStatusMessage
        return &msg, json.Unmarshal(data, &msg)

    case MessageTypeToolUseSummary:
        var msg ToolUseSummaryMessage
        return &msg, json.Unmarshal(data, &msg)

    default:
        return nil, fmt.Errorf("unknown message type: %s", raw.Type)
    }
}

func unmarshalSystemMessage(data []byte, subtype *SystemSubtype) (SDKMessage, error) {
    if subtype == nil {
        return nil, fmt.Errorf("system message missing subtype")
    }
    switch *subtype {
    case SystemSubtypeInit:
        var msg SystemInitMessage
        return &msg, json.Unmarshal(data, &msg)
    case SystemSubtypeStatus:
        var msg StatusMessage
        return &msg, json.Unmarshal(data, &msg)
    case SystemSubtypeCompactBoundary:
        var msg CompactBoundaryMessage
        return &msg, json.Unmarshal(data, &msg)
    case SystemSubtypeHookStarted:
        var msg HookStartedMessage
        return &msg, json.Unmarshal(data, &msg)
    case SystemSubtypeHookProgress:
        var msg HookProgressMessage
        return &msg, json.Unmarshal(data, &msg)
    case SystemSubtypeHookResponse:
        var msg HookResponseMessage
        return &msg, json.Unmarshal(data, &msg)
    case SystemSubtypeFilesPersisted:
        var msg FilesPersistedEvent
        return &msg, json.Unmarshal(data, &msg)
    case SystemSubtypeTaskNotification:
        var msg TaskNotificationMessage
        return &msg, json.Unmarshal(data, &msg)
    default:
        return nil, fmt.Errorf("unknown system subtype: %s", *subtype)
    }
}
```

### 5.2 ContentBlock Custom Marshaling

`ContentBlock` must omit irrelevant fields based on `Type` to match the Anthropic wire format exactly:

```go
// MarshalJSON produces a clean JSON representation with only fields relevant to the block type.
func (cb ContentBlock) MarshalJSON() ([]byte, error) {
    switch cb.Type {
    case "text":
        return json.Marshal(struct {
            Type string `json:"type"`
            Text string `json:"text"`
        }{Type: "text", Text: cb.Text})

    case "tool_use":
        return json.Marshal(struct {
            Type  string         `json:"type"`
            ID    string         `json:"id"`
            Name  string         `json:"name"`
            Input map[string]any `json:"input"`
        }{Type: "tool_use", ID: cb.ID, Name: cb.Name, Input: cb.Input})

    case "thinking":
        return json.Marshal(struct {
            Type     string `json:"type"`
            Thinking string `json:"thinking"`
        }{Type: "thinking", Thinking: cb.Thinking})

    default:
        // Fallback: marshal all fields
        type Alias ContentBlock
        return json.Marshal(Alias(cb))
    }
}
```

---

## 6. Control Protocol Types

The transport layer uses a request/response protocol for out-of-band control commands (interrupt, model change, MCP management, etc.). These messages are separate from `SDKMessage` and flow through the transport bidirectionally.

### 6.1 Control Request (sdk.d.ts:1256-1260)

```go
// ControlRequest is an out-of-band command from client to agent.
type ControlRequest struct {
    Type      string              `json:"type"`       // "control_request"
    RequestID string              `json:"request_id"` // UUID for correlation
    Request   ControlRequestInner `json:"request"`
}

// ControlRequestInner is the discriminated union of control commands.
// Source: sdk.d.ts:1262-1264
type ControlRequestInner struct {
    Subtype string `json:"subtype"` // discriminator

    // interrupt
    // (no additional fields)

    // can_use_tool (permission prompt response)
    ToolName             string             `json:"tool_name,omitempty"`
    Input                map[string]any     `json:"input,omitempty"`
    PermissionSuggestions []PermissionUpdate `json:"permission_suggestions,omitempty"`
    BlockedPath          string             `json:"blocked_path,omitempty"`
    DecisionReason       string             `json:"decision_reason,omitempty"`
    ToolUseID            string             `json:"tool_use_id,omitempty"`
    AgentID              string             `json:"agent_id,omitempty"`
    Description          string             `json:"description,omitempty"`

    // set_permission_mode
    Mode PermissionMode `json:"mode,omitempty"`

    // set_model
    Model string `json:"model,omitempty"`

    // set_max_thinking_tokens
    MaxThinkingTokens *int `json:"max_thinking_tokens,omitempty"` // nil to unset

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
    HookInput  any    `json:"input,omitempty"` // HookInput
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
```

### 6.2 Control Response (sdk.d.ts:1253-1255)

```go
// ControlResponse is the agent's reply to a ControlRequest.
type ControlResponse struct {
    Type     string `json:"type"` // "control_response"
    Response any    `json:"response"`
    // On success: ControlSuccessResponse
    // On error:   ControlErrorResponse
}

type ControlSuccessResponse struct {
    RequestID string `json:"request_id"`
    Result    any    `json:"result,omitempty"`
}

type ControlErrorResponse struct {
    RequestID string `json:"request_id"`
    Error     string `json:"error"`
}
```

### 6.3 Control Cancel Request

```go
// ControlCancelRequest cancels a pending control request.
// Source: sdk.d.ts:1186-1189
type ControlCancelRequest struct {
    Type      string `json:"type"`       // "control_cancel_request"
    RequestID string `json:"request_id"`
}
```

---

## 7. Configuration Types

### 7.1 QueryOptions (sdk.d.ts:449-818)

The complete configuration surface for a `query()` invocation. Maps to `Options` in the SDK.

```go
type QueryOptions struct {
    // === Agent Configuration ===
    Agent   string                       `json:"agent,omitempty"`
    Agents  map[string]AgentDefinition   `json:"agents,omitempty"`

    // === Tool Configuration ===
    AllowedTools    []string             `json:"allowedTools,omitempty"`
    DisallowedTools []string             `json:"disallowedTools,omitempty"`
    Tools           any                  `json:"tools,omitempty"` // []string | ToolsPreset

    // === Model Configuration ===
    Model             string             `json:"model,omitempty"`
    FallbackModel     string             `json:"fallbackModel,omitempty"`
    MaxThinkingTokens *int               `json:"maxThinkingTokens,omitempty"`
    MaxTurns          *int               `json:"maxTurns,omitempty"`
    MaxBudgetUSD      *float64           `json:"maxBudgetUsd,omitempty"`

    // === Session Configuration ===
    CWD                     string       `json:"cwd,omitempty"`
    Continue                bool         `json:"continue,omitempty"`
    Resume                  string       `json:"resume,omitempty"`
    ResumeSessionAt         string       `json:"resumeSessionAt,omitempty"`
    SessionID               string       `json:"sessionId,omitempty"`
    PersistSession          *bool        `json:"persistSession,omitempty"`   // default true
    ForkSession             bool         `json:"forkSession,omitempty"`
    EnableFileCheckpointing bool         `json:"enableFileCheckpointing,omitempty"`
    AdditionalDirectories   []string     `json:"additionalDirectories,omitempty"`

    // === Permission Configuration ===
    PermissionMode                  PermissionMode `json:"permissionMode,omitempty"`
    AllowDangerouslySkipPermissions bool           `json:"allowDangerouslySkipPermissions,omitempty"`
    PermissionPromptToolName        string         `json:"permissionPromptToolName,omitempty"`

    // === Prompt Configuration ===
    SystemPrompt SystemPromptConfig `json:"systemPrompt,omitempty"`

    // === MCP Servers ===
    McpServers    map[string]McpServerConfig `json:"mcpServers,omitempty"`
    StrictMcpConfig bool                     `json:"strictMcpConfig,omitempty"`

    // === Hooks ===
    Hooks map[HookEvent][]HookCallbackMatcher `json:"hooks,omitempty"`

    // === Plugins ===
    Plugins []PluginConfig `json:"plugins,omitempty"`

    // === Streaming ===
    IncludePartialMessages bool `json:"includePartialMessages,omitempty"`

    // === Feature Flags ===
    Betas        []string      `json:"betas,omitempty"`
    OutputFormat *OutputFormat `json:"outputFormat,omitempty"`

    // === Settings Sources ===
    SettingSources []SettingSource `json:"settingSources,omitempty"`

    // === Sandbox ===
    Sandbox *SandboxSettings `json:"sandbox,omitempty"`

    // === Debug ===
    Debug     bool   `json:"debug,omitempty"`
    DebugFile string `json:"debugFile,omitempty"`

    // === Callbacks (Go function types, not serializable) ===
    CanUseTool  CanUseToolFunc  `json:"-"`
    AbortSignal context.Context `json:"-"`
    Stderr      func(string)    `json:"-"`
}
```

### 7.2 Supporting Configuration Types

```go
// PermissionMode controls tool execution authorization.
// Source: sdk.d.ts:829
type PermissionMode string

const (
    PermissionModeDefault           PermissionMode = "default"
    PermissionModeAcceptEdits       PermissionMode = "acceptEdits"
    PermissionModeBypassPermissions PermissionMode = "bypassPermissions"
    PermissionModePlan              PermissionMode = "plan"
    PermissionModeDelegate          PermissionMode = "delegate"
    PermissionModeDontAsk           PermissionMode = "dontAsk"
)

// SettingSource identifies which settings files to load.
// Source: sdk.d.ts:1621
type SettingSource string

const (
    SettingSourceUser    SettingSource = "user"    // ~/.claude/settings.json
    SettingSourceProject SettingSource = "project" // .claude/settings.json
    SettingSourceLocal   SettingSource = "local"   // .claude/settings.local.json
)

// OutputFormat for structured response schemas.
// Source: sdk.d.ts:820-822
type OutputFormat struct {
    Type   string         `json:"type"`   // "json_schema"
    Schema map[string]any `json:"schema"` // JSON Schema object
}

// SystemPromptConfig allows custom or preset system prompts.
// Source: sdk.d.ts:796-800
//
// Usage patterns:
//   - Custom: SystemPromptConfig{Raw: "You are a helpful assistant."}
//   - Preset: SystemPromptConfig{Preset: "claude_code"}
//   - Preset+append: SystemPromptConfig{Preset: "claude_code", Append: "Always explain reasoning."}
type SystemPromptConfig struct {
    Raw    string `json:"-"`       // Custom prompt string (mutually exclusive with Preset)
    Preset string `json:"preset"`  // "claude_code"
    Append string `json:"append"`  // Additional instructions appended to preset
}

// MarshalJSON handles the string | object union for SystemPromptConfig.
func (s SystemPromptConfig) MarshalJSON() ([]byte, error) {
    if s.Raw != "" {
        return json.Marshal(s.Raw) // Serialize as plain string
    }
    type alias struct {
        Type   string `json:"type"`
        Preset string `json:"preset"`
        Append string `json:"append,omitempty"`
    }
    return json.Marshal(alias{Type: "preset", Preset: s.Preset, Append: s.Append})
}

// ExitReason describes why a session ended.
// Source: sdk.d.ts:250-252
type ExitReason string

const (
    ExitReasonClear                     ExitReason = "clear"
    ExitReasonLogout                    ExitReason = "logout"
    ExitReasonPromptInputExit           ExitReason = "prompt_input_exit"
    ExitReasonOther                     ExitReason = "other"
    ExitReasonBypassPermissionsDisabled ExitReason = "bypass_permissions_disabled"
)

// ModelInfo describes an available model.
// Source: sdk.d.ts:400-415
type ModelInfo struct {
    Value       string `json:"value"`       // Model identifier for API calls
    DisplayName string `json:"displayName"` // Human-readable name
    Description string `json:"description"` // Capabilities description
}

// HookEvent enumerates all hook event names.
// Source: sdk.d.ts (referenced from hook system)
type HookEvent string

const (
    HookEventPreToolUse      HookEvent = "PreToolUse"
    HookEventPostToolUse     HookEvent = "PostToolUse"
    HookEventPostToolUseFailure HookEvent = "PostToolUseFailure"
    HookEventNotification    HookEvent = "Notification"
    HookEventUserPromptSubmit HookEvent = "UserPromptSubmit"
    HookEventSessionStart    HookEvent = "SessionStart"
    HookEventSessionEnd      HookEvent = "SessionEnd"
    HookEventStop            HookEvent = "Stop"
    HookEventSubagentStart   HookEvent = "SubagentStart"
    HookEventSubagentStop    HookEvent = "SubagentStop"
    HookEventPreCompact      HookEvent = "PreCompact"
    HookEventPermissionRequest HookEvent = "PermissionRequest"
    HookEventSetup           HookEvent = "Setup"
    HookEventTeammateIdle    HookEvent = "TeammateIdle"
    HookEventTaskCompleted   HookEvent = "TaskCompleted"
)

// HookCallbackMatcher routes hook events to specific callback handlers.
// Source: sdk.d.ts:1295-1299
type HookCallbackMatcher struct {
    Matcher         string   `json:"matcher,omitempty"`   // Tool name glob pattern (PreToolUse/PostToolUse)
    HookCallbackIDs []string `json:"hookCallbackIds"`     // Callback IDs to invoke
    Timeout         *int     `json:"timeout,omitempty"`   // Timeout in seconds
}

// CanUseToolFunc is the Go callback type for custom permission handling.
// Returns a PermissionResult determining whether the tool can execute.
type CanUseToolFunc func(toolName string, input map[string]any) (*PermissionResult, error)

// PermissionResult from a CanUseTool callback.
type PermissionResult struct {
    Behavior     string             `json:"behavior"`      // "allow" | "deny" | "ask"
    UpdatedInput map[string]any     `json:"updatedInput,omitempty"`
    Message      string             `json:"message,omitempty"`
    Permissions  []PermissionUpdate `json:"updatedPermissions,omitempty"`
}

// PermissionUpdate describes a permission rule change.
type PermissionUpdate struct {
    Type        string                    `json:"type"`        // "addRules"|"replaceRules"|"removeRules"|"setMode"|"addDirectories"|"removeDirectories"
    Destination string                    `json:"destination"` // "userSettings"|"projectSettings"|"localSettings"|"session"|"cliArg"
    Rule        *PermissionRuleValue      `json:"rule,omitempty"`
    Mode        *PermissionMode           `json:"mode,omitempty"`
    Directories []string                  `json:"directories,omitempty"`
}

type PermissionRuleValue struct {
    ToolName    string `json:"tool_name"`
    RuleContent string `json:"rule_content"` // Semantic pattern, e.g. "run tests"
}
```

### 7.3 MCP Server Configuration

```go
// McpServerConfig is the union type for MCP server connection configuration.
// Discriminated by Type field.
type McpServerConfig struct {
    Type    string            `json:"type"`              // "stdio"|"sse"|"http"|"sdk"|"claudeai-proxy"

    // stdio
    Command string            `json:"command,omitempty"`
    Args    []string          `json:"args,omitempty"`
    Env     map[string]string `json:"env,omitempty"`

    // sse/http
    URL     string            `json:"url,omitempty"`
    Headers map[string]string `json:"headers,omitempty"`

    // sdk
    Name string `json:"name,omitempty"`

    // claudeai-proxy
    ID string `json:"id,omitempty"`
}

// PluginConfig describes a plugin to load.
// Source: sdk.d.ts:1389-1398
type PluginConfig struct {
    Type string `json:"type"` // "local"
    Path string `json:"path"` // Absolute or relative path
}
```

### 7.4 Sandbox Settings

```go
// SandboxSettings controls command execution isolation.
// Source: sdk.d.ts (referenced from Options.sandbox)
type SandboxSettings struct {
    Enabled                    bool                `json:"enabled"`
    AutoAllowBashIfSandboxed   bool                `json:"autoAllowBashIfSandboxed,omitempty"`
    Network                    *SandboxNetworkConfig `json:"network,omitempty"`
    IgnoreViolations           []string            `json:"ignoreViolations,omitempty"`
}

type SandboxNetworkConfig struct {
    AllowLocalBinding bool     `json:"allowLocalBinding,omitempty"`
    AllowUnixSockets  []string `json:"allowUnixSockets,omitempty"`
}
```

### 7.5 AgentDefinition

```go
// AgentDefinition describes a custom subagent type.
// Source: sdk.d.ts (referenced from Options.agents)
type AgentDefinition struct {
    Description     string   `json:"description"`
    Tools           []string `json:"tools,omitempty"`           // nil = inherit all
    DisallowedTools []string `json:"disallowedTools,omitempty"`
    Prompt          string   `json:"prompt"`
    Model           string   `json:"model,omitempty"`           // "sonnet"|"opus"|"haiku" or full model string
    MCPServers      []string `json:"mcpServers,omitempty"`
    MaxTurns        *int     `json:"maxTurns,omitempty"`
}
```

---

## 8. Message Constructors

Convenience constructors ensure correct field population and UUID generation.

```go
func NewAssistantMessage(msg BetaMessage, parentToolUseID *string, sessionID string) *AssistantMessage {
    return &AssistantMessage{
        BaseMessage:     BaseMessage{UUID: uuid.New(), SessionID: sessionID},
        Type:            MessageTypeAssistant,
        Message:         msg,
        ParentToolUseID: parentToolUseID,
    }
}

func NewUserMessage(content string, sessionID string) *UserMessage {
    return &UserMessage{
        BaseMessage: BaseMessage{UUID: uuid.New(), SessionID: sessionID},
        Type:        MessageTypeUser,
        Message:     MessageParam{Role: "user", Content: content},
    }
}

func NewResultSuccess(result string, numTurns int, totalCost float64, usage BetaUsage,
    modelUsage map[string]ModelUsage, duration, apiDuration int64, sessionID string) *ResultMessage {
    return &ResultMessage{
        BaseMessage:  BaseMessage{UUID: uuid.New(), SessionID: sessionID},
        Type:         MessageTypeResult,
        Subtype:      ResultSubtypeSuccess,
        IsError:      false,
        Result:       result,
        NumTurns:     numTurns,
        TotalCostUSD: totalCost,
        Usage:        usage,
        ModelUsage:   modelUsage,
        DurationMs:   duration,
        DurationAPIMs: apiDuration,
    }
}

func NewResultError(subtype ResultSubtype, errors []string, numTurns int, totalCost float64,
    usage BetaUsage, modelUsage map[string]ModelUsage, duration, apiDuration int64, sessionID string) *ResultMessage {
    return &ResultMessage{
        BaseMessage:  BaseMessage{UUID: uuid.New(), SessionID: sessionID},
        Type:         MessageTypeResult,
        Subtype:      subtype,
        IsError:      true,
        Errors:       errors,
        NumTurns:     numTurns,
        TotalCostUSD: totalCost,
        Usage:        usage,
        ModelUsage:   modelUsage,
        DurationMs:   duration,
        DurationAPIMs: apiDuration,
    }
}

func NewSystemInit(config QueryOptions, version string, sessionID string) *SystemInitMessage {
    return &SystemInitMessage{
        BaseMessage:       BaseMessage{UUID: uuid.New(), SessionID: sessionID},
        Type:              MessageTypeSystem,
        Subtype:           SystemSubtypeInit,
        Model:             config.Model,
        ClaudeCodeVersion: version,
        CWD:               config.CWD,
        PermissionMode:    config.PermissionMode,
    }
}

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
```

---

## 9. Verification Checklist

- [ ] **Type completeness**: Every variant in the `SDKMessage` union (sdk.d.ts:1371) has a corresponding Go struct implementing the `SDKMessage` interface
- [ ] **JSON round-trip fidelity**: `json.Marshal` → `UnmarshalSDKMessage` round-trips all 16 message types without data loss
- [ ] **ContentBlock marshaling**: Custom `MarshalJSON` produces clean JSON for each block type — no extra `null`/`""` fields for irrelevant type variants
- [ ] **Discriminator completeness**: `UnmarshalSDKMessage` handles all `MessageType` × `SystemSubtype` combinations, returning clear errors for unknown types
- [ ] **UserMessageReplay detection**: `isReplay: true` is correctly detected and deserialized into `UserMessageReplay`, not `UserMessage`
- [ ] **Options completeness**: All 35+ fields from `Options` (sdk.d.ts:449-818) are represented in `QueryOptions`, including callback types as `json:"-"` fields
- [ ] **Enum coverage**: All values for `PermissionMode` (6), `ResultSubtype` (5), `ExitReason` (5), `HookEvent` (15), `SystemSubtype` (8) are defined
- [ ] **Control protocol**: `ControlRequest`/`ControlResponse` types cover all 13 control subtypes from sdk.d.ts:1262-1264
- [ ] **Null handling**: Optional fields use pointers (`*string`, `*int`, `*bool`); required fields are zero-value safe; `Content any` on `ChatMessage` correctly serializes `nil` as `null`
- [ ] **UUID format**: All UUID fields use `github.com/google/uuid` with RFC 4122 compliance and proper JSON serialization as quoted strings
- [ ] **SystemPromptConfig marshaling**: Correctly serializes as plain string for `Raw`, as `{type:"preset",...}` object for `Preset`
- [ ] **BetaUsage invariant**: All fields are non-optional (zero-valued, never nil) matching `NonNullableUsage` semantics (sdk.d.ts:429-430)
- [ ] **ModelUsage camelCase**: Field names use camelCase JSON tags (`inputTokens`, `outputTokens`) matching the SDK, not snake_case
- [ ] **Constructor correctness**: All `New*` constructors generate fresh UUIDs and populate mandatory fields
- [ ] **FilesPersistedEvent fields**: `files`, `failed`, and `processed_at` fields match sdk.d.ts:1281-1293 exactly
