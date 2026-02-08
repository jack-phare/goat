# Spec 03: Agentic Loop

**Go Package**: `pkg/agent/`
**Source References**:
- `sdk.d.ts:948-1083` — Query interface (AsyncGenerator<SDKMessage>)
- `sdk.d.ts:1083-1085` — `query()` entry point (prompt + options → Query)
- `sdk.d.ts:449-818` — Options type (full configuration surface)
- `sdk.mjs` — Minified loop implementation (closed-source, behavioral reference only)
- Piebald-AI: "System Prompt: Main system prompt (269 tks)"

---

## 1. Purpose

The agentic loop is the core runtime loop that drives tool-use conversations. Claude Code's closed-source binary implements this internally. Our Go port reimplements it directly, calling the LLM API via LiteLLM and executing tools locally.

The loop must:
1. Assemble the system prompt and initial messages
2. Call the LLM with tools enabled
3. Parse the response for `tool_use` content blocks
4. Execute each tool, collecting results
5. Append tool results as `tool_result` content blocks in a `user` message
6. Repeat until `stop_reason != "tool_use"` or a termination condition is met
7. Emit `SDKMessage` events throughout for observability

---

## 2. Loop State Machine

```
                    ┌─────────────┐
                    │  INITIALIZE  │
                    │  (assemble   │
                    │   prompt)    │
                    └──────┬──────┘
                           │
                           ▼
                    ┌─────────────┐
              ┌────▶│  LLM_CALL   │◀────────────────┐
              │     │  (stream     │                  │
              │     │   response)  │                  │
              │     └──────┬──────┘                  │
              │            │                          │
              │            ▼                          │
              │     ┌─────────────┐                  │
              │     │ PARSE_RESP  │                  │
              │     │ (accumulate  │                  │
              │     │  blocks)     │                  │
              │     └──────┬──────┘                  │
              │            │                          │
              │     ┌──────┴──────┐                  │
              │     │             │                  │
              │     ▼             ▼                  │
              │  stop_reason   stop_reason           │
              │  == end_turn   == tool_use           │
              │     │             │                  │
              │     ▼             ▼                  │
              │  ┌────────┐  ┌──────────┐           │
              │  │ FINISH │  │EXEC_TOOLS│───────────┘
              │  └────────┘  │(parallel/ │
              │              │ serial)   │
              │              └──────────┘
              │
              │  (also terminates on)
              │  - max_tokens reached
              │  - maxTurns exceeded
              │  - maxBudgetUsd exceeded
              │  - interrupt() called
              │  - context overflow (→ compact, then retry)
              │  - AbortSignal triggered
              └──────────────────────────────────────
```

---

## 3. Go Types

### 3.1 Agent Configuration

```go
// AgentConfig holds the full configuration for an agentic loop.
// Maps to Options type in sdk.d.ts:449-818.
type AgentConfig struct {
    // Identity
    Model          string            // sdk.d.ts:637 — e.g. "claude-sonnet-4-5-20250929"
    FallbackModel  string            // sdk.d.ts:570
    SystemPrompt   SystemPromptConfig // sdk.d.ts:776-800

    // Execution limits
    MaxTurns       int               // sdk.d.ts:613 — max agentic turns (LLM round-trips)
    MaxBudgetUSD   float64           // sdk.d.ts:619 — USD budget cap
    MaxThinkingTkn *int              // sdk.d.ts:607 — thinking token limit (nil = default)

    // Tool configuration
    Tools          []string          // sdk.d.ts:536-542 — allowed built-in tool names
    DisallowedTools []string         // sdk.d.ts:529
    AllowedTools   []string          // sdk.d.ts:504 — auto-allowed without permission prompt

    // Agent/subagent
    AgentName      string            // sdk.d.ts:479 — named agent to use
    Agents         map[string]AgentDefinition // sdk.d.ts:497

    // Session
    CWD            string            // sdk.d.ts:525
    SessionID      string            // sdk.d.ts:709
    Resume         string            // sdk.d.ts:701 — session ID to resume
    Continue       bool              // sdk.d.ts:521
    PersistSession bool              // sdk.d.ts:600

    // Permissions
    PermissionMode PermissionMode    // sdk.d.ts:663
    CanUseTool     CanUseToolFunc    // sdk.d.ts:511

    // Hooks
    Hooks          map[HookEvent][]HookCallbackMatcher // sdk.d.ts:584-594

    // MCP
    MCPServers     map[string]MCPServerConfig // sdk.d.ts:634

    // Streaming
    IncludePartial bool              // sdk.d.ts:597

    // Context
    AdditionalDirs []string          // sdk.d.ts:457
    SettingSources []SettingSource    // sdk.d.ts:752
    Betas          []string          // sdk.d.ts:578

    // Debug
    Debug          bool              // sdk.d.ts:759
    DebugFile      string            // sdk.d.ts:764

    // Abort
    Ctx            context.Context   // maps to abortController (sdk.d.ts:453)
}
```

### 3.2 Loop State

```go
// LoopState tracks the mutable state of a running agentic loop.
type LoopState struct {
    SessionID     string
    Messages      []Message          // conversation history
    TurnCount     int                // number of LLM round-trips completed
    TotalUsage    Usage              // accumulated token usage
    TotalCostUSD  float64            // accumulated cost
    IsInterrupted bool               // set by interrupt()
    ExitReason    ExitReason         // why the loop terminated
}

// ExitReason matches sdk.d.ts exit reasons.
type ExitReason string

const (
    ExitEndTurn      ExitReason = "end_turn"
    ExitMaxTurns     ExitReason = "max_turns"
    ExitMaxBudget    ExitReason = "error_max_budget_usd"
    ExitInterrupted  ExitReason = "interrupted"
    ExitMaxTokens    ExitReason = "max_tokens"
    ExitAborted      ExitReason = "aborted"
)
```

### 3.3 Query Interface (Go channel-based equivalent)

```go
// Query is the Go equivalent of the SDK's AsyncGenerator<SDKMessage>.
// Callers receive messages on the channel, and can control execution via methods.
type Query struct {
    Messages <-chan SDKMessage       // streamed output messages
    done     chan struct{}

    mu       sync.Mutex
    state    *LoopState
    cancel   context.CancelFunc
}

// Control methods (map to sdk.d.ts:948-1083 Query interface)
func (q *Query) Interrupt() error
func (q *Query) SetPermissionMode(mode PermissionMode) error
func (q *Query) SetModel(model string) error
func (q *Query) SetMaxThinkingTokens(tokens *int) error
func (q *Query) Close()

// Informational methods
func (q *Query) SessionID() string
func (q *Query) TotalUsage() Usage
```

---

## 4. Loop Algorithm (Pseudocode)

```
func RunLoop(prompt string, config AgentConfig) Query:
    state = LoopState{SessionID: newUUID()}

    // 1. Fire SessionStart hook
    fireHook(SessionStart, {source: "startup"})

    // 2. Assemble system prompt (see Spec 05)
    systemPrompt = assembleSystemPrompt(config)

    // 3. Build initial messages
    state.Messages = [UserMessage{content: prompt}]

    // 4. If resuming, prepend conversation history
    if config.Resume != "":
        state.Messages = loadSession(config.Resume) + state.Messages

    // 5. Main loop
    loop:
        // Check termination conditions
        if state.TurnCount >= config.MaxTurns:
            state.ExitReason = ExitMaxTurns; break
        if state.TotalCostUSD >= config.MaxBudgetUSD:
            state.ExitReason = ExitMaxBudget; break
        if state.IsInterrupted:
            state.ExitReason = ExitInterrupted; break
        if ctx.Done():
            state.ExitReason = ExitAborted; break

        // 6. Call LLM (see Spec 01)
        request = buildCompletionRequest(systemPrompt, state.Messages, config)
        stream = llmClient.Complete(ctx, request)

        // 7. Stream and accumulate response
        assistantMsg = accumulateStream(stream, emitChan)
        state.Messages = append(state.Messages, assistantMsg)
        state.TotalUsage += assistantMsg.Usage
        state.TurnCount++

        // Emit SDKAssistantMessage
        emit(SDKAssistantMessage{message: assistantMsg})

        // 8. Check stop reason
        switch assistantMsg.StopReason:
        case "end_turn":
            state.ExitReason = ExitEndTurn; break loop
        case "max_tokens":
            // Context overflow — attempt compaction (see Spec 09)
            if shouldCompact(state):
                compact(state)
                continue loop
            state.ExitReason = ExitMaxTokens; break loop
        case "tool_use":
            // 9. Execute tools
            toolUseBlocks = extractToolUseBlocks(assistantMsg)
            toolResults = []

            for _, block := range toolUseBlocks:
                // Fire PreToolUse hook
                hookResult = fireHook(PreToolUse, {tool_name: block.Name, tool_input: block.Input})
                if hookResult.Decision == "deny":
                    toolResults = append(toolResults, ToolResult{
                        ToolUseID: block.ID,
                        Content:   hookResult.Message,
                        IsError:   true,
                    })
                    continue

                // Permission check (see Spec 06)
                permitted = checkPermission(block.Name, block.Input, config)
                if !permitted.Allow:
                    toolResults = append(toolResults, ToolResult{
                        ToolUseID: block.ID,
                        Content:   permitted.DenyMessage,
                        IsError:   true,
                    })
                    continue

                // Execute tool (see Spec 04)
                result, err = executeTool(block.Name, block.Input)
                if err != nil:
                    fireHook(PostToolUseFailure, {error: err.Error()})
                    toolResults = append(toolResults, ToolResult{
                        ToolUseID: block.ID,
                        Content:   err.Error(),
                        IsError:   true,
                    })
                else:
                    fireHook(PostToolUse, {tool_response: result})
                    toolResults = append(toolResults, ToolResult{
                        ToolUseID: block.ID,
                        Content:   result,
                        IsError:   false,
                    })

            // 10. Append tool results as user message
            userMsg = UserMessage{
                Content: toolResultsToContentBlocks(toolResults),
            }
            state.Messages = append(state.Messages, userMsg)
            continue loop

    // 11. Build result message
    resultMsg = buildResultMessage(state)
    emit(resultMsg)

    // 12. Fire SessionEnd hook
    fireHook(SessionEnd, {reason: state.ExitReason})

    // 13. Persist session if enabled
    if config.PersistSession:
        saveSession(state)

    return
```

---

## 5. LLM Request Construction

### 5.1 Building the Completion Request

Each turn assembles a request for LiteLLM's `/v1/chat/completions`:

```go
func buildCompletionRequest(systemPrompt string, messages []Message, config AgentConfig) CompletionRequest {
    return CompletionRequest{
        Model:       config.Model,
        Messages:    convertToOpenAIMessages(systemPrompt, messages),
        Tools:       buildToolDefinitions(config),
        Stream:      true,
        MaxTokens:   16384,  // default, adjustable
        Temperature: 1.0,    // Claude default
        // Thinking config (if model supports extended thinking)
        // Anthropic-specific headers passed via LiteLLM extra_headers
    }
}
```

### 5.2 Message Conversion for LiteLLM

```
Anthropic native                    → LiteLLM OpenAI format
─────────────────────────────────── → ─────────────────────────────────
system: "prompt"                    → {"role": "system", "content": "prompt"}
{role: "user", content: "text"}     → {"role": "user", "content": "text"}
{role: "assistant", content: [...]} → {"role": "assistant", "content": "text",
  [{type: "text"}, {type: "tool_use"}]  "tool_calls": [{type: "function", ...}]}
{role: "user", content: [           → {"role": "tool", "tool_call_id": "...",
  {type: "tool_result", ...}]}          "content": "..."}
```

Critical: LiteLLM expects one `"tool"` message per `tool_result`, not an array in a single `user` message.

---

## 6. Tool Execution Ordering

### 6.1 Serial Execution (Default)

Tools execute serially in the order they appear in the assistant's response. This ensures deterministic behavior for tools with side effects (e.g., FileWrite followed by Bash).

### 6.2 Parallel Execution (Future)

Read-only tools (FileRead, Glob, Grep, WebSearch) could execute in parallel. The loop should tag tools with their side-effect profile from the tool registry.

```go
type ToolSideEffect int
const (
    SideEffectNone     ToolSideEffect = iota // FileRead, Glob, Grep
    SideEffectReadOnly                        // WebSearch, WebFetch
    SideEffectMutating                        // Bash, FileWrite, FileEdit
)
```

---

## 7. Context Overflow Handling

When the LLM returns `stop_reason: "max_tokens"` or the accumulated message history approaches the context window limit, the loop triggers compaction (see Spec 09).

```go
func shouldCompact(state *LoopState) bool {
    // Heuristic: if total input+output tokens > 80% of context window
    contextLimit := getContextLimit(state.Model) // e.g. 200k for Sonnet
    return state.TotalUsage.InputTokens + state.TotalUsage.OutputTokens > int(float64(contextLimit) * 0.8)
}
```

On `max_tokens` specifically:
1. If the response was truncated mid-text → attempt compaction, retry
2. If the response was truncated mid-tool-use → the tool_use block is incomplete; discard it and compact
3. After compaction, the loop continues from the compacted state

---

## 8. Interrupt Handling

```go
func (q *Query) Interrupt() error {
    q.mu.Lock()
    defer q.mu.Unlock()
    q.state.IsInterrupted = true
    // Cancel any in-flight LLM stream
    q.cancel()
    return nil
}
```

Interrupt is checked at three points:
1. Before each LLM call
2. Before each tool execution
3. During stream accumulation (via context cancellation)

---

## 9. Cost Tracking

```go
// Per-model pricing (approximate, configurable)
type ModelPricing struct {
    InputPerMTok  float64 // USD per million input tokens
    OutputPerMTok float64 // USD per million output tokens
}

func updateCost(state *LoopState, usage Usage, pricing ModelPricing) {
    state.TotalCostUSD += float64(usage.InputTokens) * pricing.InputPerMTok / 1_000_000
    state.TotalCostUSD += float64(usage.OutputTokens) * pricing.OutputPerMTok / 1_000_000
}
```

Budget check happens after each LLM response, before tool execution.

---

## 10. SDKMessage Emission

The loop emits messages matching the SDK's `SDKMessage` union. Key emissions:

| Loop Phase | SDKMessage Type | Trigger |
|------------|----------------|---------|
| Start | `system` (subtype: `init`) | Session initialization |
| LLM streaming | `stream_event` | Each SSE chunk (if `includePartialMessages`) |
| LLM complete | `assistant` | Full assistant response accumulated |
| Tool start | `tool_progress` | Before tool execution begins |
| Tool complete | `tool_progress` | Tool result received |
| Compaction | `system` (subtype: `compact_boundary`) | Context compacted |
| Finish | `result` | Loop terminates |

---

## 11. Error Recovery

| Error | Recovery |
|-------|----------|
| LLM 429 (rate limit) | Exponential backoff + retry (see Spec 01) |
| LLM 529 (overloaded) | Backoff + retry |
| LLM 500/502/503 | Retry up to 3 times |
| Tool execution error | Return error as `tool_result` with `is_error: true` |
| Context overflow | Compact and retry (see Spec 09) |
| Network timeout | Retry with backoff |
| AbortSignal | Clean exit with `ExitAborted` |

---

## 12. Verification Checklist

- [ ] **Turn counting**: `TurnCount` increments exactly once per LLM round-trip, matching SDK behavior
- [ ] **Stop reason mapping**: All 4 stop reasons (`end_turn`, `tool_use`, `max_tokens`, `stop_sequence`) handled correctly
- [ ] **Tool result format**: Each `tool_result` maps to a separate `"role": "tool"` message for LiteLLM
- [ ] **MaxTurns enforcement**: Loop terminates after exactly `MaxTurns` LLM calls
- [ ] **Budget enforcement**: Loop terminates when `TotalCostUSD >= MaxBudgetUSD`
- [ ] **Interrupt behavior**: `Interrupt()` cancels in-flight stream and stops before next tool
- [ ] **Context overflow**: `max_tokens` triggers compaction, not immediate termination
- [ ] **Message ordering**: Tool results appear in same order as tool_use blocks in assistant response
- [ ] **SDKMessage emission order**: `system.init` → `assistant*` → `result` matches SDK stream
- [ ] **Session persistence**: Session saved to disk after loop completes (when enabled)
- [ ] **Hook firing order**: PreToolUse → (execute) → PostToolUse/PostToolUseFailure for each tool
- [ ] **Empty tool_use edge case**: If assistant returns 0 tool_use blocks with stop_reason=tool_use, treat as end_turn
- [ ] **Multi-tool per turn**: Multiple tool_use blocks in single response all execute and return
