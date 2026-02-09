# SDK Types — Message Protocol & Content Blocks

> `pkg/types/` — The shared type system. Defines all SDK message types,
> content blocks, hook events, permission types, and agent configuration.
> This is the leaf package imported by everything else.

## SDKMessage Interface

Every message flowing through the system implements this interface:

```go
type SDKMessage interface {
    GetType() MessageType       // "system", "assistant", "result", etc.
    GetSessionID() string       // session correlation
    GetUUID() string            // unique message ID
}
```

## Message Type Hierarchy

```
 SDKMessage (interface)
 │
 ├── SystemInitMessage         type=system, subtype=init
 │   Fields: Model, Version, CWD, PermissionMode, Tools[]
 │   When: Start of conversation
 │
 ├── AssistantMessage          type=assistant
 │   Fields: Content[], Model, StopReason, Usage, Duration
 │   When: After full LLM response accumulation
 │
 ├── PartialAssistantMessage   type=assistant, subtype=partial
 │   Fields: Partial content (streaming chunk)
 │   When: Each SSE chunk (if IncludePartial=true)
 │
 ├── UserMessage               type=user
 │   Fields: Content (string)
 │   When: User input (initial + multi-turn follow-ups)
 │
 ├── UserMessageReplay         type=user_replay
 │   Fields: Content (string)
 │   When: During session restore
 │
 ├── ResultMessage             type=result
 │   Fields: Result, Subtype, Duration, TurnCount, Cost, Usage
 │   Subtypes: success, error_max_turns, error_max_budget,
 │             error_during_execution
 │   When: Final message before channel close
 │
 ├── StatusMessage             type=system, subtype=status
 │   Fields: Status, PermissionMode
 │   When: Status transitions
 │
 ├── CompactBoundaryMessage    type=system, subtype=compact_boundary
 │   Fields: MessageCount, SummaryLength
 │   When: After context compaction
 │
 ├── ToolProgressMessage       type=tool_progress
 │   Fields: ToolUseID, ToolName, ElapsedSeconds
 │   When: Before/after each tool execution
 │
 ├── HookStartedMessage        type=system, subtype=hook_started
 │   Fields: HookID, HookName, HookEvent
 │   When: Hook begins execution
 │
 ├── HookProgressMessage       type=system, subtype=hook_progress
 │   Fields: HookID, Stdout, Stderr, Output
 │   When: During shell hook execution (streaming)
 │
 ├── HookResponseMessage       type=system, subtype=hook_response
 │   Fields: HookID, Stdout, Stderr, Outcome
 │   When: Hook completes
 │
 ├── AuthStatusMessage         type=auth_status
 │   Fields: Provider, Status, Message
 │
 ├── TaskNotificationMessage   type=task_notification
 │   Fields: TaskID, Status, AgentName, Message
 │   When: Team task state changes
 │
 └── ToolUseSummaryMessage     type=tool_use_summary
     Fields: ToolName, Input, Output, Duration
```

## Content Block Types

Inside AssistantMessage.Content[]:

```
 ┌──────────────────────────────────────────────────────────────────┐
 │  ContentBlock {                                                  │
 │    Type:      string   // "text" | "tool_use" | "thinking"      │
 │                                                                  │
 │    // Text block:                                                │
 │    Text:      string                                             │
 │                                                                  │
 │    // Tool use block:                                            │
 │    ID:        string   // "call_abc123"                          │
 │    Name:      string   // "Bash"                                 │
 │    Input:     map[string]any  // {"command": "ls"}               │
 │                                                                  │
 │    // Thinking block:                                            │
 │    Thinking:  string   // reasoning content                      │
 │  }                                                               │
 │                                                                  │
 │  Ordering (always): thinking → text → tool_use                  │
 │  This matches Claude's native content block ordering.            │
 └──────────────────────────────────────────────────────────────────┘
```

## BaseMessage — Shared Fields

```go
type BaseMessage struct {
    UUID       string `json:"uuid"`
    SessionID  string `json:"sessionId"`
    ParentUUID string `json:"parentUuid,omitempty"`
    Timestamp  string `json:"timestamp,omitempty"`
}
```

All message types embed BaseMessage for consistent identification.

## Key Enums & Constants

```
 MessageType:
   "system", "assistant", "user", "user_replay", "result",
   "tool_progress", "auth_status", "task_notification",
   "tool_use_summary"

 SystemSubtype:
   "init", "status", "compact_boundary",
   "hook_started", "hook_progress", "hook_response"

 ResultSubtype:
   "success", "success_turn", "error_max_turns",
   "error_max_budget_usd", "error_during_execution"

 PermissionMode:
   "default", "acceptEdits", "bypassPermissions",
   "plan", "delegate", "dontAsk"

 HookEvent (15 events):
   "PreToolUse", "PostToolUse", "PostToolUseFailure",
   "Notification", "UserPromptSubmit", "SessionStart",
   "SessionEnd", "Stop", "SubagentStart", "SubagentStop",
   "PreCompact", "PermissionRequest", "Setup",
   "TeammateIdle", "TaskCompleted"
```

## AgentDefinition — Custom Agent Types

```go
type AgentDefinition struct {
    Description     string   `json:"description"`
    Tools           []string `json:"tools,omitempty"`
    DisallowedTools []string `json:"disallowedTools,omitempty"`
    Prompt          string   `json:"prompt"`
    Model           string   `json:"model,omitempty"`
    MCPServers      []string `json:"mcpServers,omitempty"`
    MaxTurns        *int     `json:"maxTurns,omitempty"`

    // Extended fields (Spec 08)
    Memory       string `json:"memory,omitempty"`       // "auto"|"none"
    AllowMcp     *bool  `json:"allowMcp,omitempty"`
    CustomConfig map[string]any `json:"customConfig,omitempty"`
}
```

## PermissionResult & PermissionUpdate

```
 PermissionResult (from callbacks):
 {
   Behavior:     "allow"|"deny"|"ask"
   UpdatedInput: {command: "safe-ls"}     // optional input rewrite
   Message:      "denied: too dangerous"  // deny reason
   Permissions:  []PermissionUpdate       // suggested rule changes
 }

 PermissionUpdate (rule changes):
 {
   Type:        "addRules"|"replaceRules"|"removeRules"|"setMode"
   Destination: "session"|"userSettings"
   Rule:        {ToolName: "Bash", RuleContent: "npm test"}
   Mode:        "acceptEdits"             // for setMode type
 }
```

## Type Import Graph

```
 pkg/types/ imports NOTHING from the project.
 It is the leaf package — every other package imports it.

 This makes it the foundation layer:
   types ← llm ← tools ← agent ← everything else

 No import cycles possible because types has no project dependencies.
```

## Go vs TS: Type System

```
 ┌────────────────────────────┬──────────────────────────────────────┐
 │ Claude Code TS              │ Goat Go                              │
 ├────────────────────────────┼──────────────────────────────────────┤
 │ Interface + type guards    │ Interface + concrete structs          │
 │ Union types                │ Interface with GetType() method      │
 │ Optional fields (?:)       │ Pointer fields + omitempty           │
 │ Runtime type checking      │ Compile-time interface satisfaction  │
 │ JSON.parse → any           │ json.Unmarshal → typed struct        │
 │ Flexible (can be anything) │ Strict (must match struct exactly)   │
 │                              │                                      │
 │ Same message names, same    │ 1:1 correspondence for SDK compat   │
 │ JSON wire format            │ Go structs serialize identically    │
 └────────────────────────────┴──────────────────────────────────────┘

 The Go types are designed to produce identical JSON output to the
 TypeScript SDK. A consumer reading the SDKMessage stream cannot
 tell whether it came from Claude Code TS or Goat.
```
