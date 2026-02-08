# Hook System Implementation Plan (Spec 07)

## Overview

Implement the full hook system in `pkg/hooks/` that provides extension points throughout the agent lifecycle. Hooks allow external code (Go callbacks and shell commands) to observe events, modify behavior, inject context, control permissions, and implement custom logic. This replaces the current fire-and-forget stub with a fully functional `Runner` that processes results.

## Current State

- **`pkg/agent/interfaces.go:30-41`** — `HookRunner` interface (`Fire(ctx, event, input any) → ([]HookResult, error)`) and `HookResult` struct (4 fields: Decision, Message, Continue, SystemMessage)
- **`pkg/agent/stubs.go:26-31`** — `NoOpHookRunner` returns nil
- **`pkg/agent/loop.go:44-47,153-154,204-207`** — Fires SessionStart, Stop, SessionEnd (results ignored)
- **`pkg/agent/tools.go:88-125`** — Fires PreToolUse, PostToolUse, PostToolUseFailure (results ignored)
- **`pkg/permission/checker.go:112-118,155-183`** — Fires PermissionRequest and processes Decision field
- **`pkg/types/options.go:131-157`** — 15 `HookEvent` constants and `HookCallbackMatcher` struct (Matcher, HookCallbackIDs, Timeout)
- **`pkg/types/message_hooks.go`** — SDK emission types for hook progress

## Desired End State

- Full `pkg/hooks/` package with `Runner` struct implementing `agent.HookRunner`
- All 15 event-specific typed input structs (with `BaseHookInput`)
- Sync and async output types with event-specific output variants
- Support for both Go function callbacks and shell command hooks
- `HookCallbackMatcher` filtering (glob/exact on tool name)
- Timeout enforcement per matcher
- Async hook support with configurable timeout
- Expanded `agent.HookResult` with all SyncHookJSONOutput fields
- Full loop integration: Stop hook `continue=true` restarts loop, PreToolUse results affect permissions, additionalContext injection
- SDK message emission during hook execution (HookStarted, HookProgress, HookResponse)

## Phases

### Phase 1: Expand Core Types

**Goal**: Expand `agent.HookResult` and define all hook input/output types in `pkg/hooks/`.

**Changes**:

1. **`pkg/agent/interfaces.go:35-41`** — Expand `HookResult` to include all SyncHookJSONOutput fields:
   ```go
   type HookResult struct {
       Decision      string // "allow"|"deny"|""
       Message       string
       Continue      *bool
       SystemMessage string
       // New fields from spec:
       SuppressOutput *bool
       StopReason     string
       Reason         string
       HookSpecificOutput any // typed per-event output
   }
   ```

2. **`pkg/hooks/types.go`** (new) — All typed input/output structs from the spec:
   - `BaseHookInput` (SessionID, TranscriptPath, CWD, PermissionMode)
   - 15 per-event input structs (PreToolUseHookInput, PostToolUseHookInput, etc.)
   - `SyncHookJSONOutput`, `AsyncHookJSONOutput`
   - 8 event-specific output structs (PreToolUseSpecificOutput, etc.)
   - `HookJSONOutput` union type (Sync + Async)
   - `HookCallback` function type: `func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error)`

3. **`pkg/hooks/shell.go`** (new) — `ShellHookCallback` type:
   - Wraps a shell command string
   - On invocation: marshals input → JSON → stdin, executes command, reads stdout → unmarshals JSON → HookJSONOutput
   - Handles stderr streaming
   - Respects context cancellation

**Success Criteria**:
- Automated:
  - [x] `go vet ./pkg/hooks/...` passes
  - [x] `go vet ./pkg/agent/...` passes
  - [x] `go build ./...` passes (existing code still compiles with expanded HookResult)

### Phase 2: Implement Runner

**Goal**: Build the `Runner` struct that manages hook registration, matcher filtering, timeout enforcement, and async support.

**Changes**:

1. **`pkg/hooks/runner.go`** (new) — Core Runner implementation:
   ```go
   type Runner struct {
       hooks map[types.HookEvent][]CallbackMatcher
   }
   ```
   - `CallbackMatcher` wraps `types.HookCallbackMatcher` with resolved callbacks (both Go funcs and shell commands)
   - `NewRunner(config RunnerConfig) *Runner`
   - `Fire(ctx, event, input any) → ([]agent.HookResult, error)` — implements `agent.HookRunner`
   - Matcher filtering via `matchToolName(pattern, input)` — glob/exact match on tool_name field
   - Timeout enforcement: `context.WithTimeout` per matcher when Timeout > 0
   - Execute hooks sequentially within a matcher, stop on `Continue=false`
   - Convert `HookJSONOutput` → `agent.HookResult`

2. **`pkg/hooks/matcher.go`** (new) — Tool name matching logic:
   - `matchToolName(pattern string, input any) bool`
   - Supports exact match ("Bash"), glob match ("mcp__*"), empty matcher (matches all)
   - Uses `filepath.Match` for glob patterns

3. **`pkg/hooks/async.go`** (new) — Async hook handling:
   - When a hook returns `AsyncHookJSONOutput{Async: true}`, start a background goroutine
   - Poll/wait up to `AsyncTimeout` seconds for completion
   - Return the async result or timeout error
   - For shell hooks: the command keeps running; we read stdout when it finishes

4. **`pkg/hooks/runner_test.go`** (new) — Comprehensive tests:
   - Go callback registration and execution
   - Shell command hook execution (mock via test script)
   - Matcher filtering (exact, glob, empty)
   - Timeout enforcement (hook exceeds timeout → cancelled)
   - Continue=false stops processing
   - Multiple matchers, multiple hooks per matcher
   - Error isolation (one hook error doesn't crash others)
   - Async hook with timeout
   - Event-specific input/output round-trip

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/hooks/... -race` passes (26 tests)
  - [x] `go vet ./pkg/hooks/...` passes
  - [x] All existing tests still pass: `go test ./... -race`

### Phase 3: Loop Integration

**Goal**: Update the agentic loop and tool execution to process hook results instead of ignoring them.

**Changes**:

1. **`pkg/agent/tools.go:88-93`** — PreToolUse hook result processing:
   - Process results: check for permission decisions (allow/deny) from `HookSpecificOutput`
   - If a hook denies, skip execution and return error result
   - If a hook provides `UpdatedInput`, use that for tool execution
   - Collect `AdditionalContext` for injection

2. **`pkg/agent/tools.go:106-125`** — PostToolUse / PostToolUseFailure:
   - Collect `AdditionalContext` from results
   - Check `SuppressOutput` flag
   - Process `UpdatedMCPToolOutput` for MCP tools

3. **`pkg/agent/loop.go:152-155`** — Stop hook result processing:
   - After firing Stop hook, check results for `Continue=true`
   - If any result has `Continue=true`, don't exit — continue the loop iteration
   - Inject any `SystemMessage` from results into next system message

4. **`pkg/agent/loop.go:44-47`** — SessionStart hook result processing:
   - Collect `AdditionalContext` from results
   - Inject into system prompt for next LLM call

5. **`pkg/agent/loop.go`** — Add context injection mechanism:
   - New field in `LoopState`: `PendingAdditionalContext []string`
   - Append to system prompt on next LLM call
   - Clear after consumption

6. **`pkg/agent/loop_test.go`** — Update existing tests + add new:
   - Test Stop hook with continue=true (loop continues)
   - Test PreToolUse hook deny (tool skipped)
   - Test PreToolUse hook with updatedInput
   - Test additionalContext injection

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/agent/... -race` passes (all 24 tests: 18 existing + 6 new)
  - [x] `go test ./pkg/hooks/... -race` passes (26 tests)
  - [x] `go test ./pkg/permission/... -race` passes (unchanged, verified)
  - [x] `go vet ./...` passes

### Phase 4: SDK Message Emission

**Goal**: Emit HookStarted/HookProgress/HookResponse messages during hook execution for observability.

**Changes**:

1. **`pkg/hooks/runner.go`** — Add optional `chan<- types.SDKMessage` to Runner for emission:
   - Emit `HookStartedMessage` when a hook begins
   - Emit `HookProgressMessage` for shell hooks (stream stdout/stderr)
   - Emit `HookResponseMessage` when a hook completes

2. **`pkg/hooks/runner.go`** — RunnerConfig adds:
   ```go
   type RunnerConfig struct {
       Hooks       map[types.HookEvent][]CallbackMatcher
       EmitChannel chan<- types.SDKMessage // optional
       SessionID   string
       CWD         string
   }
   ```

3. **`pkg/agent/loop.go`** — Pass the emit channel to hooks Runner if available

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/hooks/... -race` passes (29 tests including emission)
  - [x] `go test ./... -race` all pass

## Design Decisions

### Import Graph (no cycles)
```
pkg/types/       (no internal deps)
    ↓
pkg/agent/       (imports types, llm, tools)
    ↓
pkg/hooks/       (imports agent, types) — implements agent.HookRunner
pkg/permission/  (imports agent, types, tools) — uses agent.HookRunner
```

### Go Callbacks vs Shell Commands
- `HookCallback` is a Go function type for programmatic hooks
- `ShellHookCallback` wraps a command string; marshals JSON to stdin, reads JSON from stdout
- Both implement the same callback signature
- Registration: RunnerConfig accepts both types via a unified `CallbackMatcher` struct

### Input Typing
- The `Fire(ctx, event, input any)` interface stays as `any` to maintain backward compatibility
- Inside the Runner, we type-assert input to the expected per-event struct
- For callers that pass `map[string]any` (current pattern), Runner converts to typed structs internally
- Shell hooks always receive the full typed struct as JSON

### Result Processing Priority
- For PreToolUse: first "deny" wins, then first "allow", else "ask"
- For Stop: any `continue=true` means continue
- `Continue=false` stops processing further hooks in the current matcher group

## Out of Scope

- Configuration file loading (reading `.claude/settings.json` hooks) — that's the transport/CLI layer
- Hook registration UI / interactive management
- Persistent hook state across sessions
- The `TeammateIdle` and `TaskCompleted` hook firing points (depend on Spec 08 subagent manager)
- `UserPromptSubmit` hook firing point (depends on transport layer user input)
- `Notification` hook firing point (depends on notification system)
- `Setup` hook firing point (depends on initialization system)
- `PreCompact` hook firing point (depends on Spec 09 context compaction)

## Open Questions

*(none — all resolved)*

## References

- Spec: `thoughts/specs/07-HOOK-SYSTEM.md`
- Agent interfaces: `pkg/agent/interfaces.go`
- Existing loop hooks: `pkg/agent/loop.go:44-47,153-155,204-207`
- Existing tool hooks: `pkg/agent/tools.go:88-125`
- Permission hook integration: `pkg/permission/checker.go:112-183`
- Hook message types: `pkg/types/message_hooks.go`
- HookEvent constants: `pkg/types/options.go:131-157`
- Similar package pattern: `pkg/permission/` (implements agent.PermissionChecker)
