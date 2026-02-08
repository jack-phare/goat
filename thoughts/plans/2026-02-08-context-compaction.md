# Context Compaction (Spec 09) Implementation Plan

## Overview
Implement `pkg/context/` — a context window manager that prevents the conversation from exceeding the model's token limit. When utilization crosses 80%, the compactor summarizes older messages while preserving recent context and critical information. This implements the `ContextCompactor` interface (to be expanded) and integrates into the agentic loop for both proactive and reactive compaction.

## Current State
- `ContextCompactor` interface defined at `pkg/agent/interfaces.go:47-51` — currently takes only `(messages, model)`, needs expansion
- `NoOpCompactor` stub at `pkg/agent/stubs.go:33-42` — always returns false
- Loop integration at `pkg/agent/loop.go:169-179` — reactive only (on `max_tokens` stop reason)
- Token estimation at `pkg/prompt/tokens.go:11-13` — `len(text)/4` heuristic
- `CompactBoundaryMessage` + `CompactMetadata` types at `pkg/types/message_system.go:60-74`
- `HookEventPreCompact` at `pkg/types/options.go:145`
- `ToolUseSummaryMessage` at `pkg/types/message_misc.go:63-71`
- LLM client is streaming-only (`llm.Client.Complete` → `*Stream`)

## Desired End State
- `pkg/context/` package with a real `Compactor` implementing the expanded `ContextCompactor` interface
- Proactive compaction check before each LLM call (80% threshold)
- Reactive fallback on `max_tokens` stop reason (95% threshold / critical)
- Summary generation via LLM call (draining stream internally)
- `CompactBoundaryMessage` emitted on every compaction
- `PreCompact` hook fires before, `SessionStart(source=compact)` hook fires after
- Tool result pruning as a lighter-weight pre-step
- Beta 1M context window support
- Full test coverage with `-race`

---

## Phases

### Phase 1: Expand Interface + Types

**Goal**: Update the `ContextCompactor` interface and add supporting types.

**Changes**:

1. `pkg/agent/interfaces.go:47-51` — Expand `ContextCompactor` interface:
   ```go
   type ContextCompactor interface {
       ShouldCompact(budget TokenBudget) bool
       Compact(ctx context.Context, req CompactRequest) ([]llm.ChatMessage, error)
   }
   ```

2. `pkg/agent/interfaces.go` — Add `TokenBudget` and `CompactRequest` types:
   ```go
   type TokenBudget struct {
       ContextLimit     int     // model's total context window
       SystemPromptTkns int     // estimated system prompt tokens
       MaxOutputTkns    int     // reserved for output (default 16384)
       MessageTkns      int     // current message history tokens
   }

   type CompactRequest struct {
       Messages  []llm.ChatMessage
       Model     string
       Budget    TokenBudget
       Trigger   string          // "auto" | "manual"
       SessionID string
       EmitCh    chan<- types.SDKMessage
   }
   ```

3. `pkg/agent/stubs.go:33-42` — Update `NoOpCompactor` to match new interface:
   ```go
   func (n *NoOpCompactor) ShouldCompact(_ TokenBudget) bool { return false }
   func (n *NoOpCompactor) Compact(_ context.Context, req CompactRequest) ([]llm.ChatMessage, error) {
       return req.Messages, nil
   }
   ```

4. `pkg/agent/loop.go:169-179` — Update call sites to pass `TokenBudget` and `CompactRequest`

5. `pkg/agent/loop.go:63-68` — Add proactive compaction check before LLM call:
   ```go
   // After checkTermination, before BuildCompletionRequest:
   budget := calculateTokenBudget(config, state, systemPrompt)
   if config.Compactor.ShouldCompact(budget) {
       compacted, err := config.Compactor.Compact(ctx, CompactRequest{...})
       if err == nil {
           state.Messages = compacted
       }
   }
   ```

6. Update all test files that reference the old interface signature.

**Success Criteria**:
- Automated:
  - [x] `go build ./...` compiles cleanly
  - [x] `go test ./pkg/agent/... -race` passes (all 24 existing tests)
  - [x] `go vet ./...` clean

---

### Phase 2: Token Budget & Model Limits

**Goal**: Implement token budget calculation and model context limits.

**Changes**:

1. Create `pkg/context/budget.go`:
   - `ModelContextLimits` map — model string → context window size
   - `GetContextLimit(model string, betas []string) int` — returns effective context limit, with beta 1M support
   - `TokenBudget` utility methods: `IsOverflow() bool`, `UtilizationPct() float64`, `Available() int`
   - Note: The `TokenBudget` struct lives in `pkg/agent` (to avoid import cycles), but utility functions live here

2. Create `pkg/context/estimator.go`:
   - `TokenEstimator` interface: `Estimate(text string) int`, `EstimateMessages(messages []llm.ChatMessage) int`
   - `SimpleEstimator` struct implementing the `len(text)/4` heuristic (move logic from `pkg/prompt/tokens.go`)
   - Per-message overhead calculation (~4 tokens for role/separators)
   - `ContentString(msg llm.ChatMessage) string` helper to extract text from `any` content

3. Create `pkg/context/budget_test.go`:
   - Table-driven tests for model limits
   - Beta 1M context window tests
   - `UtilizationPct` and `IsOverflow` tests
   - Edge cases: unknown model, empty betas

4. Create `pkg/context/estimator_test.go`:
   - Table-driven tests for `SimpleEstimator`
   - Multi-message estimation tests
   - Empty/nil content edge cases
   - Content as string vs `[]ContentPart`

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/context/... -race` passes
  - [x] Beta 1M returns 1_000_000 for Sonnet with correct beta flag
  - [x] Token estimates within expected range for test corpus

---

### Phase 3: Core Compactor Implementation

**Goal**: Implement the `Compactor` struct with split point calculation and summary generation.

**Changes**:

1. Create `pkg/context/compactor.go`:
   - `CompactorConfig` struct:
     ```go
     type CompactorConfig struct {
         LLMClient         llm.Client
         HookRunner        agent.HookRunner
         Estimator         TokenEstimator     // default: SimpleEstimator
         SummaryModel      string             // default: haiku
         ThresholdPct      float64            // default: 0.80
         CriticalPct       float64            // default: 0.95
         PreserveRatio     float64            // default: 0.40
     }
     ```
   - `Compactor` struct implementing `agent.ContextCompactor`
   - `NewCompactor(config CompactorConfig) *Compactor` constructor with defaults
   - `ShouldCompact(budget agent.TokenBudget) bool` — checks `UtilizationPct > ThresholdPct`
   - `Compact(ctx, req) ([]llm.ChatMessage, error)`:
     1. Fire `PreCompact` hook (via `req.HookRunner` or stored runner)
     2. Calculate split point
     3. Generate summary via LLM call
     4. Build compacted message list: `[summaryMsg] + preserveZone`
     5. Emit `CompactBoundaryMessage`
     6. Fire `SessionStart(source=compact)` hook
     7. Return compacted messages
   - Compile-time verification: `var _ agent.ContextCompactor = (*Compactor)(nil)`

2. Create `pkg/context/split.go`:
   - `calculateSplitPoint(messages []llm.ChatMessage, preserveBudget int, estimator TokenEstimator) int`
   - Walk backward from end, accumulate token estimates until preserve budget exceeded
   - Ensure split doesn't break a tool_use/tool_result pair (adjust to keep pairs together)

3. Create `pkg/context/summary.go`:
   - `generateSummary(ctx context.Context, messages []llm.ChatMessage, client llm.Client, model string, customInstructions *string) (string, error)`
   - Build compaction prompt (based on Piebald compact utility prompt text)
   - Call `client.Complete()`, drain `Stream` to get full text
   - Return summary text
   - `drainStreamText(stream *llm.Stream) (string, error)` helper

4. Create `pkg/context/prune.go`:
   - `PruneOldToolResults(messages []llm.ChatMessage, preserveRecent int) []llm.ChatMessage`
   - Replace verbose tool outputs (>1000 chars) with truncated versions
   - This is a lighter-weight alternative that can be applied before full compaction

5. Create corresponding test files:
   - `pkg/context/compactor_test.go` — mock LLM client, verify split+summary+emission
   - `pkg/context/split_test.go` — table-driven split point tests
   - `pkg/context/summary_test.go` — mock LLM, verify prompt construction
   - `pkg/context/prune_test.go` — table-driven pruning tests

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/context/... -race` passes
  - [x] Split point preserves ~40% of context
  - [x] Split doesn't break tool_use/tool_result pairs
  - [x] Summary prompt includes key preservation instructions
  - [x] `CompactBoundaryMessage` emitted with correct trigger and pre_tokens
  - [x] `PreCompact` hook is fired
  - [x] `SessionStart(source=compact)` hook is fired
  - [x] Fallback to simple truncation if summary generation fails

---

### Phase 4: Loop Integration

**Goal**: Wire the compactor into the agentic loop for proactive + reactive compaction.

**Changes**:

1. `pkg/agent/loop.go` — Add proactive compaction check:
   - After `checkTermination` (line 65-68), before building the completion request
   - Calculate `TokenBudget` from config, state, systemPrompt
   - If `ShouldCompact(budget)` → call `Compact(ctx, CompactRequest{...})`
   - On success, replace `state.Messages`
   - On failure, log but continue (let the LLM call proceed)

2. `pkg/agent/loop.go:169-179` — Update reactive compaction:
   - Build `TokenBudget` and `CompactRequest` with trigger `"auto"` (critical)
   - Pass budget to `ShouldCompact` instead of old params
   - Pass `CompactRequest` to `Compact` instead of just messages

3. `pkg/agent/config.go` — Add `WithCompactor(c ContextCompactor) Option` functional option

4. `pkg/agent/loop.go` — Add `calculateTokenBudget` helper:
   ```go
   func calculateTokenBudget(config *AgentConfig, state *LoopState, systemPrompt string) TokenBudget {
       // Use simple estimator for budget calculation
       msgTokens := estimateMessagesTokens(state.Messages)
       sysTokens := len(systemPrompt) / 4
       return TokenBudget{
           ContextLimit:     getContextLimitForModel(config.Model),
           SystemPromptTkns: sysTokens,
           MaxOutputTkns:    16384,
           MessageTkns:      msgTokens,
       }
   }
   ```

5. `pkg/agent/loop_test.go` — Add compaction integration tests:
   - Test proactive compaction triggers before LLM call
   - Test reactive compaction on max_tokens
   - Test compaction failure graceful degradation
   - Test multiple compactions in one session

6. Update `pkg/subagent/manager.go` — Wire `NoOpCompactor` explicitly (already done, verify)

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/agent/... -race` passes (existing 24 + 4 new = 28 tests)
  - [x] `go test ./pkg/context/... -race` passes (42 tests)
  - [x] `go test ./... -race` passes (all packages pass)
  - [x] `go vet ./...` clean
  - [x] Proactive compaction fires at 80% utilization
  - [x] Reactive compaction fires on `max_tokens` stop reason
  - [x] Loop continues after compaction
  - [x] Loop exits gracefully when compaction fails

---

## Out of Scope
- Tiktoken-based token estimation (requires CGO or external binary)
- Session persistence of compaction history
- Manual `/compact` command (requires CLI transport layer)
- Real Piebald-AI prompt file loading for compaction prompt (use inline prompt text)
- Context window beta flag wiring through CLI args (just implement the lookup function)

## Open Questions
*None — all resolved via user decisions.*

## Implementation Notes

### Import Graph
`pkg/context/` will import:
- `pkg/agent` (for `ContextCompactor`, `TokenBudget`, `CompactRequest`, `HookRunner`)
- `pkg/llm` (for `Client`, `ChatMessage`, `CompletionRequest`, `Stream`)
- `pkg/types` (for `SDKMessage`, `CompactBoundaryMessage`, `HookEvent`)

This follows the same pattern as `pkg/permission/` and `pkg/hooks/` which also import `pkg/agent`.

### Draining Streams for Sync LLM Calls
Since `llm.Client.Complete()` only returns `*Stream`, the summary generator will:
1. Call `client.Complete(ctx, req)` with `Stream: true`
2. Drain via `stream.Accumulate()` to get `*CompletionResponse`
3. Extract text content from `CompletionResponse.Content` blocks
4. This avoids adding `CompleteSync` to the `Client` interface

### Tool Use/Result Pair Safety
The split point calculator must never split between a tool_use assistant message and its corresponding tool result messages. It adjusts the split index backward to keep pairs together.

### Graceful Degradation
If the summary LLM call fails:
1. Fall back to simple truncation (drop oldest messages)
2. Emit a warning but don't error out
3. The loop continues with the truncated history

## References
- Spec: `thoughts/specs/09-CONTEXT-COMPACTION.md`
- Agent loop: `pkg/agent/loop.go`
- Interface: `pkg/agent/interfaces.go:47-51`
- Types: `pkg/types/message_system.go:60-74`
- Hooks: `pkg/types/options.go:145` (`HookEventPreCompact`)
- Similar packages: `pkg/permission/`, `pkg/hooks/`, `pkg/subagent/`
