# Agentic Loop + Tool Registry Implementation Plan

## Overview
Implement `pkg/agent/` (Spec 03) and `pkg/tools/` (Spec 04) — the core agentic loop and tool registry for the Go port of Claude Code. The loop drives tool-use conversations by calling the LLM via the existing `pkg/llm/` client, parsing tool_use blocks, executing tools via the registry, and looping until a termination condition is met.

Dependencies on Spec 05 (System Prompt), Spec 06 (Permissions), Spec 07 (Hooks), and Spec 09 (Compaction) are handled via interfaces with minimal stub implementations, allowing those systems to be built in future plans.

## Current State
- **`pkg/llm/`** — Fully implemented: `Client.Complete()`, `Stream.AccumulateWithCallback()`, `CompletionResponse`, `CostTracker`, `EmitAssistantMessage/EmitStreamEvent`, `buildCompletionRequest()`, `convertToToolMessages()`, `ToolResult`, `Tool` interface (Name/Description/InputSchema)
- **`pkg/types/`** — Fully implemented: `SDKMessage` interface, `AssistantMessage`, `UserMessage`, `ResultMessage`, `SystemInitMessage`, `CompactBoundaryMessage`, `ToolProgressMessage`, `PartialAssistantMessage`, `QueryOptions`, `ContentBlock`, `BetaMessage`, `BetaUsage`, control types, hook event types, permission types, constructors
- **`pkg/agent/`** — Does not exist
- **`pkg/tools/`** — Does not exist

## Desired End State
- A working agentic loop that:
  - Takes a prompt + config, returns a `Query` with a `<-chan types.SDKMessage`
  - Calls `llm.Client.Complete()` in a loop
  - Handles stop reasons: `end_turn`, `tool_use`, `max_tokens`
  - Executes tools via a `Registry` implementing Spec 04's interface
  - Enforces turn limits, budget limits, and context cancellation
  - Emits SDKMessages (init, assistant, stream_event, tool_progress, result)
  - Passes through to interface boundaries for permissions, hooks, system prompt, and compaction
- A tool registry with 6 core tools: Bash, FileRead, FileWrite, FileEdit, Glob, Grep
- Full test coverage with httptest mocks for the LLM and real tool execution tests

## Decisions (Resolved)
- **Query API**: Channel-based (`<-chan types.SDKMessage`), matching the spec's AsyncGenerator pattern
- **Tool execution**: Via `tools.Registry` (Spec 04 compatible)
- **Dependencies**: Interfaces for system prompt, permissions, hooks, compaction — stub implementations for testing
- **Tool priority**: Core 6 tools first (Bash, FileRead, FileWrite, FileEdit, Glob, Grep)
- **Bash execution**: Real `exec.CommandContext` — permission system handles safety
- **Tool interface**: Must be compatible with both `llm.Tool` (for request construction) and `tools.Tool` (for execution)

---

## Phases

### Phase 1: Dependency Interfaces & Stubs
**Goal**: Define the interfaces the loop depends on, with minimal stub implementations so we can build and test the loop immediately.

**Changes**:
- `pkg/agent/interfaces.go` — Loop dependency interfaces:
  ```go
  // SystemPromptAssembler builds the system prompt for an LLM call.
  type SystemPromptAssembler interface {
      Assemble(config *AgentConfig) string
  }

  // PermissionChecker gates tool execution.
  type PermissionChecker interface {
      Check(ctx context.Context, toolName string, input map[string]any) (PermissionResult, error)
  }

  type PermissionResult struct {
      Allowed      bool
      DenyMessage  string
      UpdatedInput map[string]any // nil if unchanged
  }

  // HookRunner fires lifecycle hooks.
  type HookRunner interface {
      Fire(ctx context.Context, event types.HookEvent, input any) ([]HookResult, error)
  }

  type HookResult struct {
      Decision       string // "allow"|"deny"|""
      Message        string
      Continue       *bool
      SystemMessage  string
  }

  // ContextCompactor handles context overflow.
  type ContextCompactor interface {
      ShouldCompact(messages []llm.ChatMessage, model string) bool
      Compact(ctx context.Context, messages []llm.ChatMessage) ([]llm.ChatMessage, error)
  }
  ```
- `pkg/agent/stubs.go` — Default stub implementations:
  - `StaticPromptAssembler{Prompt string}` — returns a fixed string
  - `AllowAllChecker{}` — always allows
  - `NoOpHookRunner{}` — returns empty results
  - `NoOpCompactor{}` — never compacts

**Success Criteria**:
- Automated:
  - [ ] `go build ./pkg/agent/...` compiles
  - [ ] `go vet ./pkg/agent/...` passes

---

### Phase 2: Agent Configuration & Loop State
**Goal**: Define the configuration and mutable state types from Spec 03 §3.1-§3.2.

**Changes**:
- `pkg/agent/config.go`:
  - `AgentConfig` struct — mirrors Spec 03 §3.1 but references the dependency interfaces rather than embedding their implementations:
    ```go
    type AgentConfig struct {
        Model          string
        SystemPrompt   types.SystemPromptConfig
        MaxTurns       int       // 0 = unlimited
        MaxBudgetUSD   float64   // 0 = unlimited
        CWD            string
        SessionID      string
        PermissionMode types.PermissionMode
        Debug          bool

        // Dependencies (injected)
        LLMClient      llm.Client
        ToolRegistry   *tools.Registry
        Prompter       SystemPromptAssembler
        Permissions    PermissionChecker
        Hooks          HookRunner
        Compactor      ContextCompactor
        CostTracker    *llm.CostTracker
    }
    ```
  - `func DefaultConfig() AgentConfig` — sensible defaults (MaxTurns=100, model from env)
- `pkg/agent/state.go`:
  - `LoopState` struct — mutable per-session state:
    ```go
    type LoopState struct {
        SessionID    string
        Messages     []llm.ChatMessage  // conversation history in OpenAI format
        TurnCount    int
        TotalUsage   types.BetaUsage
        TotalCostUSD float64
        IsInterrupted bool
        ExitReason   ExitReason
    }
    ```
  - `ExitReason` enum constants matching Spec 03 §3.2

**Success Criteria**:
- Automated:
  - [ ] `go build ./pkg/agent/...` compiles
  - [ ] `go vet ./pkg/agent/...` passes

---

### Phase 3: Tool Registry (Spec 04 §3.2)
**Goal**: Implement the tool registry that manages available tools and resolves them by name.

**Changes**:
- `pkg/tools/tool.go` — Tool interface (compatible with both `llm.Tool` and Spec 04):
  ```go
  // Tool is the interface every tool must implement.
  type Tool interface {
      Name() string
      Description() string
      InputSchema() map[string]any  // JSON Schema for the tools array
      SideEffect() SideEffectType
      Execute(ctx context.Context, input map[string]any) (ToolOutput, error)
  }

  type SideEffectType int
  const (
      SideEffectNone     SideEffectType = iota
      SideEffectReadOnly
      SideEffectMutating
      SideEffectNetwork
      SideEffectBlocking
  )

  type ToolOutput struct {
      Content string
      IsError bool
  }
  ```
- `pkg/tools/registry.go` — Registry implementation:
  ```go
  type Registry struct {
      tools     map[string]Tool
      allowed   map[string]bool  // auto-allowed tools
      disabled  map[string]bool  // explicitly disallowed
  }

  func NewRegistry(opts ...RegistryOption) *Registry
  func (r *Registry) Register(tool Tool)
  func (r *Registry) Get(name string) (Tool, bool)
  func (r *Registry) ToolDefinitions() []llm.ToolDefinition
  func (r *Registry) LLMTools() []llm.Tool  // adapter for buildCompletionRequest
  func (r *Registry) IsAllowed(name string) bool
  func (r *Registry) IsDisabled(name string) bool
  func (r *Registry) Names() []string
  ```
- `pkg/tools/adapter.go` — Bridge between `tools.Tool` and `llm.Tool`:
  ```go
  // llmToolAdapter wraps a tools.Tool to satisfy llm.Tool interface.
  type llmToolAdapter struct { tool Tool }
  func (a *llmToolAdapter) ToolName() string           { return a.tool.Name() }
  func (a *llmToolAdapter) Description() string        { return a.tool.Description() }
  func (a *llmToolAdapter) InputSchema() map[string]any { return a.tool.InputSchema() }
  ```
- `pkg/tools/registry_test.go`:
  - Register/Get/Disable/Allow round-trip tests
  - ToolDefinitions() output format verification
  - Names() ordering

**Success Criteria**:
- Automated:
  - [ ] `go test ./pkg/tools/... -run TestRegistry` passes
  - [ ] `go vet ./pkg/tools/...` passes
  - [ ] `ToolDefinitions()` produces valid OpenAI-format tool definitions
  - [ ] Disabled tools excluded from definitions but still retrievable via Get

---

### Phase 4: Core Tool Implementations (6 tools)
**Goal**: Implement Bash, FileRead, FileWrite, FileEdit, Glob, Grep tools from Spec 04 §4.

**Changes**:
- `pkg/tools/bash.go` + `pkg/tools/bash_test.go`:
  - `BashTool` implementing `Tool` interface
  - `exec.CommandContext` with configurable timeout (default 120s, max 600s)
  - Captures stdout+stderr combined
  - Background execution support via `run_in_background` input field (deferred — returns not-implemented for now)
  - Output truncation if exceeds 30000 chars
  - Input: `command` (required), `timeout` (optional, ms), `description` (optional)
  - Tests: simple command, timeout, stderr capture, large output truncation

- `pkg/tools/fileread.go` + `pkg/tools/fileread_test.go`:
  - `FileReadTool` with CWD context
  - Reads file with line numbers (`cat -n` style)
  - `offset`/`limit` support for partial reads
  - Default: 2000 lines, truncate lines >2000 chars
  - Input: `file_path` (required, must be absolute), `offset` (optional), `limit` (optional)
  - Tests: read full file, offset/limit, nonexistent file error, relative path rejection

- `pkg/tools/filewrite.go` + `pkg/tools/filewrite_test.go`:
  - `FileWriteTool`
  - Creates parent directories as needed
  - Returns line count on success
  - Input: `file_path` (required, absolute), `content` (required)
  - Tests: write new file, overwrite, create nested dirs, empty content

- `pkg/tools/fileedit.go` + `pkg/tools/fileedit_test.go`:
  - `FileEditTool`
  - Find `old_string`, replace with `new_string`
  - `replace_all` flag (default false) — if false, must match exactly once (error on 0 or >1)
  - Returns diff-like snippet showing change context
  - Input: `file_path`, `old_string`, `new_string`, `replace_all` (optional)
  - Tests: single match, multiple match with replace_all, zero matches error, multi-match without replace_all error

- `pkg/tools/glob.go` + `pkg/tools/glob_test.go`:
  - `GlobTool` with CWD context
  - Uses `doublestar` library for recursive `**` patterns (or `filepath.Glob` if no `**`)
  - Returns sorted file list, one per line
  - Input: `pattern` (required), `path` (optional, default CWD)
  - Tests: simple pattern, recursive `**`, no matches, with path override
  - Add `github.com/bmatcuk/doublestar/v4` dependency

- `pkg/tools/grep.go` + `pkg/tools/grep_test.go`:
  - `GrepTool` with CWD context
  - Shells out to `rg` (ripgrep) with mapped flags
  - Falls back to Go `regexp` + `filepath.Walk` if `rg` not available
  - Input: `pattern` (required), `path` (optional), `glob` (optional), `output_mode` (optional: "content"|"files_with_matches"|"count"), `-i` (optional), `-n` (optional, default true), `-A`/`-B`/`-C` (optional), `head_limit` (optional), `type` (optional), `multiline` (optional)
  - Tests: basic search, glob filter, output modes, case insensitive, context lines

**Success Criteria**:
- Automated:
  - [ ] `go test ./pkg/tools/... -run TestBash` passes (including timeout test)
  - [ ] `go test ./pkg/tools/... -run TestFileRead` passes
  - [ ] `go test ./pkg/tools/... -run TestFileWrite` passes
  - [ ] `go test ./pkg/tools/... -run TestFileEdit` passes
  - [ ] `go test ./pkg/tools/... -run TestGlob` passes
  - [ ] `go test ./pkg/tools/... -run TestGrep` passes
  - [ ] `go test -race ./pkg/tools/...` passes
  - [ ] `go vet ./pkg/tools/...` passes
- Manual:
  - [ ] Bash tool can run `echo hello` and return "hello"
  - [ ] FileEdit correctly errors on ambiguous matches

---

### Phase 5: Query Type & SDKMessage Emission
**Goal**: Implement the `Query` type (Spec 03 §3.3) — the public API for running the agentic loop.

**Changes**:
- `pkg/agent/query.go`:
  ```go
  // Query is the Go equivalent of the SDK's AsyncGenerator<SDKMessage>.
  type Query struct {
      messages <-chan types.SDKMessage  // streamed output
      done     chan struct{}

      mu       sync.Mutex
      state    *LoopState
      cancel   context.CancelFunc
  }

  // Public API
  func (q *Query) Messages() <-chan types.SDKMessage
  func (q *Query) Wait()                         // blocks until loop completes
  func (q *Query) Interrupt() error              // cancel in-flight work
  func (q *Query) SessionID() string
  func (q *Query) TotalUsage() types.BetaUsage
  func (q *Query) TotalCostUSD() float64
  func (q *Query) TurnCount() int
  func (q *Query) ExitReason() ExitReason
  ```
- `pkg/agent/emit.go` — helper functions for emitting SDKMessages:
  ```go
  func emitInit(ch chan<- types.SDKMessage, config *AgentConfig, state *LoopState)
  func emitAssistant(ch chan<- types.SDKMessage, resp *llm.CompletionResponse, state *LoopState)
  func emitStreamEvent(ch chan<- types.SDKMessage, chunk *llm.StreamChunk, state *LoopState)
  func emitToolProgress(ch chan<- types.SDKMessage, toolName, toolUseID string, elapsed float64, state *LoopState)
  func emitResult(ch chan<- types.SDKMessage, state *LoopState, startTime time.Time, apiDuration time.Duration)
  ```
- `pkg/agent/query_test.go`:
  - Test Messages() channel receives events
  - Test Interrupt() stops the loop
  - Test Wait() blocks until done

**Success Criteria**:
- Automated:
  - [ ] `go build ./pkg/agent/...` compiles
  - [ ] Query correctly exposes state via accessors
  - [ ] Interrupt() triggers context cancellation

---

### Phase 6: Core Agentic Loop
**Goal**: Implement the main loop algorithm from Spec 03 §4.

**Changes**:
- `pkg/agent/loop.go` — The heart of the system:
  ```go
  // RunLoop starts an agentic loop and returns a Query for observing/controlling it.
  func RunLoop(ctx context.Context, prompt string, config AgentConfig) *Query
  ```

  Internal flow (runs in a goroutine):
  1. Initialize `LoopState` with new session ID
  2. Emit `SystemInitMessage` via channel
  3. Build initial messages: `[{role: "user", content: prompt}]`
  4. Enter main loop:
     a. **Check termination**: MaxTurns, MaxBudgetUSD, IsInterrupted, ctx.Done()
     b. **Build request**: `llm.buildCompletionRequest(clientConfig, systemPrompt, messages, tools, loopState)`
     c. **Call LLM**: `config.LLMClient.Complete(ctx, request)`
     d. **Accumulate with streaming**: `stream.AccumulateWithCallback(onChunk)` — emit stream events via channel
     e. **Update state**: append assistant message, increment turn count, add usage/cost
     f. **Emit AssistantMessage**
     g. **Check stop reason**:
        - `end_turn` → set ExitEndTurn, break
        - `max_tokens` → check compaction, if compactor says yes → compact and retry, else break
        - `tool_use` → extract tool_use blocks, execute each tool, append tool results as user messages, continue
  5. Emit `ResultMessage`
  6. Close channel

- `pkg/agent/tools.go` — Tool execution within the loop:
  ```go
  // executeTools runs each tool_use block and returns tool result messages.
  func executeTools(ctx context.Context, toolBlocks []types.ContentBlock, config *AgentConfig, state *LoopState, ch chan<- types.SDKMessage) []llm.ToolResult
  ```

  For each tool_use block:
  1. Look up tool in registry
  2. Check permissions (via `PermissionChecker` interface)
  3. Fire PreToolUse hook (via `HookRunner` interface)
  4. Execute tool with timeout
  5. Emit `ToolProgressMessage` before/after
  6. Fire PostToolUse/PostToolUseFailure hook
  7. Collect `llm.ToolResult`

- `pkg/agent/messages.go` — Message conversion helpers:
  ```go
  // assistantToOpenAI converts a CompletionResponse to an OpenAI assistant ChatMessage for history.
  func assistantToOpenAI(resp *llm.CompletionResponse) llm.ChatMessage

  // toolResultsToOpenAI converts tool results to OpenAI "tool" role ChatMessages.
  func toolResultsToOpenAI(results []llm.ToolResult) []llm.ChatMessage
  ```

**Success Criteria**:
- Automated:
  - [ ] `go build ./pkg/agent/...` compiles
  - [ ] `go vet ./pkg/agent/...` passes

---

### Phase 7: Loop Tests
**Goal**: Comprehensive test suite for the agentic loop.

**Changes**:
- `pkg/agent/loop_test.go`:

  **Test infrastructure**:
  - `mockLLMClient` — implements `llm.Client`, returns pre-programmed `Stream` responses
  - `mockStream` — feeds chunks from a slice, supports context cancellation
  - `mockTool` — implements `tools.Tool`, records calls, returns configurable output

  **Test cases**:
  1. `TestLoop_SimpleTextResponse` — LLM returns text with stop_reason=end_turn → loop emits init + assistant + result, exits with ExitEndTurn
  2. `TestLoop_SingleToolCall` — LLM returns tool_use → tool executes → tool result sent → LLM returns end_turn → exits
  3. `TestLoop_MultipleToolCalls` — LLM returns 3 tool_use blocks → all execute → all results sent → LLM returns end_turn
  4. `TestLoop_ToolError` — Tool returns error → error sent as tool_result with is_error content → LLM handles gracefully
  5. `TestLoop_MaxTurns` — After N turns, loop exits with ExitMaxTurns
  6. `TestLoop_MaxBudget` — After cost exceeds budget, loop exits with ExitMaxBudget
  7. `TestLoop_Interrupt` — Calling Interrupt() during LLM call → loop exits with ExitInterrupted
  8. `TestLoop_ContextCancel` — Parent context cancelled → loop exits with ExitAborted
  9. `TestLoop_ToolPermissionDenied` — PermissionChecker denies → error result sent to LLM
  10. `TestLoop_StreamEvents` — With includePartial, stream events emitted for each chunk
  11. `TestLoop_EmptyToolUse` — stop_reason=tool_use but 0 tool blocks → treat as end_turn
  12. `TestLoop_UnknownTool` — Tool not in registry → error result
  13. `TestLoop_MessageOrdering` — Verify messages channel delivers in correct order: init, [stream_events], assistant, [tool_progress], result
  14. `TestLoop_CostTracking` — Verify TotalCostUSD accumulates correctly across turns

- `pkg/agent/integration_test.go` (build tag: `integration`):
  - End-to-end test with httptest server simulating multi-turn LLM conversation
  - Tests the full pipeline: prompt → LLM → tool_use → tool execution → tool_result → LLM → end_turn

**Success Criteria**:
- Automated:
  - [ ] `go test ./pkg/agent/... -run TestLoop` passes (all 14 cases)
  - [ ] `go test -race ./pkg/agent/...` passes
  - [ ] `go vet ./pkg/agent/...` passes
  - [ ] Integration test passes with httptest mock LLM server
- Manual:
  - [ ] Message ordering verified: init before assistant before result

---

### Phase 8: End-to-End Integration
**Goal**: Wire everything together and verify the complete system works.

**Changes**:
- `pkg/agent/agent.go` — Convenience constructor:
  ```go
  // New creates a fully wired AgentConfig with sensible defaults.
  func New(llmClient llm.Client, opts ...Option) *AgentConfig

  // Option functions for configuration
  type Option func(*AgentConfig)
  func WithModel(model string) Option
  func WithMaxTurns(n int) Option
  func WithMaxBudget(usd float64) Option
  func WithCWD(dir string) Option
  func WithTools(tools ...tools.Tool) Option
  func WithPermissions(checker PermissionChecker) Option
  func WithHooks(runner HookRunner) Option
  ```
- `pkg/agent/defaults.go` — Default tool set registration:
  ```go
  // DefaultRegistry creates a Registry with the core 6 tools.
  func DefaultRegistry(cwd string) *tools.Registry
  ```
- Update `go.mod` with new dependency: `github.com/bmatcuk/doublestar/v4`
- Full integration test in `pkg/agent/agent_test.go`:
  - Creates agent with mock LLM + real tools
  - Sends "List files in the current directory" → LLM calls Glob → returns results → LLM summarizes → end_turn
  - Verifies entire SDKMessage stream

**Success Criteria**:
- Automated:
  - [ ] `go test ./...` passes (entire project)
  - [ ] `go test -race ./...` passes
  - [ ] `go vet ./...` passes
- Manual:
  - [ ] The full prompt → tool_use → tool_result → response cycle works end-to-end with mock LLM

---

## File Layout Summary

```
pkg/
├── agent/
│   ├── agent.go              — New() convenience constructor with options
│   ├── agent_test.go         — End-to-end integration tests
│   ├── config.go             — AgentConfig, DefaultConfig
│   ├── defaults.go           — DefaultRegistry (core 6 tools)
│   ├── emit.go               — SDKMessage emission helpers
│   ├── interfaces.go         — SystemPromptAssembler, PermissionChecker, HookRunner, ContextCompactor
│   ├── integration_test.go   — Full pipeline test with httptest LLM mock
│   ├── loop.go               — RunLoop() — the core agentic loop
│   ├── loop_test.go          — 14 unit test cases for loop behavior
│   ├── messages.go           — Message conversion helpers
│   ├── query.go              — Query type (channel-based SDKMessage delivery)
│   ├── query_test.go         — Query API tests
│   ├── state.go              — LoopState, ExitReason
│   ├── stubs.go              — Stub implementations of dependency interfaces
│   └── tools.go              — executeTools() — tool execution within the loop
├── tools/
│   ├── adapter.go            — llmToolAdapter (tools.Tool → llm.Tool bridge)
│   ├── bash.go               — BashTool (exec.CommandContext)
│   ├── bash_test.go
│   ├── fileedit.go           — FileEditTool (find-and-replace)
│   ├── fileedit_test.go
│   ├── fileread.go           — FileReadTool (cat -n style)
│   ├── fileread_test.go
│   ├── filewrite.go          — FileWriteTool (create/overwrite)
│   ├── filewrite_test.go
│   ├── glob.go               — GlobTool (doublestar patterns)
│   ├── glob_test.go
│   ├── grep.go               — GrepTool (ripgrep wrapper)
│   ├── grep_test.go
│   ├── registry.go           — Registry (tool catalog)
│   ├── registry_test.go
│   └── tool.go               — Tool interface, SideEffectType, ToolOutput
├── llm/                      — (existing, no changes)
└── types/                    — (existing, no changes)
```

## Out of Scope
- Full system prompt assembly (Spec 05) — interface + stub only
- Permission system implementation (Spec 06) — interface + stub only
- Hook system implementation (Spec 07) — interface + stub only
- Context compaction implementation (Spec 09) — interface + stub only
- MCP tool dynamic registration (Spec 04 §7) — future work
- Session persistence (Spec 11) — future work
- Subagent spawning (Spec 08) — future work
- Tools not in core 6: Agent/Task, WebSearch, WebFetch, TodoWrite, AskUserQuestion, NotebookEdit, Config, ListMcpResources, ReadMcpResource, TaskOutput, TaskStop, ExitPlanMode
- Background task execution (`run_in_background` for Bash/Agent)
- PDF support in FileRead
- Go `regexp` fallback for Grep (ripgrep required for now)

## Open Questions
*None — all resolved.*

## References
- Spec 03: `thoughts/specs/03-AGENTIC-LOOP.md`
- Spec 04: `thoughts/specs/04-TOOL-REGISTRY.md`
- Spec 05: `thoughts/specs/05-SYSTEM-PROMPT.md` (interface reference)
- Spec 06: `thoughts/specs/06-PERMISSION-SYSTEM.md` (interface reference)
- Spec 07: `thoughts/specs/07-HOOK-SYSTEM.md` (interface reference)
- Spec 09: `thoughts/specs/09-CONTEXT-COMPACTION.md` (interface reference)
- LLM client plan: `thoughts/plans/2026-02-08-llm-client.md`
- Existing code: `pkg/llm/`, `pkg/types/`
