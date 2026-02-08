# Spec 01: LLM Client & Streaming

**Go Package**: `pkg/llm/`
**Source References**:
- `sdk.d.ts:1373-1379` — `SDKPartialAssistantMessage` (wraps `BetaRawMessageStreamEvent`)
- `sdk.d.ts:1140-1147` — `SDKAssistantMessage` (wraps `BetaMessage`)
- `sdk.d.ts:1149` — `SDKAssistantMessageError` union
- `sdk.d.ts:429-431` — `NonNullableUsage`, `ModelUsage`
- Anthropic Messages API streaming: `docs.anthropic.com/en/api/messages-streaming`
- LiteLLM proxy docs: OpenAI-compatible `/v1/chat/completions` with `stream: true`

---

## 1. Purpose

The LLM client handles all inference calls from the agentic loop. Claude Code natively uses the Anthropic Messages API (SSE streaming). Our Go port routes through **LiteLLM proxy**, which exposes an OpenAI-compatible `/v1/chat/completions` endpoint that translates to/from the Anthropic backend.

The client must:
1. Compose request payloads (system prompt, messages, tools, thinking config, betas)
2. Stream responses via SSE, yielding events as they arrive
3. Parse content blocks (text, tool_use, thinking) from streaming deltas
4. Accumulate the final `BetaMessage` equivalent from stream chunks
5. Map OpenAI wire format back to internal Anthropic-equivalent types
6. Handle errors, retries, rate limits, and cost tracking
7. Support extended thinking via LiteLLM `extra_body` passthrough

---

## 2. Streaming State Machine

```
                ┌──────────────┐
                │   IDLE       │
                │  (no active  │
                │   request)   │
                └──────┬───────┘
                       │ Complete(ctx, req)
                       ▼
                ┌──────────────┐
           ┌───▶│  CONNECTING  │
           │    │  (HTTP POST  │
           │    │   /v1/chat/  │
           │    │ completions) │
           │    └──────┬───────┘
           │           │ 200 OK + SSE body
           │           ▼
           │    ┌──────────────┐
           │    │  STREAMING   │◀─────────────────────┐
           │    │  (read SSE   │                       │
           │    │   lines)     │                       │
           │    └──────┬───────┘                       │
           │           │                               │
           │    ┌──────┴──────────┐                    │
           │    │                 │                    │
           │    ▼                 ▼                    │
           │  "data: {...}"    "data: [DONE]"          │
           │    │                 │                    │
           │    ▼                 ▼                    │
           │  ┌──────────┐  ┌──────────┐              │
           │  │ PARSE    │  │ DONE     │              │
           │  │ CHUNK    │  │ (emit    │              │
           │  │ (unmarshal│  │  final   │              │
           │  │  delta)  │  │  Usage)  │              │
           │  └────┬─────┘  └──────────┘              │
           │       │                                   │
           │       ├── role delta ─────────────────────┘
           │       ├── content delta ──────────────────┘
           │       ├── tool_calls delta ───────────────┘
           │       └── finish_reason set ──► FINALIZING
           │
           │  (on HTTP error)
           │    ┌──────────────┐
           └────│   RETRY      │
                │  (backoff    │
                │   + jitter)  │
                └──────────────┘
```

---

## 3. Wire Format: Anthropic Messages API (Native Reference)

This section documents the native Anthropic wire format for reference. The Go client does **not** use this directly — it is routed through LiteLLM. However, the internal types (`BetaMessage`, `ContentBlock`) mirror this format, so understanding it is essential.

### 3.1 Request

```json
{
  "model": "claude-opus-4-5-20250514",
  "max_tokens": 16384,
  "system": "<system prompt string>",
  "messages": [
    {"role": "user", "content": "..."},
    {"role": "assistant", "content": [
      {"type": "text", "text": "..."},
      {"type": "tool_use", "id": "toolu_xxx", "name": "Bash", "input": {"command": "ls"}}
    ]},
    {"role": "user", "content": [
      {"type": "tool_result", "tool_use_id": "toolu_xxx", "content": "file1.go\nfile2.go"}
    ]}
  ],
  "tools": [
    {"name": "Bash", "description": "...", "input_schema": {"type": "object", "properties": {"...": "..."}}}
  ],
  "metadata": {"user_id": "session-xxx"},
  "stream": true
}
```

### 3.2 SSE Event Sequence (Anthropic native)

```
event: message_start
data: {"type":"message_start","message":{"id":"msg_xxx","type":"message","role":"assistant","content":[],"model":"...","stop_reason":null,"usage":{"input_tokens":1234,"output_tokens":1}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Let me analyze..."}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Hello"}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: content_block_start
data: {"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"toolu_xxx","name":"Bash","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"command\":"}}

event: content_block_delta
data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":" \"ls\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":2}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":142}}

event: message_stop
data: {"type":"message_stop"}
```

### 3.3 Content Block Types

| Block Type | Start Payload | Delta Type | When |
|------------|---------------|------------|------|
| `text` | `{"type":"text","text":""}` | `text_delta` | Normal text response |
| `tool_use` | `{"type":"tool_use","id":"...","name":"...","input":{}}` | `input_json_delta` | Tool invocation |
| `thinking` | `{"type":"thinking","thinking":""}` | `thinking_delta` | Extended thinking mode enabled |

### 3.4 Stop Reasons (Anthropic → Agent Loop Action)

| `stop_reason` | Meaning | Agent Loop Action |
|---------------|---------|-------------------|
| `"end_turn"` | Model finished naturally | Return result |
| `"tool_use"` | Model wants to call tools | Execute tools → continue loop |
| `"max_tokens"` | Hit max_tokens limit | May need continuation or compaction |
| `"stop_sequence"` | Hit a stop sequence | Return result |

---

## 4. Wire Format: LiteLLM Proxy (OpenAI-Compatible)

LiteLLM translates the Anthropic format to OpenAI format. Our Go client speaks this protocol exclusively.

### 4.1 Request (OpenAI format via LiteLLM)

```json
{
  "model": "anthropic/claude-opus-4-5-20250514",
  "messages": [
    {"role": "system", "content": "<system prompt>"},
    {"role": "user", "content": "..."},
    {"role": "assistant", "content": null, "tool_calls": [
      {"id": "call_xxx", "type": "function", "function": {"name": "Bash", "arguments": "{\"command\":\"ls\"}"}}
    ]},
    {"role": "tool", "tool_call_id": "call_xxx", "content": "file1.go\nfile2.go"}
  ],
  "tools": [
    {"type": "function", "function": {"name": "Bash", "description": "...", "parameters": {"type": "object", "properties": {"...": "..."}}}}
  ],
  "stream": true,
  "stream_options": {"include_usage": true},
  "extra_body": {
    "thinking": {"type": "enabled", "budget_tokens": 10000},
    "betas": ["context-1m-2025-08-07"],
    "metadata": {"user_id": "session-xxx"}
  }
}
```

### 4.2 LiteLLM `extra_body` Passthrough

LiteLLM forwards unrecognized fields in `extra_body` directly to the Anthropic API. We use this for:

| Field | Type | Purpose |
|-------|------|---------|
| `thinking` | `{"type":"enabled","budget_tokens":N}` | Enable extended thinking; N from `Options.maxThinkingTokens` |
| `betas` | `string[]` | Beta feature flags, e.g. `["context-1m-2025-08-07"]` for 1M context |
| `metadata` | `{"user_id":string}` | Request metadata for audit/tracking |

When `maxThinkingTokens` is set in `Options` (sdk.d.ts:609), the client **must** include the `thinking` field in `extra_body`. When `betas` is set (sdk.d.ts:596), the client **must** forward them.

### 4.3 SSE Streaming (OpenAI format from LiteLLM)

```
data: {"id":"chatcmpl-xxx","object":"chat.completion.chunk","created":1234,"model":"claude-opus-4-5-20250514","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}

data: {"id":"chatcmpl-xxx","object":"chat.completion.chunk","created":1234,"model":"claude-opus-4-5-20250514","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}

data: {"id":"chatcmpl-xxx","object":"chat.completion.chunk","created":1234,"model":"claude-opus-4-5-20250514","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_xxx","type":"function","function":{"name":"Bash","arguments":""}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-xxx","object":"chat.completion.chunk","created":1234,"model":"claude-opus-4-5-20250514","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"command\":"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-xxx","object":"chat.completion.chunk","created":1234,"model":"claude-opus-4-5-20250514","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: {"id":"chatcmpl-xxx","object":"chat.completion.chunk","created":1234,"model":"claude-opus-4-5-20250514","choices":[],"usage":{"prompt_tokens":1234,"completion_tokens":142,"total_tokens":1376}}

data: [DONE]
```

### 4.4 Thinking Blocks via LiteLLM

LiteLLM exposes Anthropic thinking blocks as a provider-specific extension. Two possible representations:

1. **`reasoning_content` field on delta** — LiteLLM may add a `reasoning_content` string field alongside `content`
2. **Provider-prefixed metadata** — Varies by LiteLLM version

The client must probe for both and accumulate thinking text into a `ContentBlock{Type: "thinking"}`. If neither is present but `extra_body.thinking` was sent, the thinking content is suppressed by LiteLLM and the client proceeds without it.

### 4.5 Key Translation Table

| Aspect | Anthropic Native | OpenAI (LiteLLM) | Notes |
|--------|-----------------|-------------------|-------|
| Model ID | `claude-opus-4-5-20250514` | `anthropic/claude-opus-4-5-20250514` | Prepend `anthropic/` prefix |
| System prompt | `"system": "..."` (top-level) | `{"role": "system", "content": "..."}` | First message |
| Tool definition schema key | `"input_schema"` | `"parameters"` | Nested under `function` |
| Tool definition wrapper | `{"name", "description", "input_schema"}` | `{"type":"function","function":{"name","description","parameters"}}` | Extra nesting |
| Tool call in response | Content block `{"type":"tool_use","id","name","input"}` | `"tool_calls":[{"id","type":"function","function":{"name","arguments"}}]` | `input` is object vs `arguments` is JSON string |
| Tool result | `{"role":"user","content":[{"type":"tool_result","tool_use_id","content"}]}` | `{"role":"tool","tool_call_id","content"}` | Separate message per tool |
| Stop: tools | `"stop_reason":"tool_use"` | `"finish_reason":"tool_calls"` | |
| Stop: normal | `"stop_reason":"end_turn"` | `"finish_reason":"stop"` | |
| Stop: length | `"stop_reason":"max_tokens"` | `"finish_reason":"length"` | |
| Streaming | Named SSE events (`event: content_block_start`) | `data:` lines only, `data: [DONE]` terminal | No event names |
| Usage | In `message_start` + `message_delta` | Final chunk when `stream_options.include_usage` | Single aggregated usage |
| Thinking | `{"type":"thinking"}` content block with `thinking_delta` | `reasoning_content` field on delta (provider-specific) | See §4.4 |
| Cache tokens | `cache_read_input_tokens`, `cache_creation_input_tokens` | Same fields in usage (LiteLLM passes through) | Anthropic-specific |

---

## 5. Go Type Definitions

### 5.1 Client Configuration

```go
package llm

import (
    "context"
    "io"
    "net/http"
    "time"
)

// ClientConfig holds LLM client configuration.
type ClientConfig struct {
    BaseURL     string            // LiteLLM proxy URL, e.g. "http://localhost:4000/v1"
    APIKey      string            // LiteLLM virtual key or Anthropic API key
    Model       string            // Default model, e.g. "anthropic/claude-opus-4-5-20250514"
    MaxTokens   int               // Default max_tokens for responses (16384)
    Headers     map[string]string // Additional HTTP headers (e.g. X-Request-ID)
    HTTPClient  *http.Client      // Custom HTTP client (for timeouts, TLS, proxies)
    Retry       RetryConfig
    CostTracker *CostTracker      // Optional cost accumulation across requests
}

// RetryConfig controls retry behavior for transient failures.
type RetryConfig struct {
    MaxRetries     int           // Max retry attempts (default: 3)
    InitialBackoff time.Duration // Initial backoff (default: 1s)
    MaxBackoff     time.Duration // Max backoff cap (default: 30s)
    BackoffFactor  float64       // Multiplier per retry (default: 2.0)
    JitterFraction float64       // Random jitter as fraction of backoff (default: 0.1)
    RetryableStatuses []int      // HTTP codes to retry (default: 429, 529, 500, 502, 503)
}
```

### 5.2 Request Types (OpenAI Wire Format)

```go
// CompletionRequest maps to OpenAI /v1/chat/completions request body.
type CompletionRequest struct {
    Model         string           `json:"model"`
    Messages      []ChatMessage    `json:"messages"`
    Tools         []ToolDefinition `json:"tools,omitempty"`
    Stream        bool             `json:"stream"`
    MaxTokens     int              `json:"max_tokens,omitempty"`
    Temperature   *float64         `json:"temperature,omitempty"`
    TopP          *float64         `json:"top_p,omitempty"`
    Stop          []string         `json:"stop,omitempty"`
    StreamOptions *StreamOptions   `json:"stream_options,omitempty"`

    // LiteLLM passthrough for Anthropic-specific fields
    ExtraBody map[string]any `json:"extra_body,omitempty"`
}

// StreamOptions requests usage info in the final streaming chunk.
type StreamOptions struct {
    IncludeUsage bool `json:"include_usage"`
}

// ChatMessage is an OpenAI-format message for the messages array.
// Content is `any` because it can be:
//   - string (simple text)
//   - []ContentPart (multimodal: text + images)
//   - nil (assistant message with only tool_calls)
type ChatMessage struct {
    Role       string     `json:"role"`                  // "system"|"user"|"assistant"|"tool"
    Content    any        `json:"content"`               // string | []ContentPart | nil
    ToolCalls  []ToolCall `json:"tool_calls,omitempty"`  // assistant messages only
    ToolCallID string     `json:"tool_call_id,omitempty"` // tool result messages only
    Name       string     `json:"name,omitempty"`        // optional sender name
}

// ContentPart for multi-part content arrays (text, images, tool results).
type ContentPart struct {
    Type      string    `json:"type"`                  // "text"|"image_url"
    Text      string    `json:"text,omitempty"`        // for type="text"
    ImageURL  *ImageURL `json:"image_url,omitempty"`   // for type="image_url"
}

type ImageURL struct {
    URL    string `json:"url"`              // base64 data URI or HTTPS URL
    Detail string `json:"detail,omitempty"` // "auto"|"low"|"high"
}

// ToolCall represents an assistant's request to invoke a tool.
type ToolCall struct {
    Index    int          `json:"index,omitempty"` // streaming only: identifies which call
    ID       string       `json:"id,omitempty"`
    Type     string       `json:"type,omitempty"`  // "function"
    Function FunctionCall `json:"function"`
}

type FunctionCall struct {
    Name      string `json:"name,omitempty"`
    Arguments string `json:"arguments"`        // JSON string, accumulated incrementally
}

// ToolDefinition is an OpenAI-format tool for the tools array.
type ToolDefinition struct {
    Type     string      `json:"type"`     // "function"
    Function FunctionDef `json:"function"`
}

type FunctionDef struct {
    Name        string         `json:"name"`
    Description string         `json:"description"`
    Parameters  map[string]any `json:"parameters"` // JSON Schema object
}
```

### 5.3 Response Types (OpenAI Wire Format)

```go
// StreamChunk represents a single SSE chunk from LiteLLM.
type StreamChunk struct {
    ID                 string   `json:"id"`
    Object             string   `json:"object"`  // "chat.completion.chunk"
    Created            int64    `json:"created"`
    Model              string   `json:"model"`
    Choices            []Choice `json:"choices"`
    Usage              *Usage   `json:"usage,omitempty"`              // final chunk only (stream_options)
    SystemFingerprint  string   `json:"system_fingerprint,omitempty"`
}

type Choice struct {
    Index        int     `json:"index"`
    Delta        Delta   `json:"delta"`
    FinishReason *string `json:"finish_reason"` // null | "stop" | "tool_calls" | "length"
}

// Delta is the incremental content in a streaming chunk.
type Delta struct {
    Role             string     `json:"role,omitempty"`
    Content          *string    `json:"content,omitempty"`            // text content (nil vs "" matters)
    ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
    ReasoningContent *string    `json:"reasoning_content,omitempty"` // LiteLLM thinking passthrough
}

// Usage from the final streaming chunk or non-streaming response.
type Usage struct {
    PromptTokens              int `json:"prompt_tokens"`
    CompletionTokens          int `json:"completion_tokens"`
    TotalTokens               int `json:"total_tokens"`
    // Anthropic cache tokens (passed through by LiteLLM)
    CacheReadInputTokens      int `json:"cache_read_input_tokens,omitempty"`
    CacheCreationInputTokens  int `json:"cache_creation_input_tokens,omitempty"`
}
```

### 5.4 Internal Types (Anthropic-Equivalent)

These types mirror the Anthropic API and are used throughout the agentic loop, session storage, and transport. The LLM client translates from OpenAI wire format into these.

```go
// BetaMessage mirrors the Anthropic Messages API response object.
// This is the canonical internal representation after stream accumulation.
// Referenced by SDKAssistantMessage.Message (sdk.d.ts:1142).
type BetaMessage struct {
    ID           string         `json:"id"`            // "msg_xxx"
    Type         string         `json:"type"`          // "message"
    Role         string         `json:"role"`          // "assistant"
    Content      []ContentBlock `json:"content"`       // text, tool_use, thinking blocks
    Model        string         `json:"model"`
    StopReason   *string        `json:"stop_reason"`   // Anthropic stop_reason (translated from finish_reason)
    StopSequence *string        `json:"stop_sequence"`
    Usage        BetaUsage      `json:"usage"`
}

// ContentBlock is a discriminated union for message content.
// Type field determines which other fields are populated.
type ContentBlock struct {
    Type     string         `json:"type"`              // "text" | "tool_use" | "thinking"

    // type="text"
    Text     string         `json:"text,omitempty"`

    // type="tool_use"
    ID       string         `json:"id,omitempty"`      // "toolu_xxx" (Anthropic) or "call_xxx" (OpenAI)
    Name     string         `json:"name,omitempty"`
    Input    map[string]any `json:"input,omitempty"`   // Parsed JSON (not string)

    // type="thinking"
    Thinking string         `json:"thinking,omitempty"`
}

// BetaUsage mirrors Anthropic's usage object with cache token fields.
// All fields are non-optional (zero-valued if absent).
// Maps to NonNullableUsage (sdk.d.ts:429-430).
type BetaUsage struct {
    InputTokens              int `json:"input_tokens"`
    OutputTokens             int `json:"output_tokens"`
    CacheReadInputTokens     int `json:"cache_read_input_tokens"`
    CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}
```

---

## 6. Client Interface

```go
// Client is the LLM inference client. All methods are safe for concurrent use.
type Client interface {
    // Complete sends a streaming completion request and returns a Stream.
    // The caller MUST call Stream.Close() when done (or let Accumulate drain it).
    // ctx cancellation aborts the HTTP request and closes the stream.
    Complete(ctx context.Context, req *CompletionRequest) (*Stream, error)

    // Model returns the configured default model string.
    Model() string

    // SetModel changes the default model for subsequent requests.
    SetModel(model string)
}

// Stream represents an active SSE streaming response.
type Stream struct {
    // Next returns the next parsed StreamChunk, or io.EOF when done.
    // Returns context.Canceled if the parent context was cancelled.
    Next() (*StreamChunk, error)

    // Accumulate reads all remaining chunks and returns the fully assembled
    // CompletionResponse. Automatically calls Close() when done.
    // This is the primary API used by the agentic loop.
    Accumulate() (*CompletionResponse, error)

    // AccumulateWithCallback reads all chunks, calling cb for each chunk
    // before accumulation. Used to emit SDKPartialAssistantMessage events.
    AccumulateWithCallback(cb func(*StreamChunk)) (*CompletionResponse, error)

    // Close terminates the stream early and releases the HTTP connection.
    Close() error
}

// CompletionResponse is the accumulated result of a streaming (or sync) completion.
// The agentic loop uses this as the basis for constructing SDKAssistantMessage.
type CompletionResponse struct {
    ID           string         // Message ID (e.g. "chatcmpl-xxx")
    Model        string         // Actual model used (from response)
    Content      []ContentBlock // Accumulated content blocks (text, tool_use, thinking)
    ToolCalls    []ToolCall     // Extracted tool calls (OpenAI format, for reference)
    FinishReason string         // OpenAI finish_reason: "stop"|"tool_calls"|"length"
    StopReason   string         // Translated Anthropic stop_reason: "end_turn"|"tool_use"|"max_tokens"
    Usage        BetaUsage      // Token usage (translated to Anthropic format)
}
```

---

## 7. SSE Parser

### 7.1 Parser Interface

```go
// ParseSSEStream reads an HTTP response body line-by-line and yields StreamEvents.
// The returned channel is closed when the stream ends (either [DONE] or error).
func ParseSSEStream(ctx context.Context, body io.ReadCloser) <-chan StreamEvent

// StreamEvent wraps a parsed chunk or an error.
type StreamEvent struct {
    Chunk *StreamChunk // Non-nil on successful parse
    Err   error        // Non-nil on parse error or stream error
    Done  bool         // True when "data: [DONE]" received
}
```

### 7.2 SSE Line Parsing Rules

1. Read line-by-line from the response body (buffered, `\n` or `\r\n` delimiters)
2. Lines starting with `:` are SSE comments (keep-alive pings) — skip
3. Empty lines are event boundaries — skip
4. Lines starting with `data: ` contain the payload
5. `data: [DONE]` terminates the stream — emit `StreamEvent{Done: true}` and close
6. All other `data: ` lines contain JSON — unmarshal into `StreamChunk`
7. Malformed JSON should be logged and skipped (not fatal)
8. On `ctx.Done()`, close body and return `context.Canceled`

### 7.3 Tool Call Accumulator

Tool calls arrive as incremental deltas across multiple chunks. The `index` field within `delta.tool_calls[N]` identifies which call is being updated. The first chunk for a tool call carries `id` and `function.name`; subsequent chunks append to `function.arguments`.

```go
// ToolCallAccumulator collects incremental tool call deltas into complete ToolCalls.
type ToolCallAccumulator struct {
    calls    map[int]*ToolCall // index → accumulated call
    maxIndex int               // track highest index seen
}

func NewToolCallAccumulator() *ToolCallAccumulator {
    return &ToolCallAccumulator{calls: make(map[int]*ToolCall)}
}

// AddDelta merges an incremental tool call delta.
func (a *ToolCallAccumulator) AddDelta(delta ToolCall) {
    idx := delta.Index
    if idx > a.maxIndex {
        a.maxIndex = idx
    }
    existing, ok := a.calls[idx]
    if !ok {
        a.calls[idx] = &ToolCall{
            ID:       delta.ID,
            Type:     delta.Type,
            Function: FunctionCall{Name: delta.Function.Name},
        }
        existing = a.calls[idx]
    }
    // ID and Name only arrive on the first delta for this index
    if delta.ID != "" {
        existing.ID = delta.ID
    }
    if delta.Function.Name != "" {
        existing.Function.Name = delta.Function.Name
    }
    // Arguments are always appended
    existing.Function.Arguments += delta.Function.Arguments
}

// Complete returns all accumulated tool calls in index order.
func (a *ToolCallAccumulator) Complete() []ToolCall {
    result := make([]ToolCall, 0, len(a.calls))
    for i := 0; i <= a.maxIndex; i++ {
        if call, ok := a.calls[i]; ok {
            result = append(result, *call)
        }
    }
    return result
}
```

### 7.4 Stream Accumulation Algorithm

```
FUNCTION Accumulate(stream, callback) → CompletionResponse:
    textAccum    ← strings.Builder{}
    thinkAccum   ← strings.Builder{}
    toolAccum    ← NewToolCallAccumulator()
    var response CompletionResponse
    var usage    *Usage

    FOR EACH event IN stream:
        IF event.Err ≠ nil:
            RETURN error
        IF event.Done:
            BREAK

        chunk ← event.Chunk
        IF callback ≠ nil:
            callback(chunk)

        // Extract response metadata from first chunk
        IF response.ID == "":
            response.ID = chunk.ID
            response.Model = chunk.Model

        // Process usage (arrives in final chunk with stream_options)
        IF chunk.Usage ≠ nil:
            usage = chunk.Usage

        FOR EACH choice IN chunk.Choices:
            delta ← choice.Delta

            // Accumulate text content
            IF delta.Content ≠ nil:
                textAccum.WriteString(*delta.Content)

            // Accumulate thinking content (LiteLLM passthrough)
            IF delta.ReasoningContent ≠ nil:
                thinkAccum.WriteString(*delta.ReasoningContent)

            // Accumulate tool calls
            FOR EACH tc IN delta.ToolCalls:
                toolAccum.AddDelta(tc)

            // Capture finish reason
            IF choice.FinishReason ≠ nil:
                response.FinishReason = *choice.FinishReason

    // Build content blocks in order: thinking → text → tool_use
    IF thinkAccum.Len() > 0:
        response.Content = append(response.Content, ContentBlock{
            Type:     "thinking",
            Thinking: thinkAccum.String(),
        })
    IF textAccum.Len() > 0:
        response.Content = append(response.Content, ContentBlock{
            Type: "text",
            Text: textAccum.String(),
        })
    FOR EACH tc IN toolAccum.Complete():
        input ← json.Unmarshal(tc.Function.Arguments) // Parse arguments JSON string → map
        response.Content = append(response.Content, ContentBlock{
            Type:  "tool_use",
            ID:    tc.ID,
            Name:  tc.Function.Name,
            Input: input,
        })

    response.ToolCalls = toolAccum.Complete()
    response.StopReason = translateFinishReason(response.FinishReason)
    response.Usage = translateUsage(usage)
    RETURN response
```

---

## 8. Translation Functions

### 8.1 Finish Reason → Stop Reason

```go
// translateFinishReason converts OpenAI finish_reason to Anthropic stop_reason.
func translateFinishReason(fr string) string {
    switch fr {
    case "stop":
        return "end_turn"
    case "tool_calls":
        return "tool_use"
    case "length":
        return "max_tokens"
    default:
        return fr // pass through unknown values
    }
}
```

### 8.2 Usage Translation

```go
// translateUsage converts OpenAI Usage to Anthropic BetaUsage.
func translateUsage(u *Usage) BetaUsage {
    if u == nil {
        return BetaUsage{}
    }
    return BetaUsage{
        InputTokens:              u.PromptTokens,
        OutputTokens:             u.CompletionTokens,
        CacheReadInputTokens:     u.CacheReadInputTokens,
        CacheCreationInputTokens: u.CacheCreationInputTokens,
    }
}
```

### 8.3 Model ID Translation

```go
// toRequestModel prepends the LiteLLM provider prefix.
func toRequestModel(model string) string {
    if strings.HasPrefix(model, "anthropic/") {
        return model
    }
    return "anthropic/" + model
}

// fromResponseModel strips the LiteLLM provider prefix.
func fromResponseModel(model string) string {
    return strings.TrimPrefix(model, "anthropic/")
}
```

### 8.4 CompletionResponse → BetaMessage

```go
// ToBetaMessage converts the accumulated CompletionResponse to an Anthropic-equivalent BetaMessage.
// This is used by the agentic loop to construct SDKAssistantMessage.
func (r *CompletionResponse) ToBetaMessage() BetaMessage {
    stopReason := r.StopReason
    return BetaMessage{
        ID:         r.ID,
        Type:       "message",
        Role:       "assistant",
        Content:    r.Content,
        Model:      fromResponseModel(r.Model),
        StopReason: &stopReason,
        Usage:      r.Usage,
    }
}
```

---

## 9. Request Construction

The agentic loop calls `buildCompletionRequest()` before each LLM call. This assembles the full request from loop state.

### 9.1 Algorithm

```
FUNCTION buildCompletionRequest(config, systemPrompt, messages, tools, loopState) → CompletionRequest:
    req ← CompletionRequest{
        Model:         toRequestModel(config.Model),
        Stream:        true,
        MaxTokens:     config.MaxTokens,
        StreamOptions: &StreamOptions{IncludeUsage: true},
    }

    // System prompt as first message
    req.Messages = append(req.Messages, ChatMessage{
        Role:    "system",
        Content: systemPrompt,
    })

    // Convert internal messages to OpenAI format
    FOR EACH msg IN messages:
        req.Messages = append(req.Messages, convertToOpenAIMessage(msg))

    // Tool definitions
    FOR EACH tool IN tools:
        req.Tools = append(req.Tools, ToolDefinition{
            Type: "function",
            Function: FunctionDef{
                Name:        tool.Name(),
                Description: tool.Description(),
                Parameters:  tool.InputSchema(),
            },
        })

    // Anthropic-specific passthrough
    extraBody := map[string]any{}

    IF config.MaxThinkingTokens > 0:
        extraBody["thinking"] = map[string]any{
            "type":          "enabled",
            "budget_tokens": config.MaxThinkingTokens,
        }

    IF len(config.Betas) > 0:
        extraBody["betas"] = config.Betas

    IF loopState.SessionID != "":
        extraBody["metadata"] = map[string]any{
            "user_id": loopState.SessionID,
        }

    IF len(extraBody) > 0:
        req.ExtraBody = extraBody

    RETURN req
```

### 9.2 Message Conversion (Internal → OpenAI)

```
FUNCTION convertToOpenAIMessage(msg) → ChatMessage:
    SWITCH msg.Role:
        CASE "user":
            IF msg has tool_result content blocks:
                // Each tool_result becomes a separate "tool" message
                RETURN one ChatMessage per tool_result:
                    Role: "tool", ToolCallID: block.ToolUseID, Content: block.Content
            ELSE:
                RETURN ChatMessage{Role: "user", Content: msg.Content}

        CASE "assistant":
            cm ← ChatMessage{Role: "assistant"}
            // Separate text content from tool calls
            textParts ← extract text/thinking blocks → Content string
            toolCalls ← extract tool_use blocks → ToolCall array
                (each: ID=block.ID, Type="function",
                 Function={Name: block.Name, Arguments: json.Marshal(block.Input)})
            cm.Content = textParts (or nil if empty)
            cm.ToolCalls = toolCalls
            RETURN cm
```

---

## 10. Error Handling & Retry

### 10.1 Error Types

Mapping from HTTP status to `SDKAssistantMessageError` (sdk.d.ts:1149):

| HTTP Status | LiteLLM Scenario | SDK Error Type | Retryable |
|-------------|-------------------|----------------|-----------|
| 401 | Invalid API key | `authentication_failed` | No |
| 402 | Payment required | `billing_error` | No |
| 403 | Forbidden / policy | `billing_error` | No |
| 400 | Malformed request | `invalid_request` | No |
| 422 | Validation error | `invalid_request` | No |
| 429 | Rate limited | `rate_limit` | Yes |
| 529 | Anthropic overloaded | `rate_limit` | Yes |
| 500 | Internal server error | `server_error` | Yes |
| 502 | Bad gateway | `server_error` | Yes |
| 503 | Service unavailable | `server_error` | Yes |
| Other | Unknown | `unknown` | No |

### 10.2 Retry Algorithm

```
FUNCTION doWithRetry(ctx, config, makeRequest) → (response, error):
    FOR attempt ← 0; attempt ≤ config.Retry.MaxRetries; attempt++:
        IF attempt > 0:
            backoff ← config.Retry.InitialBackoff * (config.Retry.BackoffFactor ^ (attempt-1))
            backoff = min(backoff, config.Retry.MaxBackoff)
            jitter  ← backoff * config.Retry.JitterFraction * rand.Float64()
            sleep(backoff + jitter)  // respect ctx cancellation

        resp, err ← makeRequest(ctx)

        IF err == nil AND resp.StatusCode == 200:
            RETURN resp, nil

        // Check Retry-After header (429 responses)
        IF retryAfter ← resp.Header.Get("Retry-After"); retryAfter != "":
            sleep(parseRetryAfter(retryAfter))

        IF !isRetryable(resp.StatusCode, config.Retry.RetryableStatuses):
            RETURN nil, classifyError(resp)

    RETURN nil, ErrMaxRetriesExceeded{Attempts: config.Retry.MaxRetries + 1, LastStatus: lastStatus}
```

### 10.3 LLMError Type

```go
// LLMError wraps HTTP-level errors from the LLM API with SDK classification.
type LLMError struct {
    StatusCode int
    SDKError   string // SDKAssistantMessageError value
    Message    string // Error message from response body
    Retryable  bool
    RetryAfter time.Duration // From Retry-After header, if present
}

func (e *LLMError) Error() string {
    return fmt.Sprintf("llm: %s (HTTP %d): %s", e.SDKError, e.StatusCode, e.Message)
}
```

---

## 11. Cost Tracking

The client tracks per-request and cumulative costs for budget enforcement (Options.maxBudgetUsd, sdk.d.ts:625).

### 11.1 Model Pricing Table

```go
// ModelPricing holds per-model token costs. Updated as models change.
type ModelPricing struct {
    InputPerMTok       float64 // USD per 1M input tokens
    OutputPerMTok      float64 // USD per 1M output tokens
    CacheReadPerMTok   float64 // USD per 1M cache-read tokens
    CacheCreatePerMTok float64 // USD per 1M cache-creation tokens
}

// DefaultPricing for known models (as of 2025-06).
var DefaultPricing = map[string]ModelPricing{
    "claude-opus-4-5-20250514":     {InputPerMTok: 15.0, OutputPerMTok: 75.0, CacheReadPerMTok: 1.50, CacheCreatePerMTok: 18.75},
    "claude-sonnet-4-5-20250929":   {InputPerMTok: 3.0,  OutputPerMTok: 15.0, CacheReadPerMTok: 0.30, CacheCreatePerMTok: 3.75},
    "claude-haiku-4-5-20251001":    {InputPerMTok: 0.80, OutputPerMTok: 4.0,  CacheReadPerMTok: 0.08, CacheCreatePerMTok: 1.0},
}
```

### 11.2 Cost Calculator

```go
// CalculateCost computes the USD cost for a single API response.
func CalculateCost(model string, usage BetaUsage) float64 {
    pricing, ok := DefaultPricing[model]
    if !ok {
        return 0 // Unknown model, can't calculate
    }
    cost := float64(usage.InputTokens) * pricing.InputPerMTok / 1_000_000
    cost += float64(usage.OutputTokens) * pricing.OutputPerMTok / 1_000_000
    cost += float64(usage.CacheReadInputTokens) * pricing.CacheReadPerMTok / 1_000_000
    cost += float64(usage.CacheCreationInputTokens) * pricing.CacheCreatePerMTok / 1_000_000
    return cost
}
```

### 11.3 Cost Tracker (Cumulative)

```go
// CostTracker accumulates costs across multiple requests for budget enforcement.
// Safe for concurrent use.
type CostTracker struct {
    mu         sync.Mutex
    totalCost  float64
    modelUsage map[string]*ModelUsageAccum // per-model accumulation
}

type ModelUsageAccum struct {
    InputTokens              int
    OutputTokens             int
    CacheReadInputTokens     int
    CacheCreationInputTokens int
    CostUSD                  float64
}

// Add records usage from a single API response.
func (ct *CostTracker) Add(model string, usage BetaUsage) float64 {
    cost := CalculateCost(model, usage)
    ct.mu.Lock()
    defer ct.mu.Unlock()
    ct.totalCost += cost
    // ... accumulate per-model
    return ct.totalCost
}

// TotalCost returns the cumulative cost in USD.
func (ct *CostTracker) TotalCost() float64 {
    ct.mu.Lock()
    defer ct.mu.Unlock()
    return ct.totalCost
}
```

---

## 12. Mapping to SDK Emission Types

The agentic loop wraps client output into SDK message types for transport.

### 12.1 Streaming → SDKPartialAssistantMessage

Each `StreamChunk` from the callback is wrapped into the equivalent of `SDKPartialAssistantMessage` (sdk.d.ts:1373-1379). Since we use OpenAI format, we synthesize a `BetaRawMessageStreamEvent`-equivalent:

```go
// EmitStreamEvent wraps a StreamChunk as an SDKPartialAssistantMessage-equivalent.
// The agentic loop calls this from the AccumulateWithCallback handler.
func EmitStreamEvent(chunk *StreamChunk, parentToolUseID *string, sessionID string) types.PartialAssistantMessage {
    return types.PartialAssistantMessage{
        Type:            types.MessageTypeStreamEvent,
        Event:           chunkToStreamEvent(chunk), // Convert to Anthropic-equivalent event
        ParentToolUseID: parentToolUseID,
        UUID:            uuid.New(),
        SessionID:       sessionID,
    }
}
```

### 12.2 Accumulated → SDKAssistantMessage

After `Accumulate()` completes, the response is wrapped into `SDKAssistantMessage` (sdk.d.ts:1140-1147):

```go
// EmitAssistantMessage wraps a CompletionResponse as an SDKAssistantMessage.
func EmitAssistantMessage(resp *CompletionResponse, parentToolUseID *string, sessionID string, err *string) types.AssistantMessage {
    return types.AssistantMessage{
        Type:            types.MessageTypeAssistant,
        Message:         resp.ToBetaMessage(),
        ParentToolUseID: parentToolUseID,
        Error:           err, // nil on success, SDKAssistantMessageError on failure
        UUID:            uuid.New(),
        SessionID:       sessionID,
    }
}
```

---

## 13. Verification Checklist

- [ ] **Streaming parity**: Given identical prompt + tools, Go client produces same tool_calls and text content as TS SDK `query()` output
- [ ] **Tool call accumulation**: When model returns 3+ tool_use blocks, all are correctly accumulated from incremental deltas with correct index ordering
- [ ] **Tool call ID mapping**: LiteLLM `call_xxx` IDs correctly preserved through round-trip (request → response → tool result → next request)
- [ ] **Arguments parsing**: Accumulated `arguments` JSON string correctly unmarshaled into `ContentBlock.Input` map — handles escaped quotes, unicode, nested objects
- [ ] **Usage tracking**: `BetaUsage` from `stream_options.include_usage` matches expected values including cache token fields
- [ ] **Cost calculation**: `CalculateCost` produces correct USD values for all model tiers; `CostTracker` accumulates correctly across concurrent requests
- [ ] **Finish reason translation**: `"stop"` → `"end_turn"`, `"tool_calls"` → `"tool_use"`, `"length"` → `"max_tokens"`
- [ ] **Error classification**: HTTP 429 → `rate_limit`, HTTP 401 → `authentication_failed`, HTTP 500 → `server_error`, etc.
- [ ] **Retry behavior**: 429 respects `Retry-After` header; exponential backoff with jitter; non-retryable codes (400, 401, 402) fail immediately
- [ ] **SSE parser robustness**: Handles keep-alive pings (`:` lines), empty lines, malformed JSON (skip, don't crash), `[DONE]` termination
- [ ] **Thinking mode**: When `extra_body.thinking` is set, `reasoning_content` deltas are accumulated into a `ContentBlock{Type:"thinking"}` in the response
- [ ] **LiteLLM extra_body**: `thinking`, `betas`, and `metadata` fields correctly serialized in `extra_body` and not duplicated at top level
- [ ] **Model prefix**: Requests use `anthropic/` prefix; responses strip it for internal types
- [ ] **Context cancellation**: `ctx.Done()` during streaming closes the HTTP body and returns `context.Canceled`
- [ ] **Concurrent safety**: `Client` and `CostTracker` are safe for concurrent use from multiple goroutines
- [ ] **BetaMessage construction**: `ToBetaMessage()` produces JSON-serializable output identical to Anthropic native format (validated by golden file tests)
