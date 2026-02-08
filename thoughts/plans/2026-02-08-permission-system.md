# Permission System (Spec 06) Implementation Plan

## Overview
Implement the full permission system that gates tool execution through a layered check stack: **mode → disabled → allowed → rules → hook → callback → mode default**. This replaces the current `AllowAllChecker` stub with a real `pkg/permission/` package.

## Current State
- **Interface**: `PermissionChecker` in `pkg/agent/interfaces.go:16-18` — simple `Check(ctx, toolName, input) → (PermissionResult{Allowed, DenyMessage, UpdatedInput}, error)`
- **Stub**: `AllowAllChecker` in `pkg/agent/stubs.go:20-24` — always allows
- **Integration point**: `pkg/agent/tools.go:49-71` — permission check before each tool execution
- **Tool classification**: `SideEffectType` enum in `pkg/tools/tool.go:6-15` — 6 levels
- **Registry lists**: `pkg/tools/registry.go:12-13` — `allowed` and `disabled` maps already exist
- **Types**: `PermissionMode` (6 modes), `PermissionUpdate`, `PermissionRuleValue`, `CanUseToolFunc` all in `pkg/types/`
- **Hook event**: `HookEventPermissionRequest` defined in `pkg/types/options.go:146`
- **Test**: 1 denial test in `pkg/agent/loop_test.go:514-546`

## Desired End State
- New `pkg/permission/` package with a `Checker` struct implementing the full spec check flow
- Updated `PermissionResult` interface with `Behavior`, `UpdatedPermissions`, `Interrupt`, `ToolUseID`
- Tool risk classification mapping `SideEffectType` → `ToolRiskLevel` for mode-based defaults
- Permission rule matching (substring/glob on tool input fields)
- Session-scoped rule accumulation (rules added during session via `ApplyUpdate`)
- Hook integration point (wired but uses `NoOpHookRunner` until Spec 07)
- `CanUseToolFunc` callback support
- `UserPrompter` interface for interactive "ask" behavior
- `bypassPermissions` mode gated behind `AllowDangerouslySkipPermissions` flag
- Agent loop updated to use richer `PermissionResult`

## Phases

### Phase 1: Core Types & Updated Interface
**Goal**: Establish the `pkg/permission/` package with core types and update the `PermissionChecker` interface in `pkg/agent/`.

**Changes**:
- Create `pkg/permission/types.go` — Define package-local types:
  - `PermissionBehavior` (allow/deny/ask)
  - `ToolRiskLevel` (None/Low/Medium/High/Critical) with mapping from `tools.SideEffectType`
  - `PermissionRule` struct (ToolName, RuleContent, Behavior, Source)
  - `CheckerConfig` struct (all Checker constructor params)
- Update `pkg/agent/interfaces.go:20-25` — Expand `PermissionResult`:
  ```go
  type PermissionResult struct {
      Behavior           string             // "allow"|"deny"|"ask"
      UpdatedInput       map[string]any     // nil if unchanged
      UpdatedPermissions []types.PermissionUpdate // rule changes to persist
      Message            string             // deny reason
      Interrupt          bool               // stop the loop entirely
      ToolUseID          string             // for correlation
  }
  ```
- Update `pkg/agent/tools.go:49-71` — Adapt to new `PermissionResult.Behavior` field instead of `Allowed bool`. Handle `Interrupt` flag (return early from `executeTools`).
- Update `pkg/agent/stubs.go:20-24` — `AllowAllChecker` returns `PermissionResult{Behavior: "allow"}`
- Update `pkg/agent/loop_test.go:542-546` — `denyAllChecker` returns `PermissionResult{Behavior: "deny", Message: "..."}`

**Success Criteria**:
- Automated:
  - [x] `go vet ./pkg/permission/...` clean
  - [x] `go vet ./pkg/agent/...` clean
  - [x] `go test ./pkg/agent/... -race` — all 16 existing tests pass with updated interface
  - [x] `go test ./pkg/tools/... -race` — all ~130 existing tests pass

### Phase 2: Checker Implementation — Mode & Lists
**Goal**: Implement the first 3 layers of the check flow: mode check, disabled check, allowed check.

**Changes**:
- Create `pkg/permission/checker.go` — `Checker` struct with:
  - `NewChecker(config CheckerConfig) *Checker` constructor
  - `Check(ctx, toolName, input) → (agent.PermissionResult, error)` — implements the full check flow
  - First 3 layers: mode → disabled → allowed
  - `SetMode(mode)` method
  - `Mode() PermissionMode` getter
- Create `pkg/permission/risk.go` — `DefaultBehaviorForTool(mode, sideEffect) → PermissionBehavior`
  - Maps `SideEffectType` → `ToolRiskLevel`
  - Applies mode behavior matrix from spec section 6
- Create `pkg/permission/checker_test.go` — Tests for:
  - `bypassPermissions` mode allows everything (with safety flag)
  - `bypassPermissions` without safety flag returns error
  - `plan` mode denies all tools
  - `delegate` mode denies all except Agent tool
  - `default` mode: auto-allows read tools, asks for write/bash
  - `acceptEdits` mode: auto-allows read + write tools, asks for bash
  - `dontAsk` mode: denies unless pre-approved
  - Disabled tools always denied regardless of mode
  - Allowed tools always allowed (except plan/delegate modes)
- Create `pkg/permission/risk_test.go` — Tests for risk mapping and mode defaults

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/permission/... -race` — all new tests pass (24 tests)
  - [x] Mode behavior matrix fully covered by table-driven tests

### Phase 3: Permission Rules
**Goal**: Implement rule matching and session-scoped rule accumulation.

**Changes**:
- Create `pkg/permission/rules.go`:
  - `PermissionRule.Matches(toolName, input) bool` — exact tool name match + substring/glob match on input
  - `matchRuleContent(ruleContent, toolName, input) bool` — field-specific matching:
    - Bash: match against `command` input field
    - FileWrite/FileEdit: match against `file_path` input field
    - Glob/Grep: match against `pattern` or `path` input field
    - Generic: match against any string input field value
  - Glob matching uses `doublestar.Match` (already a dependency)
- Add to `pkg/permission/checker.go`:
  - `ApplyUpdate(update types.PermissionUpdate) error` — handles addRules, replaceRules, removeRules, setMode, addDirectories, removeDirectories
  - Rules stored in two slices: `configRules` (from settings) and `sessionRules` (accumulated)
  - Rules checked in order: configRules first, then sessionRules
- Create `pkg/permission/rules_test.go` — Tests for:
  - Exact tool name match
  - Empty ruleContent matches all invocations of that tool
  - Substring match on Bash command
  - Glob match on file paths (e.g., `"/src/**"`)
  - No match when tool name differs
  - `ApplyUpdate` addRules/removeRules/replaceRules
  - Session rules persist across checks
  - Config rules vs session rules ordering

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/permission/... -race` — all rule tests pass (44 tests total)
  - [x] Rule matching handles edge cases (empty input, missing fields, nil input)

### Phase 4: Hook & Callback Integration
**Goal**: Wire in the hook and callback layers (layers 5 and 6 of the check flow).

**Changes**:
- Update `pkg/permission/checker.go`:
  - Add `hookRunner agent.HookRunner` field to Checker
  - Add `canUseTool types.CanUseToolFunc` field to Checker
  - In `Check()`, after rules and before mode default:
    1. If result is "ask" and hookRunner is not nil, fire `HookEventPermissionRequest`
    2. If hook returns allow/deny, use that result
    3. If result is still "ask" and canUseTool is not nil, call callback
    4. If callback returns a result, use it
    5. Otherwise fall through to mode default
- Create `pkg/permission/prompter.go`:
  - `UserPrompter` interface: `PromptForPermission(toolName, input, suggestions) → (PermissionResult, error)`
  - `StubPrompter` that always denies (for headless/testing)
  - In `Check()`, if final result is "ask" and userPrompter is not nil, call it
  - If userPrompter is nil and result is "ask", deny (headless default)
- Update `pkg/permission/checker_test.go` — Tests for:
  - Hook returning allow short-circuits callback
  - Hook returning deny short-circuits callback
  - Hook returning continue falls through to callback
  - Callback allow/deny honored
  - No hook/no callback falls to mode default
  - StubPrompter denies "ask" results
  - UserPrompter approval persists as session rule

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/permission/... -race` — all hook/callback tests pass (55 tests total)
  - [x] Full check flow exercised in integration test (mode → disabled → allowed → rules → hook → callback → default)

### Phase 5: Agent Loop Integration
**Goal**: Wire the real `permission.Checker` into the agent loop, replacing `AllowAllChecker`.

**Changes**:
- Update `pkg/agent/tools.go:49-71`:
  - Handle `Behavior: "ask"` — this shouldn't reach here if Checker is correct, but safety check
  - Handle `Interrupt: true` — stop processing remaining tool blocks, signal loop to stop
  - Pass `UpdatedPermissions` back via SDK message emission if present
- Update `pkg/agent/agent.go`:
  - Add `WithAllowedTools(names ...string) Option`
  - Add `WithDisallowedTools(names ...string) Option`
  - Add `WithCanUseTool(fn types.CanUseToolFunc) Option`
  - Add `WithAllowDangerouslySkipPermissions(allow bool) Option`
- Update `pkg/agent/config.go`:
  - Add `AllowedTools []string` and `DisallowedTools []string` fields
  - Add `AllowDangerouslySkipPermissions bool` field
  - Add `CanUseTool types.CanUseToolFunc` field
- Create integration test in `pkg/agent/loop_test.go`:
  - Test with real `permission.Checker` (not AllowAllChecker)
  - Test mode switching mid-session (via `SetMode`)
  - Test `Interrupt` stops the loop
  - Test `UpdatedInput` flows through to tool execution

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/agent/... -race` — all 18 tests pass (16 existing + 2 new)
  - [x] `go test ./pkg/permission/... -race` — all 55 tests pass
  - [x] `go test ./... -race` — full suite passes (all packages green)
  - [x] `go vet ./...` — clean

## Out of Scope
- Full hook system implementation (Spec 07) — we use `NoOpHookRunner` for now
- Persistent storage of permission rules to disk (session rules are in-memory only)
- LLM-based semantic rule matching (we use substring/glob)
- Real `UserPrompter` implementation (requires terminal UI / transport layer)
- Settings file loading (user/project/local settings sources)
- Directory-scoped rules (`addDirectories`/`removeDirectories` types parsed but not enforced)

## Open Questions
None — all design decisions resolved.

## References
- Spec: `thoughts/specs/06-PERMISSION-SYSTEM.md`
- Hook spec: `thoughts/specs/07-HOOK-SYSTEM.md`
- Current interface: `pkg/agent/interfaces.go:16-25`
- Current stub: `pkg/agent/stubs.go:20-24`
- Current integration: `pkg/agent/tools.go:49-71`
- Tool classification: `pkg/tools/tool.go:6-15`
- Registry lists: `pkg/tools/registry.go:12-13, 61-69`
- Permission types: `pkg/types/options.go:38-183`
- Permission modes: `pkg/types/message_system.go:38-47`
- Existing test: `pkg/agent/loop_test.go:514-546`
