# Spec Gap Closure Implementation Plan (Excluding Spec 12)

## Overview
Close all remaining implementation gaps identified in the spec audit (`thoughts/reports/2026-02-08-spec-implementation-audit.md`), excluding Spec 12 (Transport Layer) which will be handled in a separate session.

## Current State
- 12 packages implemented, ~750+ tests passing with `-race`
- Gaps range from 5% to 30% across specs 03-11
- Most gaps are small wiring issues, not architectural

## Desired End State
- All specs 03-11 at 100% completeness
- All existing tests still pass
- New tests for every change
- `go vet ./...` clean

---

## Phase 1: Quick Fixes (Small, isolated changes)

**Goal**: Close all "Small effort" gaps that are 1-20 line changes.

### 1.1 Subagent maxTurns default (Spec 08)
- `pkg/subagent/manager.go:129` — Change `maxTurns := 100` → `maxTurns := 50`
- **Test**: Update any test asserting default of 100

### 1.2 Permission bypass override (Spec 08)
- `pkg/subagent/manager.go` — In `resolvePermissionMode()`, add early return if parent mode is `bypassPermissions`
- **Test**: Add test spawning subagent when parent uses bypassPermissions

### 1.3 Task restriction enforcement (Spec 08)
- `pkg/subagent/manager.go` — In Spawn(), after `parseTaskRestriction()`, validate spawned type against restriction
- Currently parsed at line 111 but restriction is discarded at line 480
- **Test**: Spawn with `Task(Explore, Plan)` restriction, verify other types rejected

### 1.4 Memory tools auto-enable (Spec 08)
- `pkg/subagent/manager.go` — After line 123, when `def.Memory != ""`, ensure `FileRead`, `FileWrite`, `FileEdit` in tool list
- Use existing `ensureTools()` helper at `pkg/subagent/tools.go:105`
- **Test**: Spawn with memory enabled but empty tools list, verify R/W/E tools present

### 1.5 MustCompact() method (Spec 09)
- `pkg/context/compactor.go` — Add `MustCompact(budget agent.TokenBudget) bool` that returns true when utilization > `criticalPct` (0.95)
- Wire into loop: check `MustCompact()` on `max_tokens` stop reason
- **Test**: Test MustCompact at 94% (false) and 96% (true)

### 1.6 Custom instructions from PreCompact hook (Spec 09)
- `pkg/context/compactor.go:81-85` — Capture hook result, extract `custom_instructions` field from hook output
- Pass to `generateSummary()` at line 98 (currently passes `nil`)
- **Test**: Fire PreCompact hook returning custom instructions, verify they appear in compaction prompt

### 1.7 Session metadata fields (Spec 11)
- `pkg/session/store.go` (or metadata struct) — Add `ProjectHash`, `ExitReason`, `AgentName` fields to `SessionMetadata`
- Populate in loop finalization: `ProjectHash` from config, `ExitReason` from LoopState, `AgentName` from config
- **Test**: Verify metadata saved with all fields populated

### 1.8 MCP annotation → permission integration (Spec 10)
- `pkg/permission/risk.go:42-50` — In `ToolRisk()` for MCP tools (`mcp__*`), check annotations:
  - `readOnly: true` → `RiskLow`
  - `destructive: true` → `RiskCritical`
  - `openWorld: true` → `RiskHigh` (already default)
- Need: Pass annotation data through. Options: registry lookup or tool metadata in permission context
- **Test**: MCP tool with `readOnly` annotation gets RiskLow, `destructive` gets RiskCritical

### 1.9 Delegate mode tool list (Spec 08c)
- `pkg/teams/delegate.go:7-15` — Update `DelegateModeTools` to match spec
- Add: any missing tools from spec (verify actual registered tool names)
- Remove: `TaskGet` if not in spec
- **Test**: Update delegate_test.go expectations

**Success Criteria (Phase 1)**:
- Automated:
  - [x] `go test ./pkg/subagent/... -race` passes
  - [x] `go test ./pkg/context/... -race` passes
  - [x] `go test ./pkg/session/... -race` passes
  - [x] `go test ./pkg/permission/... -race` passes
  - [x] `go test ./pkg/teams/... -race` passes
  - [x] `go vet ./...` clean

---

## Phase 2: AgentConfig & Loop Control (Spec 03)

**Goal**: Add missing AgentConfig fields and Query control methods.

### 2.1 AgentConfig missing fields
- `pkg/agent/config.go` — Add fields:
  - `FallbackModel string` — for automatic model fallback
  - `MaxThinkingTkn *int` — thinking token limit (wire to LLM request)
  - `AdditionalDirs []string` — extra directories for prompt assembly
  - `Betas []string` — beta features list
  - `DebugFile string` — debug output file path
- These are config fields only; actual usage comes with transport layer
- **Test**: Verify config serialization/deserialization with new fields

### 2.2 Query control methods
- `pkg/agent/loop.go` — Add methods on the loop (or config):
  - `SetPermissionMode(mode)` — update permission checker mode at runtime
  - `SetModel(model)` — call `llmClient.SetModel()` (already exists on client)
  - `SetMaxThinkingTokens(tokens *int)` — update config field
  - `Close()` — cancel context, clean up resources
- These will be fully useful with transport layer, but the methods should exist now
- **Test**: Un-skip `TestLoop_DynamicControlSetPermissionMode` and `TestLoop_DynamicControlSetModel` at `loop_test.go:1948-1954`

### 2.3 stop_sequence handling
- `pkg/agent/loop.go` — Add explicit `stop_sequence` case in stop reason switch (currently falls through to end_turn)
- **Test**: Mock LLM response with stop_sequence, verify correct handling

### 2.4 Truncated tool_use on max_tokens
- `pkg/agent/loop.go` — Before executing tool_use blocks from a `max_tokens` response, validate JSON completeness
- If tool_use block has incomplete JSON arguments, discard it and trigger compaction instead
- **Test**: Response with max_tokens + incomplete tool_use JSON → compaction triggered

**Success Criteria (Phase 2)**:
- Automated:
  - [x] `go test ./pkg/agent/... -race` passes (including previously-skipped tests)
  - [x] `go vet ./...` clean

---

## Phase 3: System Prompt Assembly (Spec 05)

**Goal**: Wire missing prompt sections and fix subagent prompt assembly.

### 3.1 Add environment details to main prompt
- `pkg/prompt/assembler.go:28` — Add `formatEnvironmentDetails(config)` to main assembly (currently only in subagent prompts)
- **Test**: Verify assembled prompt contains OS, CWD, date info

### 3.2 Wire tool documentation loading
- `pkg/prompt/assembler.go` — After existing sections, add conditional loading:
  - `toolEnabled(config, "Glob")` → `loadToolPrompt("tool-description-glob.md")`
  - `toolEnabled(config, "Grep")` → `loadToolPrompt("tool-description-grep.md")`
  - `toolEnabled(config, "FileEdit")` → `loadToolPrompt("tool-description-edit.md")`
- Files already exist in `prompts/tools/`
- **Test**: Assembled prompt with Glob tool includes glob documentation

### 3.3 Create missing "always" prompt files
- Create in `pkg/prompt/prompts/system/`:
  - `system-prompt-file-read-limits.md` — Read tool's 2000-line default limit
  - `system-prompt-large-file-handling.md` — offset/limit for large files
  - `system-prompt-use-tools-to-verify.md` — verify assumptions with tools
- Content sourced from Piebald-AI embedded prompts (already extracted)
- Wire into assembler as "always" sections
- **Test**: Assembled prompt contains "file read limits" content

### 3.4 Subagent prompt: skills injection
- `pkg/prompt/subagent.go:14-25` — In `AssembleSubagentPrompt()`:
  - After custom prompt, iterate `agentDef.Skills`
  - Call `loadSkillPrompt(skillName)` for each (function exists in embed.go:80)
  - Append non-empty content to parts
- **Test**: Subagent with skills=["skill-debugging.md"] includes debugging content

### 3.5 Subagent prompt: critical system reminder
- `pkg/prompt/subagent.go` — After skills, add:
  - If `agentDef.CriticalReminder != ""`, prepend `"CRITICAL REMINDER: " + content`
- **Test**: Verify critical reminder appears in assembled subagent prompt

**Success Criteria (Phase 3)**:
- Automated:
  - [x] `go test ./pkg/prompt/... -race` passes
  - [x] `go vet ./...` clean

---

## Phase 4: Hook System & Async (Spec 07)

**Goal**: Wire async hook execution and hook progress emission.

### 4.1 Async hook execution
- `pkg/hooks/runner.go` — In `executeCallbacks()` and `executeShellCommands()`:
  - Parse hook stdout; if `{"async": true}` detected → call `executeAsync()`
  - `executeAsync()` already exists at `async.go:14-26`
  - Async hooks run in goroutine with timeout from `AsyncTimeout` field
- **Test**: Shell hook returning `{"async": true, "asyncTimeout": 5}` → async execution

### 4.2 Hook progress emission
- `pkg/hooks/runner.go` — During shell command execution:
  - Stream stdout/stderr lines as `HookProgressMessage` on emit channel
  - `HookProgressMessage` type already exists at `types/message_hooks.go:16`
- **Test**: Shell hook emitting progress → HookProgressMessage on emit channel

### 4.3 Permission hook interrupt flag
- `pkg/permission/checker.go` — When PermissionRequest hook returns `decision.interrupt: true`:
  - Set `PermissionResult.Interrupt = true` (field already exists)
  - This should already propagate to loop (loop checks Interrupt flag)
- Verify the PermissionRequestHookOutput type in hooks includes `Interrupt` field
- **Test**: Hook returns deny+interrupt → PermissionResult has Interrupt=true

**Success Criteria (Phase 4)**:
- Automated:
  - [x] `go test ./pkg/hooks/... -race` passes
  - [x] `go test ./pkg/permission/... -race` passes
  - [x] `go vet ./...` clean

---

## Phase 5: Tool Gaps (Spec 04)

**Goal**: Add PDF reading to FileRead and WebFetch summarizer.

### 5.1 PDF text extraction in FileRead
- `pkg/tools/fileread.go` — Add `pages` parameter to schema and execution:
  - Detect `.pdf` extension
  - Add Go PDF library dependency (e.g., `github.com/ledongthuc/pdf` or `github.com/pdfcpu/pdfcpu`)
  - Parse page range string ("1-5", "3", "10-20")
  - Extract text from specified pages (max 20)
  - Return with line numbers like regular text
- **Test**: Read test PDF file with pages param, verify text extraction

### 5.2 WebFetch LLM summarizer
- `pkg/tools/webfetch.go` — Add `Summarizer` interface or direct LLM call:
  - Option A: Add `LLMClient llm.Client` field, call LLM directly with prompt + content
  - Option B: Add `SubagentSpawner` field, spawn subagent for summarization
  - Prefer Option A (simpler, no subagent overhead for a single LLM call)
  - When `LLMClient` is nil, fall back to current behavior (return raw content)
- **Test**: Mock LLM client, verify prompt + content sent for summarization

**Success Criteria (Phase 5)**:
- Automated:
  - [x] `go test ./pkg/tools/... -race` passes
  - [x] `go vet ./...` clean

---

## Phase 6: Subagent Advanced Features (Spec 08/08b)

**Goal**: Background agent output files, transcript persistence, permission handling.

### 6.1 Background agent output files
- `pkg/subagent/manager.go` — For background agents:
  - Create output file at `{outputDir}/{agentID}.output`
  - Write accumulated output incrementally
  - Return output file path in spawn result message
- **Test**: Spawn background agent, verify output file created and written

### 6.2 Background agent permissions
- `pkg/subagent/manager.go:462-471` — Replace AllowAllChecker with pre-approved checker:
  - Background agents auto-deny anything not pre-approved
  - AskUserQuestion tool should fail in background mode
  - MCP tools filtered out for background agents
- **Test**: Background agent denied on non-pre-approved tool, AskUserQuestion fails

### 6.3 Subagent transcript persistence
- `pkg/subagent/manager.go` — Set `PersistSession: true` in subagent config when parent has persistence:
  - Add `TranscriptPath` to subagent config: `{transcriptDir}/agent-{agentID}.jsonl`
  - Wire `pkg/session` store into subagent config
- **Test**: Subagent with persistence creates transcript file

**Success Criteria (Phase 6)**:
- Automated:
  - [x] `go test ./pkg/subagent/... -race` passes
  - [x] `go vet ./...` clean

---

## Phase 7: Teams Adapter & Hooks (Spec 08c)

**Goal**: Create TeamCoordinator adapter, wire team hooks.

### 7.1 TeamCoordinator adapter
- Create `pkg/teams/coordinator_adapter.go`:
  - `TeamManagerAdapter` struct wrapping `*TeamManager`
  - Implement all 8 methods of `tools.TeamCoordinator` interface
  - Map return types: `*Team` → `tools.TeamInfo`, `*TeamMember` → `tools.TeamMemberInfo`
  - Bridge methods: `SendMessage()` → `team.Mailbox.Send()`, `Broadcast()` → `team.Mailbox.Broadcast()`
  - `GetTeamName()` → return `team.Name`, `GetMemberNames()` → return member names
- **Test**: Verify adapter satisfies `tools.TeamCoordinator` interface; test each method

### 7.2 Wire team hooks
- `pkg/teams/task.go` (SharedTaskList.Complete or equivalent):
  - Before marking task complete, call `tm.fireTaskCompleted(ctx, taskID, subject, agentName)`
  - If `ShouldPreventCompletion()` returns true, abort and return feedback
- `pkg/teams/teammate.go` (TeammateRuntime idle detection):
  - Before going idle, call `tm.fireTeammateIdle(ctx, agentName)`
  - If `ShouldKeepWorking()` returns true, continue processing
- **Test**: Task completion with hook preventing completion; idle with hook keeping working

### 7.3 Plan approval message type
- `pkg/tools/sendmessage.go:25` — Add `"plan_approval_request"` to message type enum
- Plan enforcement logic deferred to transport layer session
- **Test**: SendMessage with plan_approval_request type succeeds

**Success Criteria (Phase 7)**:
- Automated:
  - [x] `go test ./pkg/teams/... -race` passes
  - [x] `go test ./pkg/tools/... -race` passes
  - [x] `go vet ./...` clean

---

## Phase 8: Validation Fix-ups (5 remaining gaps)

**Goal**: Close the 5 partially-implemented items identified during plan validation.

### 8.1 Wire MCP annotations into Checker.Check() (Spec 10 × Spec 06)

The `ToolRiskWithAnnotations()` function exists in `risk.go` but `Checker.Check()` never passes annotations.

**Approach**: Add `MCPAnnotations *MCPAnnotations` field to `PermissionRequest` (or add a `ToolRegistry` to `Checker`). Simplest: pass annotations through the existing Check signature.

- `pkg/permission/checker.go` — Add `Annotations *MCPAnnotations` field to Checker struct or accept via a new `CheckWithAnnotations` method
  - Simpler: Add a `ToolAnnotationLookup func(string) *MCPAnnotations` field to Checker
  - In `Check()`, call `ToolAnnotationLookup(toolName)` to get annotations
  - Pass annotations to `DefaultBehaviorForTool(mode, toolName, annotations)`
- `pkg/permission/risk.go:75-76` — Change `DefaultBehaviorForTool(mode, toolName)` signature to `DefaultBehaviorForTool(mode, toolName, annotations *MCPAnnotations)`
  - Replace `ToolRisk(toolName)` call with `ToolRiskWithAnnotations(toolName, annotations)`
- `pkg/agent/tools.go` or wherever Checker is constructed — Wire annotation lookup via registry:
  ```go
  checker.ToolAnnotationLookup = func(name string) *permission.MCPAnnotations {
      tool, ok := registry.Get(name)
      if !ok { return nil }
      if mcp, ok := tool.(*tools.MCPTool); ok {
          ann := mcp.Annotations()
          if ann == nil { return nil }
          return &permission.MCPAnnotations{
              ReadOnly: ann.ReadOnlyHint, Destructive: ann.DestructiveHint, OpenWorld: ann.OpenWorldHint,
          }
      }
      return nil
  }
  ```
- **Test**: Update `pkg/permission/checker_test.go` — Checker with annotation lookup returning `{ReadOnly: true}` → MCP tool gets `RiskLow` behavior; `{Destructive: true}` → `RiskCritical`
- **Test**: Update `pkg/permission/risk_test.go` — `DefaultBehaviorForTool` with annotations parameter

### 8.2 Wire MaxThinkingTokens into LLM request (Spec 03)

`LoopState.MaxThinkingTokens` and `AgentConfig.MaxThinkingTkns` exist but the `BuildCompletionRequest` call hardcodes `MaxThinkingTokens: 0`.

- `pkg/agent/loop.go:133-143` — Before the `BuildCompletionRequest` call, resolve the value:
  ```go
  maxThinkingTokens := 0
  if state.MaxThinkingTokens > 0 {
      maxThinkingTokens = state.MaxThinkingTokens
  } else if config.MaxThinkingTkns != nil {
      maxThinkingTokens = *config.MaxThinkingTkns
  }
  ```
  Then use `MaxThinkingTokens: maxThinkingTokens` in the ClientConfig
- **Test**: Add test in `loop_test.go` — set `config.MaxThinkingTkns = intPtr(1024)`, verify the LLM request includes `MaxThinkingTokens: 1024`
- **Test**: Add test — set via control command `SetMaxThinkingTokens(2048)`, verify next LLM call uses 2048

### 8.3 Add date and OS version to environment details (Spec 05)

`formatEnvironmentDetails()` outputs CWD, Platform, Shell but is missing date and OS version.

- `pkg/agent/config.go:47-49` — Add two fields:
  ```go
  OSVersion   string // e.g., "Darwin 25.2.0"
  CurrentDate string // e.g., "2026-02-09"
  ```
- `pkg/prompt/subagent.go` — Update `formatEnvironmentDetails()` to include:
  ```go
  if config.OSVersion != "" {
      lines = append(lines, fmt.Sprintf("- OS Version: %s", config.OSVersion))
  }
  if config.CurrentDate != "" {
      lines = append(lines, fmt.Sprintf("- Today's date: %s", config.CurrentDate))
  }
  ```
- **Test**: Update `pkg/prompt/assembler_test.go` — config with OSVersion + CurrentDate set → assembled prompt contains both

### 8.4 Remove non-existent task tools from DelegateModeTools (Spec 08c)

`DelegateModeTools` references TaskCreate/TaskUpdate/TaskList/TaskGet which don't exist as tools in `pkg/tools/`. The underlying `SharedTaskList` exists in teams but isn't exposed as tools yet.

**Approach**: Keep the tool names in the list (they represent the intended API) but add a comment noting they are forward declarations. The actual tool implementations will come when the teams feature is fully wired. Meanwhile, the `IsAllowed()` check in delegate mode already handles unknown tools gracefully (they just won't match any registered tool).

- `pkg/teams/delegate.go:7-16` — Add clarifying comment:
  ```go
  // DelegateModeTools lists the tools available in delegate (lead agent) mode.
  // TaskCreate/TaskUpdate/TaskList are forward-declared; implementations pending.
  var DelegateModeTools = []string{
      "TeamCreate",
      "SendMessage",
      "TeamDelete",
      "TaskCreate",
      "TaskUpdate",
      "TaskList",
      "AskUserQuestion",
  }
  ```
- Remove `TaskGet` (not in spec)
- `pkg/teams/delegate_test.go` — Update test expectations to remove TaskGet
- **Test**: Verify DelegateModeTools length is 7 (not 8)

### 8.5 Populate subagent transcript path in SubagentStop hook (Spec 08/08b)

The `SubagentStop` hook input has an `AgentTranscriptPath` field but it's never populated.

- `pkg/subagent/running.go` — Add `TranscriptPath string` field to `RunningAgent`
- `pkg/subagent/manager.go` — During spawn (around line 181-208), if `SessionStore != nil`:
  - Build transcript path: use `m.opts.TranscriptDir` + `agentID` or derive from session store
  - Store on `RunningAgent.TranscriptPath`
- `pkg/subagent/manager.go:411-421` — In SubagentStop hook firing, add:
  ```go
  AgentTranscriptPath: ra.TranscriptPath,
  ```
- `pkg/subagent/manager.go:26` — Add `TranscriptDir string` field to `ManagerOpts` if not present
- **Test**: Spawn subagent with SessionStore configured → SubagentStop hook receives non-empty AgentTranscriptPath

**Success Criteria (Phase 8)**:
- Automated:
  - [x] `go test ./pkg/permission/... -race` passes
  - [x] `go test ./pkg/agent/... -race` passes
  - [x] `go test ./pkg/prompt/... -race` passes
  - [x] `go test ./pkg/teams/... -race` passes
  - [x] `go test ./pkg/subagent/... -race` passes
  - [x] `go vet ./...` clean

---

## Out of Scope

- **Spec 12: Transport Layer** — separate session entirely
- **Spec 08c: Display modes** (tmux/in-process) — large effort, UI-dependent
- **Spec 08c: SessionHandle abstraction** — coupled with display modes
- **Spec 10: SDK transport** — uncommon use case
- **Spec 10: ClaudeAI proxy transport** — requires Anthropic API integration
- **Spec 10: needs-auth flow** — requires authentication handler design
- **Spec 03: Parallel tool execution** — marked "Future" in spec
- **Spec 06: Directory restrictions** — marked "out of scope per plan" in code
- **Spec 05: Custom slash commands** — requires slash command feature first
- **Spec 11: LoadTranscript()** — reader method only needed for transport

## Open Questions
*None — all resolved through research.*

## References
- Audit report: `thoughts/reports/2026-02-08-spec-implementation-audit.md`
- Specs: `thoughts/specs/01-LLM-CLIENT.md` through `thoughts/specs/12-TRANSPORT.md`
- Implementation: `pkg/{agent,tools,prompt,permission,hooks,context,mcp,session,subagent,teams}/`
