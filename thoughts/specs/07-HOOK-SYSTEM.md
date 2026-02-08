# Spec 07: Hook System

**Go Package**: `pkg/hooks/`
**Source References**:
- `sdk.d.ts:254` — `HOOK_EVENTS` constant (15 events)
- `sdk.d.ts:259-277` — HookCallback, HookCallbackMatcher, HookEvent, HookInput, HookJSONOutput
- `sdk.d.ts:75-100` — AsyncHookJSONOutput, BaseHookInput
- `sdk.d.ts:1738-1757` — SyncHookJSONOutput (continue, suppressOutput, stopReason, decision, systemMessage, reason, hookSpecificOutput)
- `sdk.d.ts:584-594` — Options.hooks configuration
- `sdk.d.ts:831-948` — All hook-specific input/output types

---

## 1. Purpose

The hook system provides extension points throughout the agent lifecycle. Hooks allow external code to observe events, modify behavior, inject context, control permissions, and implement custom logic without modifying the core loop.

---

## 2. Hook Events (from `sdk.d.ts:254`)

```go
type HookEvent string

const (
    HookPreToolUse          HookEvent = "PreToolUse"
    HookPostToolUse         HookEvent = "PostToolUse"
    HookPostToolUseFailure  HookEvent = "PostToolUseFailure"
    HookNotification        HookEvent = "Notification"
    HookUserPromptSubmit    HookEvent = "UserPromptSubmit"
    HookSessionStart        HookEvent = "SessionStart"
    HookSessionEnd          HookEvent = "SessionEnd"
    HookStop                HookEvent = "Stop"
    HookSubagentStart       HookEvent = "SubagentStart"
    HookSubagentStop        HookEvent = "SubagentStop"
    HookPreCompact          HookEvent = "PreCompact"
    HookPermissionRequest   HookEvent = "PermissionRequest"
    HookSetup               HookEvent = "Setup"
    HookTeammateIdle        HookEvent = "TeammateIdle"
    HookTaskCompleted       HookEvent = "TaskCompleted"
)
```

---

## 3. Hook Input Types (from `sdk.d.ts`)

### 3.1 Base Input (all hooks receive this)

```go
// BaseHookInput is embedded in all hook inputs. (sdk.d.ts:80-85)
type BaseHookInput struct {
    SessionID      string `json:"session_id"`
    TranscriptPath string `json:"transcript_path"`
    CWD            string `json:"cwd"`
    PermissionMode string `json:"permission_mode,omitempty"`
}
```

### 3.2 Per-Event Inputs

```go
// PreToolUseHookInput (sdk.d.ts:933-938)
type PreToolUseHookInput struct {
    BaseHookInput
    HookEventName string      `json:"hook_event_name"` // "PreToolUse"
    ToolName      string      `json:"tool_name"`
    ToolInput     interface{} `json:"tool_input"`
    ToolUseID     string      `json:"tool_use_id"`
}

// PostToolUseHookInput (sdk.d.ts:913-919)
type PostToolUseHookInput struct {
    BaseHookInput
    HookEventName string      `json:"hook_event_name"` // "PostToolUse"
    ToolName      string      `json:"tool_name"`
    ToolInput     interface{} `json:"tool_input"`
    ToolResponse  interface{} `json:"tool_response"`
    ToolUseID     string      `json:"tool_use_id"`
}

// PostToolUseFailureHookInput (sdk.d.ts:899-906)
type PostToolUseFailureHookInput struct {
    BaseHookInput
    HookEventName string      `json:"hook_event_name"` // "PostToolUseFailure"
    ToolName      string      `json:"tool_name"`
    ToolInput     interface{} `json:"tool_input"`
    ToolUseID     string      `json:"tool_use_id"`
    Error         string      `json:"error"`
    IsInterrupt   bool        `json:"is_interrupt,omitempty"`
}

// SessionStartHookInput (sdk.d.ts:1606-1612)
type SessionStartHookInput struct {
    BaseHookInput
    HookEventName string `json:"hook_event_name"` // "SessionStart"
    Source        string `json:"source"`           // "startup"|"resume"|"clear"|"compact"
    AgentType     string `json:"agent_type,omitempty"`
    Model         string `json:"model,omitempty"`
}

// SessionEndHookInput (sdk.d.ts:1601-1604)
type SessionEndHookInput struct {
    BaseHookInput
    HookEventName string `json:"hook_event_name"` // "SessionEnd"
    Reason        string `json:"reason"`           // ExitReason
}

// StopHookInput (sdk.d.ts:1714-1717)
type StopHookInput struct {
    BaseHookInput
    HookEventName  string `json:"hook_event_name"` // "Stop"
    StopHookActive bool   `json:"stop_hook_active"`
}

// SubagentStartHookInput (sdk.d.ts:1720-1724)
type SubagentStartHookInput struct {
    BaseHookInput
    HookEventName string `json:"hook_event_name"` // "SubagentStart"
    AgentID       string `json:"agent_id"`
    AgentType     string `json:"agent_type"`
}

// SubagentStopHookInput (sdk.d.ts:1730-1736)
type SubagentStopHookInput struct {
    BaseHookInput
    HookEventName       string `json:"hook_event_name"` // "SubagentStop"
    StopHookActive      bool   `json:"stop_hook_active"`
    AgentID             string `json:"agent_id"`
    AgentTranscriptPath string `json:"agent_transcript_path"`
    AgentType           string `json:"agent_type"`
}

// PreCompactHookInput (sdk.d.ts:927-931)
type PreCompactHookInput struct {
    BaseHookInput
    HookEventName      string  `json:"hook_event_name"` // "PreCompact"
    Trigger            string  `json:"trigger"` // "manual"|"auto"
    CustomInstructions *string `json:"custom_instructions"`
}

// NotificationHookInput (sdk.d.ts:433-438)
type NotificationHookInput struct {
    BaseHookInput
    HookEventName    string `json:"hook_event_name"` // "Notification"
    Message          string `json:"message"`
    Title            string `json:"title,omitempty"`
    NotificationType string `json:"notification_type"`
}

// UserPromptSubmitHookInput (sdk.d.ts:1828-1831)
type UserPromptSubmitHookInput struct {
    BaseHookInput
    HookEventName string `json:"hook_event_name"` // "UserPromptSubmit"
    Prompt        string `json:"prompt"`
}

// SetupHookInput (sdk.d.ts:1623-1626)
type SetupHookInput struct {
    BaseHookInput
    HookEventName string `json:"hook_event_name"` // "Setup"
    Trigger       string `json:"trigger"` // "init"|"maintenance"
}

// TaskCompletedHookInput (sdk.d.ts:1759-1766)
type TaskCompletedHookInput struct {
    BaseHookInput
    HookEventName   string `json:"hook_event_name"` // "TaskCompleted"
    TaskID          string `json:"task_id"`
    TaskSubject     string `json:"task_subject"`
    TaskDescription string `json:"task_description,omitempty"`
    TeammateName    string `json:"teammate_name,omitempty"`
    TeamName        string `json:"team_name,omitempty"`
}

// TeammateIdleHookInput (sdk.d.ts:1768-1771)
type TeammateIdleHookInput struct {
    BaseHookInput
    HookEventName string `json:"hook_event_name"` // "TeammateIdle"
    TeammateName  string `json:"teammate_name"`
    TeamName      string `json:"team_name"`
}

// PermissionRequestHookInput — see Spec 06
```

---

## 4. Hook Output Types (from `sdk.d.ts:1738-1757`)

### 4.1 Sync Output

```go
// SyncHookJSONOutput is the synchronous return value from hooks.
type SyncHookJSONOutput struct {
    Continue       *bool   `json:"continue,omitempty"`       // continue processing
    SuppressOutput *bool   `json:"suppressOutput,omitempty"` // hide from user
    StopReason     string  `json:"stopReason,omitempty"`     // custom stop reason
    Decision       string  `json:"decision,omitempty"`       // "approve"|"block"
    SystemMessage  string  `json:"systemMessage,omitempty"`  // inject system message
    Reason         string  `json:"reason,omitempty"`         // explanation

    // Event-specific output (one of):
    HookSpecificOutput interface{} `json:"hookSpecificOutput,omitempty"`
}
```

### 4.2 Async Output

```go
// AsyncHookJSONOutput signals that the hook will complete asynchronously.
type AsyncHookJSONOutput struct {
    Async        bool `json:"async"`
    AsyncTimeout int  `json:"asyncTimeout,omitempty"` // seconds
}
```

### 4.3 Event-Specific Outputs

```go
// PreToolUseHookSpecificOutput (sdk.d.ts:940-946)
type PreToolUseSpecificOutput struct {
    HookEventName          string                 `json:"hookEventName"` // "PreToolUse"
    PermissionDecision     string                 `json:"permissionDecision,omitempty"` // "allow"|"deny"|"ask"
    PermissionDecisionReason string               `json:"permissionDecisionReason,omitempty"`
    UpdatedInput           map[string]interface{} `json:"updatedInput,omitempty"`
    AdditionalContext      string                 `json:"additionalContext,omitempty"`
}

// PostToolUseSpecificOutput (sdk.d.ts:921-925)
type PostToolUseSpecificOutput struct {
    HookEventName       string      `json:"hookEventName"` // "PostToolUse"
    AdditionalContext   string      `json:"additionalContext,omitempty"`
    UpdatedMCPToolOutput interface{} `json:"updatedMCPToolOutput,omitempty"`
}

// PostToolUseFailureSpecificOutput (sdk.d.ts:908-911)
type PostToolUseFailureSpecificOutput struct {
    HookEventName     string `json:"hookEventName"` // "PostToolUseFailure"
    AdditionalContext string `json:"additionalContext,omitempty"`
}

// SessionStartSpecificOutput (sdk.d.ts:1614-1617)
type SessionStartSpecificOutput struct {
    HookEventName     string `json:"hookEventName"` // "SessionStart"
    AdditionalContext string `json:"additionalContext,omitempty"`
}

// SubagentStartSpecificOutput (sdk.d.ts:1726-1729)
type SubagentStartSpecificOutput struct {
    HookEventName     string `json:"hookEventName"` // "SubagentStart"
    AdditionalContext string `json:"additionalContext,omitempty"`
}

// SetupSpecificOutput (sdk.d.ts:1631-1634)
type SetupSpecificOutput struct {
    HookEventName     string `json:"hookEventName"` // "Setup"
    AdditionalContext string `json:"additionalContext,omitempty"`
}

// NotificationSpecificOutput (sdk.d.ts:440-443)
type NotificationSpecificOutput struct {
    HookEventName     string `json:"hookEventName"` // "Notification"
    AdditionalContext string `json:"additionalContext,omitempty"`
}

// UserPromptSubmitSpecificOutput — see UserPromptSubmitHookInput
type UserPromptSubmitSpecificOutput struct {
    HookEventName string `json:"hookEventName"` // "UserPromptSubmit"
    // Can block prompt submission via SyncHookJSONOutput.Decision = "block"
}

// PermissionRequestSpecificOutput — see Spec 06
```

---

## 5. Hook Callback Matcher (from `sdk.d.ts:267-272`)

```go
// HookCallbackMatcher groups callbacks with an optional tool name matcher.
type HookCallbackMatcher struct {
    Matcher  string         // tool name pattern (glob or exact), e.g. "Bash", "mcp__*"
    Hooks    []HookCallback // callbacks to run
    Timeout  int            // seconds, for all hooks in this matcher
}

// HookCallback is the function signature for hook implementations.
// Maps to sdk.d.ts:259-262.
type HookCallback func(input HookInput, toolUseID string, ctx context.Context) (HookJSONOutput, error)

// HookJSONOutput is the union of sync and async outputs.
type HookJSONOutput struct {
    Sync  *SyncHookJSONOutput
    Async *AsyncHookJSONOutput
}
```

---

## 6. Hook Runner

```go
// Runner manages hook registration and execution.
type Runner struct {
    hooks map[HookEvent][]HookCallbackMatcher
}

func NewRunner(config map[HookEvent][]HookCallbackMatcher) *Runner

// Fire executes all matching hooks for an event, collecting results.
func (r *Runner) Fire(ctx context.Context, event HookEvent, input HookInput, toolUseID string) ([]HookJSONOutput, error) {
    matchers, ok := r.hooks[event]
    if !ok {
        return nil, nil
    }

    var results []HookJSONOutput
    for _, matcher := range matchers {
        // Check matcher pattern (for tool-specific hooks)
        if matcher.Matcher != "" && !matchToolName(matcher.Matcher, input) {
            continue
        }

        // Apply timeout
        hookCtx := ctx
        if matcher.Timeout > 0 {
            var cancel context.CancelFunc
            hookCtx, cancel = context.WithTimeout(ctx, time.Duration(matcher.Timeout)*time.Second)
            defer cancel()
        }

        // Run all hooks in matcher
        for _, hook := range matcher.Hooks {
            result, err := hook(input, toolUseID, hookCtx)
            if err != nil {
                // Log error but don't fail the loop
                continue
            }
            results = append(results, result)

            // Check for early termination
            if result.Sync != nil && result.Sync.Continue != nil && !*result.Sync.Continue {
                return results, nil // stop processing further hooks
            }
        }
    }

    return results, nil
}
```

---

## 7. Hook Lifecycle in the Agentic Loop

```
SessionStart ─────────────────────────────────────────────┐
  │                                                        │
  ▼                                                        │
UserPromptSubmit (each user message)                       │
  │                                                        │
  ▼                                                        │
[LLM Call] ──► [For each tool_use block:]                 │
  │              PreToolUse ──► (permission check)         │
  │              │ allow ──► execute ──► PostToolUse       │
  │              │ deny ──► skip                           │
  │              │ error ──► PostToolUseFailure             │
  │                                                        │
  │ (if subagent spawned)                                  │
  │  SubagentStart ──► [subagent loop] ──► SubagentStop   │
  │                                                        │
  │ (if context overflow)                                  │
  │  PreCompact ──► [compact] ──► SessionStart(compact)   │
  │                                                        │
  │ (if stop_reason == end_turn)                           │
  │  Stop ──► (hook can request continuation)              │
  │                                                        │
  │ (task completed in background)                         │
  │  TaskCompleted                                         │
  │                                                        │
  │ (teammate idle notification)                           │
  │  TeammateIdle                                          │
  │                                                        │
  │ (any notification to user)                             │
  │  Notification                                          │
  │                                                        │
  ▼                                                        │
SessionEnd ◄──────────────────────────────────────────────┘
```

---

## 8. Hook Result Processing

The agentic loop processes hook results based on event type:

### PreToolUse Result Handling
```go
func processPreToolUseResults(results []HookJSONOutput) PermissionBehavior {
    for _, r := range results {
        if r.Sync == nil { continue }
        if specific, ok := r.Sync.HookSpecificOutput.(*PreToolUseSpecificOutput); ok {
            if specific.PermissionDecision == "deny" {
                return BehaviorDeny
            }
            if specific.PermissionDecision == "allow" {
                return BehaviorAllow
            }
        }
        if r.Sync.Decision == "block" {
            return BehaviorDeny
        }
    }
    return BehaviorAsk // no hook made a decision
}
```

### Stop Hook Result Handling
```go
func processStopResults(results []HookJSONOutput) bool {
    for _, r := range results {
        if r.Sync != nil && r.Sync.Continue != nil && *r.Sync.Continue {
            return true // continue the loop
        }
    }
    return false // stop as normal
}
```

---

## 9. Additional Context Injection

Several hooks can inject `additionalContext` which is appended to the next system message:

```go
func collectAdditionalContext(results []HookJSONOutput) string {
    var contexts []string
    for _, r := range results {
        if r.Sync == nil { continue }
        if r.Sync.SystemMessage != "" {
            contexts = append(contexts, r.Sync.SystemMessage)
        }
        // Also check hookSpecificOutput.additionalContext for each type
    }
    return strings.Join(contexts, "\n")
}
```

---

## 10. Verification Checklist

- [ ] **Event coverage**: All 15 hook events from `HOOK_EVENTS` constant implemented
- [ ] **Input types**: Each hook event receives its correct input type with all fields
- [ ] **Output types**: Sync and async outputs parsed correctly
- [ ] **Matcher filtering**: Tool name matchers correctly filter hook execution
- [ ] **Timeout enforcement**: Hook timeouts cancel execution after specified duration
- [ ] **Continue semantics**: `continue: false` stops processing further hooks
- [ ] **Decision propagation**: PreToolUse permission decisions flow to permission system
- [ ] **Stop hook**: `continue: true` in Stop hook restarts the loop
- [ ] **Context injection**: additionalContext appended to next LLM system message
- [ ] **Error isolation**: Hook errors logged but don't crash the agentic loop
- [ ] **Async support**: AsyncHookJSONOutput handled with timeout
- [ ] **Ordering**: Hooks within a matcher execute in array order
