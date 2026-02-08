# LLM Client & Streaming Implementation Plan

## Overview
Implement `pkg/llm/` — the LLM inference client for the Go port of Claude Code. This package handles all communication with the LiteLLM proxy (OpenAI-compatible `/v1/chat/completions`), including SSE streaming, content block accumulation, error handling, retries, cost tracking, and translation between OpenAI wire format and internal Anthropic-equivalent types.

Also scaffolds minimal `pkg/types/` to support SDK emission types (§12).

## Current State
- **Implementation**: 0% — greenfield project, no Go files exist
- **Spec**: Complete at `thoughts/specs/01-LLM-CLIENT.md` (1034 lines, 13 sections)
- **Dependencies**: No `go.mod`, no `pkg/` directory, no build infrastructure
- **Related spec**: `thoughts/specs/02-MESSAGE-TYPES.md` defines types needed by §12

## Desired End State
- Fully functional `pkg/llm/` package matching Spec 01 §5–§12
- Minimal `pkg/types/` stub for SDK emission types
- All 16 verification checklist items passing
- httptest + golden file test suite covering SSE parsing, accumulation, retry, cost tracking

## Decisions (Resolved)
- **Module name**: `github.com/jg-phare/goat`
- **Go version**: 1.23+
- **Test strategy**: httptest mocks + golden files
- **Types scope**: Minimal `pkg/types/` stub (just what §12 needs)

---

## Phases

### Phase 1: Project Scaffolding
**Goal**: Initialize Go module and create the directory structure.

**Changes**:
- Create `go.mod` with `module github.com/jg-phare/goat` and `go 1.23`
- Create directories: `pkg/llm/`, `pkg/types/`, `testdata/`
- Run `go mod tidy`

**Success Criteria**:
- Automated:
  - [x] `go build ./...` succeeds (no source yet, but module is valid)

---

### Phase 2: Wire Types & Internal Types
**Goal**: Define all Go types from Spec 01 §5 — both OpenAI wire format and Anthropic internal format.

**Changes**:
- `pkg/llm/types_wire.go` — OpenAI wire types (§5.2–§5.3):
  - `CompletionRequest`, `StreamOptions`, `ChatMessage`, `ContentPart`, `ImageURL`
  - `ToolCall`, `FunctionCall`, `ToolDefinition`, `FunctionDef`
  - `StreamChunk`, `Choice`, `Delta`, `Usage`
- `pkg/llm/types_internal.go` — Anthropic internal types (§5.4):
  - `BetaMessage`, `ContentBlock`, `BetaUsage`
- `pkg/llm/config.go` — Configuration types (§5.1):
  - `ClientConfig`, `RetryConfig` with sensible defaults

**Success Criteria**:
- Automated:
  - [x] `go build ./pkg/llm/...` compiles
  - [x] `go vet ./pkg/llm/...` passes
- Manual:
  - [ ] Types match spec §5 definitions exactly (field names, JSON tags, types)

---

### Phase 3: Translation Functions
**Goal**: Implement all format translation helpers from §8.

**Changes**:
- `pkg/llm/translate.go`:
  - `translateFinishReason(fr string) string` — "stop"→"end_turn", "tool_calls"→"tool_use", "length"→"max_tokens"
  - `translateUsage(u *Usage) BetaUsage` — OpenAI Usage → Anthropic BetaUsage
  - `toRequestModel(model string) string` — prepend "anthropic/" if needed
  - `fromResponseModel(model string) string` — strip "anthropic/" prefix
  - `ToBetaMessage()` method on `CompletionResponse`
- `pkg/llm/translate_test.go`:
  - Table-driven tests for all translation functions
  - Edge cases: empty strings, nil usage, already-prefixed models, unknown finish reasons

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/llm/... -run TestTranslate` passes
  - [x] Finish reason translation covers all 3 mappings + unknown passthrough
  - [x] Usage translation handles nil input (returns zero BetaUsage)
  - [x] Model prefix is idempotent (double-prefix protection)

---

### Phase 4: SSE Parser & Tool Call Accumulator
**Goal**: Implement SSE line parsing (§7.1–§7.2) and tool call delta accumulation (§7.3).

**Changes**:
- `pkg/llm/sse.go`:
  - `StreamEvent` struct (Chunk, Err, Done fields)
  - `ParseSSEStream(ctx context.Context, body io.ReadCloser) <-chan StreamEvent`
  - Line-by-line SSE parsing: skip comments (`:`), skip empty lines, handle `data: [DONE]`, unmarshal JSON chunks
  - Context cancellation support
- `pkg/llm/accumulator.go`:
  - `ToolCallAccumulator` struct with `calls map[int]*ToolCall`
  - `NewToolCallAccumulator()`, `AddDelta(delta ToolCall)`, `Complete() []ToolCall`
  - Index-ordered output, ID/Name from first delta, arguments appended
- `pkg/llm/sse_test.go`:
  - httptest server returning canned SSE responses
  - Golden files in `testdata/sse/` for expected parse output
  - Cases: simple text, tool calls, thinking blocks, malformed JSON (skip), keep-alive pings, `[DONE]`
- `pkg/llm/accumulator_test.go`:
  - Table-driven: single tool call, multiple tool calls, interleaved deltas, sparse indices

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/llm/... -run TestParseSSE` passes
  - [x] `go test ./pkg/llm/... -run TestToolCallAccumulator` passes
  - [x] SSE parser handles: comment lines, empty lines, `[DONE]`, malformed JSON (skipped, not fatal)
  - [x] Tool call accumulator correctly orders by index, merges ID/Name/Arguments
  - [x] Context cancellation returns `context.Canceled`

---

### Phase 5: Error Handling & Retry
**Goal**: Implement error classification (§10.1–§10.3) and retry logic (§10.2).

**Changes**:
- `pkg/llm/errors.go`:
  - `LLMError` struct with `StatusCode`, `SDKError`, `Message`, `Retryable`, `RetryAfter`
  - `classifyError(resp *http.Response) *LLMError` — maps HTTP status to SDK error type
  - `isRetryable(statusCode int, retryableStatuses []int) bool`
  - `ErrMaxRetriesExceeded` error type
- `pkg/llm/retry.go`:
  - `doWithRetry(ctx, config, makeRequest) (*http.Response, error)`
  - Exponential backoff with jitter
  - `Retry-After` header parsing (both seconds and date formats)
  - Context cancellation during sleep
- `pkg/llm/errors_test.go`:
  - Table-driven classification: 401→authentication_failed, 429→rate_limit, 500→server_error, etc.
  - Retry-After header parsing
- `pkg/llm/retry_test.go`:
  - httptest server returning sequential status codes (429, 429, 200)
  - Verify retry count, backoff timing (approximate), context cancellation during retry

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/llm/... -run TestClassifyError` passes — all 9 HTTP status mappings
  - [x] `go test ./pkg/llm/... -run TestRetry` passes — retryable codes retried, non-retryable fail immediately
  - [x] 429 respects `Retry-After` header
  - [x] Context cancellation during backoff sleep returns promptly

---

### Phase 6: Cost Tracking
**Goal**: Implement per-model pricing, cost calculation, and cumulative cost tracking (§11).

**Changes**:
- `pkg/llm/cost.go`:
  - `ModelPricing` struct (InputPerMTok, OutputPerMTok, CacheReadPerMTok, CacheCreatePerMTok)
  - `DefaultPricing` map for claude-opus-4-5, claude-sonnet-4-5, claude-haiku-4-5
  - `CalculateCost(model string, usage BetaUsage) float64`
  - `CostTracker` struct with sync.Mutex, `Add()`, `TotalCost()`, `ModelBreakdown()`
  - `ModelUsageAccum` for per-model accumulation
- `pkg/llm/cost_test.go`:
  - Known-value cost calculations for each model tier
  - Concurrent `Add()` calls (goroutine safety test)
  - Unknown model returns 0 cost
  - Edge case: zero usage

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/llm/... -run TestCalculateCost` passes
  - [x] `go test ./pkg/llm/... -run TestCostTracker` passes (including concurrent test with `-race`)
  - [x] Known-value: 1000 input tokens on claude-opus-4-5 = $0.015

---

### Phase 7: Stream & Accumulation
**Goal**: Implement the `Stream` type and accumulation algorithm (§6, §7.4).

**Changes**:
- `pkg/llm/stream.go`:
  - `Stream` struct wrapping the SSE event channel and HTTP response body
  - `Next() (*StreamChunk, error)` — reads next event from channel
  - `Accumulate() (*CompletionResponse, error)` — full algorithm from §7.4
  - `AccumulateWithCallback(cb func(*StreamChunk)) (*CompletionResponse, error)`
  - `Close() error` — cancels context and closes HTTP body
  - Content block ordering: thinking → text → tool_use
  - Tool call arguments JSON parsing (string → map[string]any)
- `pkg/llm/stream_test.go`:
  - httptest SSE server with various response shapes
  - Golden files for accumulated `CompletionResponse` (JSON comparison)
  - Cases: text only, tool calls only, text + tool calls, thinking + text + tool calls, empty response
  - Arguments parsing: simple JSON, nested objects, escaped quotes, unicode

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/llm/... -run TestStreamAccumulate` passes
  - [x] Content blocks ordered: thinking → text → tool_use
  - [x] Tool call arguments correctly parsed from JSON string to map
  - [x] Golden file comparison matches expected output
  - [x] `Close()` releases HTTP body

---

### Phase 8: Client Implementation & Request Construction
**Goal**: Implement the `Client` interface, HTTP transport, and request construction (§6, §9).

**Changes**:
- `pkg/llm/client.go`:
  - `httpClient` struct implementing `Client` interface
  - `NewClient(cfg ClientConfig) Client` constructor with defaults
  - `Complete(ctx, req) (*Stream, error)` — HTTP POST, retry wrapper, SSE body
  - `Model() string`, `SetModel(model string)` with mutex
  - Default configuration: MaxRetries=3, InitialBackoff=1s, MaxBackoff=30s, BackoffFactor=2.0
- `pkg/llm/request.go`:
  - `buildCompletionRequest(config, systemPrompt, messages, tools, loopState) *CompletionRequest`
  - `convertToOpenAIMessage(msg) ChatMessage` — handles user, assistant, tool_result conversion
  - `extra_body` assembly: thinking, betas, metadata
- `pkg/llm/client_test.go`:
  - httptest server simulating full LiteLLM responses
  - End-to-end: `Complete()` → `Accumulate()` → verify `CompletionResponse`
  - Error scenarios: 401 (immediate fail), 429 (retry then succeed), 500 (retry exhausted)
  - Context cancellation during streaming
- `pkg/llm/request_test.go`:
  - Request construction verification
  - extra_body serialization (thinking, betas, metadata)
  - Message conversion: user messages, assistant with tool calls, tool results

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/llm/... -run TestClient` passes
  - [x] `go test ./pkg/llm/... -run TestBuildRequest` passes
  - [x] End-to-end streaming: HTTP → SSE → accumulate → CompletionResponse correct
  - [x] extra_body contains thinking config when MaxThinkingTokens > 0
  - [x] Model prefix applied on request, stripped on response
  - [x] `go test -race ./pkg/llm/...` passes (concurrent safety)

---

### Phase 9: Minimal Types Stub & SDK Emission
**Goal**: Scaffold minimal `pkg/types/` and implement SDK emission functions (§12).

**Changes**:
- `pkg/types/messages.go`:
  - `MessageType` string constants: `MessageTypeStreamEvent`, `MessageTypeAssistant`
  - `PartialAssistantMessage` struct (Type, Event, ParentToolUseID, UUID, SessionID)
  - `AssistantMessage` struct (Type, Message, ParentToolUseID, Error, UUID, SessionID)
- `pkg/llm/emit.go`:
  - `EmitStreamEvent(chunk, parentToolUseID, sessionID) types.PartialAssistantMessage`
  - `EmitAssistantMessage(resp, parentToolUseID, sessionID, err) types.AssistantMessage`
  - `chunkToStreamEvent(chunk)` helper — converts OpenAI chunk to Anthropic-equivalent event
- `pkg/llm/emit_test.go`:
  - Verify emission wraps correctly
  - UUID populated
  - ParentToolUseID forwarded
  - Error field nil on success, populated on failure
- Add `github.com/google/uuid` dependency

**Success Criteria**:
- Automated:
  - [x] `go build ./pkg/types/...` compiles
  - [x] `go build ./pkg/llm/...` compiles
  - [x] `go test ./pkg/llm/... -run TestEmit` passes
  - [x] `go test ./...` — full project builds and tests pass

---

### Phase 10: Golden Files & Integration Test Suite
**Goal**: Create comprehensive golden file test data and run the full verification checklist.

**Changes**:
- `testdata/sse/text_only.txt` — SSE stream with text-only response
- `testdata/sse/tool_calls.txt` — SSE stream with 3 tool calls
- `testdata/sse/thinking_text_tools.txt` — SSE stream with thinking + text + tool calls
- `testdata/sse/malformed.txt` — SSE stream with malformed JSON lines (should be skipped)
- `testdata/sse/keepalive.txt` — SSE stream with comment/ping lines interspersed
- `testdata/golden/text_only.json` — Expected accumulated CompletionResponse
- `testdata/golden/tool_calls.json` — Expected accumulated CompletionResponse
- `testdata/golden/thinking_text_tools.json` — Expected accumulated CompletionResponse
- `testdata/golden/beta_message_text.json` — Expected BetaMessage JSON output
- `testdata/golden/beta_message_tools.json` — Expected BetaMessage JSON output
- `pkg/llm/golden_test.go`:
  - Reads SSE test data, feeds to parser, accumulates, compares to golden JSON
  - BetaMessage serialization comparison (validates JSON structure matches Anthropic native)
  - `-update` flag to regenerate golden files

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/llm/... -run TestGolden` passes
  - [x] `go test -race ./pkg/llm/...` passes (all tests, race detector)
  - [x] `go vet ./...` passes
  - [x] All 16 verification checklist items from §13 covered by tests

---

## File Layout Summary

```
github.com/jg-phare/goat/
├── go.mod
├── go.sum
├── pkg/
│   ├── llm/
│   │   ├── accumulator.go        — ToolCallAccumulator (§7.3)
│   │   ├── accumulator_test.go
│   │   ├── client.go             — Client interface + httpClient impl (§6)
│   │   ├── client_test.go
│   │   ├── config.go             — ClientConfig, RetryConfig (§5.1)
│   │   ├── cost.go               — ModelPricing, CalculateCost, CostTracker (§11)
│   │   ├── cost_test.go
│   │   ├── emit.go               — EmitStreamEvent, EmitAssistantMessage (§12)
│   │   ├── emit_test.go
│   │   ├── errors.go             — LLMError, classifyError (§10)
│   │   ├── errors_test.go
│   │   ├── golden_test.go        — Golden file integration tests
│   │   ├── request.go            — buildCompletionRequest, convertToOpenAIMessage (§9)
│   │   ├── request_test.go
│   │   ├── retry.go              — doWithRetry, backoff logic (§10.2)
│   │   ├── retry_test.go
│   │   ├── sse.go                — ParseSSEStream, StreamEvent (§7.1-§7.2)
│   │   ├── sse_test.go
│   │   ├── stream.go             — Stream type, Accumulate (§6, §7.4)
│   │   ├── stream_test.go
│   │   ├── translate.go          — Translation functions (§8)
│   │   ├── translate_test.go
│   │   ├── types_internal.go     — BetaMessage, ContentBlock, BetaUsage (§5.4)
│   │   └── types_wire.go         — OpenAI wire types (§5.2-§5.3)
│   └── types/
│       └── messages.go           — Minimal stub: message type constants + emission structs
├── testdata/
│   ├── sse/                      — Canned SSE response files
│   │   ├── text_only.txt
│   │   ├── tool_calls.txt
│   │   ├── thinking_text_tools.txt
│   │   ├── malformed.txt
│   │   └── keepalive.txt
│   └── golden/                   — Expected output JSON files
│       ├── text_only.json
│       ├── tool_calls.json
│       ├── thinking_text_tools.json
│       ├── beta_message_text.json
│       └── beta_message_tools.json
└── thoughts/
    ├── specs/
    │   ├── 01-LLM-CLIENT.md      — Source spec (reference)
    │   └── 02-MESSAGE-TYPES.md   — Types spec (reference)
    └── plans/
        └── 2026-02-08-llm-client.md  — This plan
```

## Out of Scope
- Full `pkg/types/` implementation (Spec 02) — only minimal stub for §12
- `pkg/agent/` (Spec 03) — the agentic loop that consumes this client
- `pkg/tools/` (Spec 04) — tool registry and execution
- Non-streaming `/v1/chat/completions` (the spec is streaming-only)
- Direct Anthropic API calls (we go through LiteLLM exclusively)
- Authentication/OAuth flows
- WebSocket transport (SSE only)
- `cmd/` entrypoint or CLI scaffolding

## Open Questions
*None — all resolved.*

## References
- Spec: `thoughts/specs/01-LLM-CLIENT.md`
- Types spec: `thoughts/specs/02-MESSAGE-TYPES.md`
- Architecture: `thoughts/specs/00-SPEC-INDEX.md`
- Research: `thoughts/research/2026-02-08-llm-client-spec-vs-codebase.md`
