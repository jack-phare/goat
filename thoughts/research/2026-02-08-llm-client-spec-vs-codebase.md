---
date: 2026-02-08
topic: LLM Client Spec vs Codebase Analysis
status: complete
---

# LLM Client (Spec 01) vs Codebase Analysis

## Research Question
What parts of the LLM Client spec (01-LLM-CLIENT.md) are already implemented in pkg/llm/ or elsewhere? What's missing? What's the current file structure? Are there any deviations from the spec?

## Summary

**Implementation Status: 0% — This is a greenfield project.**

The repository at `/Users/jg_phare/Desktop/side/goat` contains **only specification documents** — 12 comprehensive specs in `thoughts/specs/`. There are **zero Go source files**, no `go.mod`, no `pkg/` directory, no `cmd/` directory, no tests, and no build infrastructure. The project is entirely in the design/planning phase.

The LLM Client spec (`01-LLM-CLIENT.md`, 1034 lines) is thorough and implementation-ready, providing complete Go type definitions, interface contracts, algorithms (in pseudocode), translation tables, and a 16-point verification checklist.

## Detailed Findings

### 1. Current File Structure

```
/Users/jg_phare/Desktop/side/goat/
└── thoughts/
    └── specs/
        ├── 00-SPEC-INDEX.md          (Master index)
        ├── 01-LLM-CLIENT.md          (LLM client & streaming)
        ├── 02-MESSAGE-TYPES.md       (Message types & protocol)
        ├── 03-AGENTIC-LOOP.md        (Agentic loop)
        ├── 04-TOOL-REGISTRY.md       (Tool registry & execution)
        ├── 05-SYSTEM-PROMPT.md       (System prompt assembly)
        ├── 06-PERMISSION-SYSTEM.md   (Permission system)
        ├── 07-HOOK-SYSTEM.md         (Hook system)
        ├── 08-SUBAGENT-MANAGER.md    (Subagent & task manager)
        ├── 09-CONTEXT-COMPACTION.md  (Context & compaction)
        ├── 10-MCP-INTEGRATION.md     (MCP integration)
        ├── 11-SESSION-CHECKPOINT.md  (Session & checkpoint store)
        └── 12-TRANSPORT.md           (Transport layer)
```

**Missing entirely:**
- `go.mod` / `go.sum`
- `pkg/` directory (all 12 planned packages)
- `cmd/` directory
- Any `.go` files
- Any test files or testdata
- Any build scripts (Makefile, Taskfile)

### 2. Spec 01 Component Breakdown — What Needs Implementation

#### §5 Go Type Definitions (spec lines 261–457) — NOT IMPLEMENTED
15 types defined in spec, zero implemented:

| Type | Spec Location | Purpose |
|------|---------------|---------|
| `ClientConfig` | §5.1 | LLM client configuration (BaseURL, APIKey, Model, etc.) |
| `RetryConfig` | §5.1 | Retry behavior (MaxRetries, backoff, jitter, retryable statuses) |
| `CompletionRequest` | §5.2 | OpenAI `/v1/chat/completions` request body |
| `StreamOptions` | §5.2 | `stream_options.include_usage` toggle |
| `ChatMessage` | §5.2 | OpenAI-format message (role, content, tool_calls) |
| `ContentPart` | §5.2 | Multi-part content (text, image_url) |
| `ImageURL` | §5.2 | Image URL for multimodal |
| `ToolCall` | §5.2 | Assistant tool invocation |
| `FunctionCall` | §5.2 | Function name + arguments string |
| `ToolDefinition` | §5.2 | OpenAI-format tool definition |
| `FunctionDef` | §5.2 | Function name, description, parameters schema |
| `StreamChunk` | §5.3 | Single SSE chunk from LiteLLM |
| `Choice` / `Delta` | §5.3 | Streaming choice with incremental delta |
| `Usage` | §5.3 | Token usage (OpenAI wire format) |
| `BetaMessage` | §5.4 | Anthropic-equivalent internal message |
| `ContentBlock` | §5.4 | Discriminated union: text / tool_use / thinking |
| `BetaUsage` | §5.4 | Anthropic usage with cache token fields |

#### §6 Client Interface (spec lines 462–508) — NOT IMPLEMENTED
- `Client` interface: `Complete()`, `Model()`, `SetModel()`
- `Stream` struct: `Next()`, `Accumulate()`, `AccumulateWithCallback()`, `Close()`
- `CompletionResponse` struct: accumulated result with content blocks, tool calls, stop reason

#### §7 SSE Parser (spec lines 512–665) — NOT IMPLEMENTED
- `ParseSSEStream()` — channel-based SSE line parser
- `StreamEvent` — chunk/error/done wrapper
- `ToolCallAccumulator` — incremental delta accumulation by index
- Full accumulation algorithm (pseudocode provided)

#### §8 Translation Functions (spec lines 668–740) — NOT IMPLEMENTED
- `translateFinishReason()` — "stop"→"end_turn", "tool_calls"→"tool_use", "length"→"max_tokens"
- `translateUsage()` — OpenAI Usage → Anthropic BetaUsage
- `toRequestModel()` / `fromResponseModel()` — "anthropic/" prefix handling
- `ToBetaMessage()` — CompletionResponse → BetaMessage conversion

#### §9 Request Construction (spec lines 743–827) — NOT IMPLEMENTED
- `buildCompletionRequest()` — assembles full request from loop state
- `convertToOpenAIMessage()` — internal messages → OpenAI format
- `extra_body` assembly (thinking, betas, metadata)

#### §10 Error Handling & Retry (spec lines 829–891) — NOT IMPLEMENTED
- HTTP status → SDK error type classification (9 mappings)
- `doWithRetry()` — exponential backoff with jitter, Retry-After header support
- `LLMError` type — wraps HTTP errors with SDK classification

#### §11 Cost Tracking (spec lines 893–970) — NOT IMPLEMENTED
- `ModelPricing` struct + `DefaultPricing` map (3 models)
- `CalculateCost()` — USD cost from model + usage
- `CostTracker` — concurrent-safe cumulative cost tracking
- `ModelUsageAccum` — per-model usage accumulation

#### §12 SDK Emission Types (spec lines 972–1012) — NOT IMPLEMENTED
- `EmitStreamEvent()` — wrap StreamChunk as PartialAssistantMessage
- `EmitAssistantMessage()` — wrap CompletionResponse as AssistantMessage
- Note: depends on `pkg/types/` (Spec 02) which also doesn't exist

### 3. Cross-Spec Dependencies

Spec 01 references types from Spec 02 (Message Types):
- `types.PartialAssistantMessage` (§12.1)
- `types.AssistantMessage` (§12.2)
- `types.MessageTypeStreamEvent`, `types.MessageTypeAssistant` (§12)

**Implication**: Implementing §12 of Spec 01 requires at least partial implementation of Spec 02 (`pkg/types/`). All other sections (§5–§11) are self-contained within `pkg/llm/`.

### 4. Deviations from Spec

**N/A** — There is no implementation to deviate. The spec itself is internally consistent and well-structured.

### 5. External Dependencies (from spec analysis)

The implementation will require these Go dependencies:
- `net/http` (stdlib) — HTTP client
- `encoding/json` (stdlib) — JSON marshal/unmarshal
- `bufio` (stdlib) — SSE line reading
- `sync` (stdlib) — Mutex for CostTracker
- `github.com/google/uuid` — UUID generation for message IDs (§12)
- Potentially `golang.org/x/time/rate` or similar for rate limiting

## Suggested Implementation File Layout

Based on the spec's logical groupings:

```
pkg/llm/
├── client.go           — Client interface, httpClient struct, Complete(), doWithRetry()
├── config.go           — ClientConfig, RetryConfig, defaults
├── types_wire.go       — OpenAI wire types: CompletionRequest, ChatMessage, StreamChunk, etc. (§5.2, §5.3)
├── types_internal.go   — Anthropic internal types: BetaMessage, ContentBlock, BetaUsage (§5.4)
├── stream.go           — Stream struct, Next(), Accumulate(), AccumulateWithCallback()
├── sse.go              — ParseSSEStream(), StreamEvent, SSE line parsing (§7.1, §7.2)
├── accumulator.go      — ToolCallAccumulator (§7.3)
├── translate.go        — translateFinishReason, translateUsage, model prefix helpers (§8)
├── request.go          — buildCompletionRequest(), convertToOpenAIMessage() (§9)
├── errors.go           — LLMError, classifyError(), isRetryable() (§10)
├── cost.go             — ModelPricing, CalculateCost, CostTracker (§11)
├── emit.go             — EmitStreamEvent, EmitAssistantMessage (§12)
└── client_test.go      — Tests covering verification checklist (§13)
```

## Verification Checklist Status

All 16 items from §13 are **unchecked** — none can be verified without implementation:

- [ ] Streaming parity
- [ ] Tool call accumulation
- [ ] Tool call ID mapping
- [ ] Arguments parsing
- [ ] Usage tracking
- [ ] Cost calculation
- [ ] Finish reason translation
- [ ] Error classification
- [ ] Retry behavior
- [ ] SSE parser robustness
- [ ] Thinking mode
- [ ] LiteLLM extra_body
- [ ] Model prefix
- [ ] Context cancellation
- [ ] Concurrent safety
- [ ] BetaMessage construction

## Related Documentation

| Document | Description |
|----------|-------------|
| `thoughts/specs/00-SPEC-INDEX.md` | Master index, architecture rationale, verification strategy |
| `thoughts/specs/01-LLM-CLIENT.md` | This spec — LLM client & streaming (1034 lines) |
| `thoughts/specs/02-MESSAGE-TYPES.md` | Message protocol types (1192 lines) — dependency for §12 |
| `thoughts/specs/03-AGENTIC-LOOP.md` | Agentic loop — primary consumer of LLM client |
| `thoughts/specs/12-TRANSPORT.md` | Transport layer — uses SDK emission types from §12 |

## Open Questions

1. **Module name**: What should the Go module be named? (e.g., `github.com/username/goat`)
2. **Dependency on Spec 02**: Should `pkg/types/` be scaffolded first (or concurrently) to unblock §12?
3. **LiteLLM version pinning**: Spec mentions version-dependent behavior for thinking blocks (§4.4) — which LiteLLM version to target?
4. **Test strategy**: Should we use golden file tests, httptest server mocks, or both for SSE streaming verification?
5. **Build target**: Minimum Go version? (impacts generics usage, `errors.Join`, etc.)
