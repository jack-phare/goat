# LLM Client — SSE Streaming & Accumulation

> `pkg/llm/` — The inference layer. Handles SSE parsing, stream accumulation,
> tool call delta merging, cost tracking, and retry logic.

## How Goat Talks to LLMs (vs Claude Code TS)

Claude Code TS calls the Anthropic API directly using the Anthropic SDK.
Goat routes through a LiteLLM proxy (OpenAI-compatible `/v1/chat/completions`),
which means all LLM communication uses the OpenAI streaming format.

```
┌───────────────────────────────────────────────────────────────────┐
│                    Claude Code TS                                  │
│                                                                   │
│  Agent Loop ──▶ Anthropic SDK ──▶ api.anthropic.com               │
│                 (native Messages API format)                      │
│                 (SSE: message_start, content_block_delta, etc.)   │
└───────────────────────────────────────────────────────────────────┘

┌───────────────────────────────────────────────────────────────────┐
│                    Goat                                            │
│                                                                   │
│  Agent Loop ──▶ pkg/llm Client ──▶ LiteLLM Proxy ──▶ Any LLM    │
│                 (OpenAI chat format)                              │
│                 (SSE: data: {choices:[{delta:{...}}]})            │
│                                                                   │
│  Key difference: OpenAI format uses "choices[0].delta" with      │
│  incremental tool_calls and content fields, not Anthropic's      │
│  content_block events. Goat's accumulator handles this.          │
└───────────────────────────────────────────────────────────────────┘
```

## SSE Stream Processing Pipeline

```
 HTTP Response Body (io.ReadCloser)
         │
         ▼
 ┌───────────────────────┐
 │  ParseSSEStream()     │
 │                       │
 │  bufio.Scanner line   │
 │  by line:             │
 │                       │
 │  "data: {...}"        │──▶ JSON unmarshal → StreamChunk
 │  "data: [DONE]"       │──▶ close channel
 │  "" (empty line)      │──▶ skip
 │  ": comment"          │──▶ skip
 │                       │
 │  On EOF or ctx.Done:  │
 │    close channel      │
 └──────────┬────────────┘
            │
   chan StreamEvent (buffered)
            │
            ▼
 ┌───────────────────────┐
 │      Stream           │
 │                       │
 │  events <-chan        │
 │  body   io.ReadCloser │
 │  cancel func()        │
 │                       │
 │  Two consumption      │
 │  modes:               │
 │                       │
 │  • Accumulate()       │──▶ Full CompletionResponse
 │  • AccumulateWith     │
 │    Callback(fn)       │──▶ fn(chunk) per event +
 │                       │    Full CompletionResponse
 └───────────────────────┘
```

## StreamChunk → CompletionResponse Accumulation

The accumulator builds up a full response from incremental SSE chunks.
This is the trickiest part of the LLM client.

```
 Chunk 1: {id:"msg-1", model:"claude", choices:[{delta:{role:"assistant",
           reasoning_content:"Let me think..."}}]}
         │
         ▼
 ┌──────────────────────────────────────────────────────────────────┐
 │                        Accumulator State                         │
 │                                                                  │
 │  ID:           "msg-1"                                          │
 │  Model:        "claude" (anthropic/ prefix stripped)            │
 │  Text:         ""                                                │
 │  Thinking:     "Let me think..."  ◄── reasoning_content delta   │
 │  ToolCalls:    ToolCallAccumulator (empty)                      │
 │  FinishReason: ""                                                │
 │  Usage:        nil                                               │
 └──────────────────────────────────────────────────────────────────┘

 Chunk 2: {choices:[{delta:{reasoning_content:" about this."}}]}
         │
         ▼
 ┌──────────────────────────────────────────────────────────────────┐
 │  Thinking:     "Let me think... about this."  ◄── appended      │
 └──────────────────────────────────────────────────────────────────┘

 Chunk 3: {choices:[{delta:{content:"I'll run a command."}}]}
         │
         ▼
 ┌──────────────────────────────────────────────────────────────────┐
 │  Text:         "I'll run a command."  ◄── content delta         │
 └──────────────────────────────────────────────────────────────────┘

 Chunk 4: {choices:[{delta:{tool_calls:[{index:0, id:"call_1",
           type:"function", function:{name:"Bash", arguments:""}}]}}]}
         │
         ▼
 ┌──────────────────────────────────────────────────────────────────┐
 │  ToolCalls:    [0] → {ID:"call_1", Name:"Bash", Args:""}       │
 └──────────────────────────────────────────────────────────────────┘

 Chunk 5: {choices:[{delta:{tool_calls:[{index:0,
           function:{arguments:'{"command": "ls"}'}}]}}]}
         │
         ▼
 ┌──────────────────────────────────────────────────────────────────┐
 │  ToolCalls:    [0] → {ID:"call_1", Name:"Bash",                │
 │                        Args:'{"command": "ls"}'}  ◄── appended  │
 └──────────────────────────────────────────────────────────────────┘

 Chunk 6: {choices:[{finish_reason:"tool_calls"}],
           usage:{prompt_tokens:200, completion_tokens:80}}
         │
         ▼
 ┌──────────────────────────────────────────────────────────────────┐
 │  FINAL CompletionResponse:                                      │
 │                                                                  │
 │  ID:           "msg-1"                                          │
 │  Model:        "claude"                                          │
 │  FinishReason: "tool_calls"                                     │
 │  StopReason:   "tool_use"    ◄── mapped from finish_reason     │
 │  Content: [                                                      │
 │    {Type:"thinking", Thinking:"Let me think... about this."},   │
 │    {Type:"text",     Text:"I'll run a command."},               │
 │    {Type:"tool_use", ID:"call_1", Name:"Bash",                 │
 │                      Input:{"command":"ls"}}                    │
 │  ]                                                               │
 │  Usage: {InputTokens:200, OutputTokens:80}                      │
 └──────────────────────────────────────────────────────────────────┘

  Content block ordering is ALWAYS: thinking → text → tool_use
  This matches Claude Code TS behavior.
```

## ToolCallAccumulator — Handling Sparse & Interleaved Deltas

The OpenAI streaming format sends tool call deltas with index numbers.
Deltas can arrive interleaved across multiple tool calls, and indices
can be sparse (e.g., 0, 3 with no 1 or 2).

```
 Example: Interleaved deltas for 2 tool calls

 Delta 1: {index:0, id:"call_1", function:{name:"Bash"}}
 Delta 2: {index:1, id:"call_2", function:{name:"Read"}}
 Delta 3: {index:0, function:{arguments:'{"cmd":'}}
 Delta 4: {index:1, function:{arguments:'{"path":'}}
 Delta 5: {index:0, function:{arguments:'"ls"}'}}
 Delta 6: {index:1, function:{arguments:'"f.go"}'}}

 ┌──────────────────────────────────────────────────────────────┐
 │              ToolCallAccumulator                              │
 │                                                              │
 │  Internal: map[int]*toolCallEntry                            │
 │  (index → accumulating tool call)                            │
 │                                                              │
 │  AddDelta(tc ToolCall):                                      │
 │    entry = getOrCreate(tc.Index)                             │
 │    if tc.ID != "":       entry.ID = tc.ID                   │
 │    if tc.Type != "":     entry.Type = tc.Type               │
 │    if tc.Function.Name:  entry.Name += tc.Function.Name     │
 │    entry.Args += tc.Function.Arguments  ◄── string concat   │
 │                                                              │
 │  Complete() []ToolCall:                                      │
 │    Sort entries by index                                     │
 │    Return as []ToolCall                                      │
 │                                                              │
 │  After deltas above:                                         │
 │    [0] → {ID:"call_1", Name:"Bash", Args:'{"cmd":"ls"}'}   │
 │    [1] → {ID:"call_2", Name:"Read", Args:'{"path":"f.go"}'}│
 └──────────────────────────────────────────────────────────────┘
```

## Finish Reason → Stop Reason Mapping

```
 OpenAI finish_reason    Goat StopReason     Claude Code equivalent
 ─────────────────────   ─────────────────   ──────────────────────
 "stop"                  "end_turn"          end_turn
 "tool_calls"            "tool_use"          tool_use
 "length"                "max_tokens"        max_tokens
 "content_filter"        "content_filter"    (N/A in Anthropic)
 "stop_sequence"         "stop_sequence"     stop_sequence (beta)
```

## Model ID Handling

```
 Request flow:
   config.Model = "claude-sonnet-4-5-20250929"
         │
         ▼
   BuildCompletionRequest adds "anthropic/" prefix
         │
         ▼
   LiteLLM receives "anthropic/claude-sonnet-4-5-20250929"
         │
         ▼
   Routes to correct provider


 Response flow:
   SSE chunk: model = "anthropic/claude-sonnet-4-5-20250929"
         │
         ▼
   Accumulator strips "anthropic/" prefix
         │
         ▼
   resp.Model = "claude-sonnet-4-5-20250929"
```

## Retry Mechanism

```
 doWithRetry(ctx, config, httpFunc)
         │
         ▼
 ┌─────────────────────────────────────────┐
 │  attempt = 0                             │
 │  backoff = config.InitialBackoff         │
 │                                          │
 │  LOOP:                                   │
 │    resp, err = httpFunc(ctx)             │
 │    attempt++                             │
 │                                          │
 │    if err != nil:                        │
 │      return nil, err                     │
 │                                          │
 │    if resp.StatusCode in RetryableSet:   │
 │      ├── 429, 500, 502, 503, 529        │
 │      │                                   │
 │      │  if attempt > MaxRetries:         │
 │      │    return ErrMaxRetriesExceeded   │
 │      │                                   │
 │      │  if StatusCode == 429:            │
 │      │    check Retry-After header       │
 │      │    use max(backoff, retryAfter)   │
 │      │                                   │
 │      │  sleep(backoff + jitter)          │
 │      │  backoff *= BackoffFactor         │
 │      │  backoff = min(backoff, MaxBack)  │
 │      │  continue LOOP                    │
 │      │                                   │
 │    else:                                 │
 │      return resp, nil                    │
 │                                          │
 │  Default config:                         │
 │    MaxRetries: 3                         │
 │    InitialBackoff: 1s                    │
 │    MaxBackoff: 30s                       │
 │    BackoffFactor: 2.0                    │
 │    JitterFraction: 0.1                   │
 │    RetryableStatuses: [429,500,502,503]  │
 └─────────────────────────────────────────┘
```

## Dynamic Pricing — FetchPricing from LiteLLM Proxy

The hardcoded `DefaultPricing` map only covers 3 Claude models. For all other
models (gpt-5-nano, gpt-5-mini, llama-3.3-70b, etc.), `FetchPricing()` pulls
per-model pricing from the LiteLLM `/model/info` endpoint at startup.

```
 ┌──────────────────────────────────────────────────────────────────────┐
 │  FetchPricing(ctx, baseURL, apiKey)                                  │
 │                                                                      │
 │  1. Derive URL:                                                      │
 │     baseURL = "http://localhost:4000/v1"                             │
 │              → strip "/v1" suffix                                    │
 │              → "http://localhost:4000/model/info"                    │
 │                                                                      │
 │  2. GET /model/info (10s timeout, Bearer auth)                       │
 │                                                                      │
 │  3. Parse response:                                                  │
 │     { "data": [                                                      │
 │         { "model_name": "gpt-5-nano",                                │
 │           "model_info": {                                            │
 │             "input_cost_per_token": 1.1e-07,    ◄── per token       │
 │             "output_cost_per_token": 4.4e-07,                       │
 │             "cache_read_input_token_cost": 5.5e-08,                 │
 │             "cache_creation_input_token_cost": 1.375e-07            │
 │           }                                                          │
 │         }, ...                                                       │
 │     ]}                                                               │
 │                                                                      │
 │  4. Convert per-token → per-million-token:                           │
 │     InputPerMTok = input_cost_per_token × 1,000,000                 │
 │     e.g. 1.1e-07 × 1M = $0.11 per MTok                             │
 │                                                                      │
 │  5. Merge into DefaultPricing via SetPricing():                      │
 │     - Overwrites matching model names                                │
 │     - Skips models with no pricing data (all zeros)                  │
 │     - Hardcoded Claude entries preserved for unmatched models        │
 │                                                                      │
 │  GRACEFUL DEGRADATION:                                               │
 │  If proxy is down, auth fails, or response is malformed:             │
 │    → returns error (caller logs warning)                             │
 │    → continues with hardcoded pricing                                │
 │    → never blocks startup                                            │
 └──────────────────────────────────────────────────────────────────────┘
```

## DefaultPricing — Thread-Safe Access

```
 ┌──────────────────────────────────────────────────────────────────────┐
 │  DefaultPricing map[string]ModelPricing                              │
 │  pricingMu     sync.RWMutex                                         │
 │                                                                      │
 │  Hardcoded fallbacks (always present):                               │
 │    claude-opus-4-5-20250514:    $15.00 / $75.00 per MTok            │
 │    claude-sonnet-4-5-20250929:  $3.00  / $15.00 per MTok            │
 │    claude-haiku-4-5-20251001:   $0.80  / $4.00  per MTok            │
 │                                                                      │
 │  GetPricing(model) → (ModelPricing, bool)    RLock                  │
 │  SetPricing(model, pricing)                  Lock                    │
 │  CalculateCost(model, usage)                 calls GetPricing        │
 │                                                                      │
 │  At startup, FetchPricing merges proxy data → SetPricing()          │
 │  All access after init is through GetPricing (read-locked).         │
 └──────────────────────────────────────────────────────────────────────┘
```

## CostTracker — Thread-Safe Cost Accounting

```
 ┌─────────────────────────────────────────────────────────┐
 │                    CostTracker                           │
 │                                                         │
 │  mu sync.Mutex  (protects all fields)                   │
 │                                                         │
 │  totalCost    float64   // USD accumulated              │
 │  modelUsage   map[string]*ModelUsageAccum               │
 │                                                         │
 │  Add(model, usage) float64:                             │
 │    cost = CalculateCost(model, usage)                   │
 │    mu.Lock()                                            │
 │    totalCost += cost                                    │
 │    accum[model].InputTokens += ...                      │
 │    mu.Unlock()                                          │
 │    return totalCost                                     │
 │                                                         │
 │  Thread-safe for concurrent goroutines                  │
 │  (subagents running in parallel)                        │
 └─────────────────────────────────────────────────────────┘
```

## Request Construction

```go
BuildCompletionRequest(model, maxTokens, maxThinking, system, messages, tools, sessionID)
    │
    ▼
CompletionRequest {
    Model:       "anthropic/" + model,       // prefix for LiteLLM
    MaxTokens:   16384,                       // default
    System:      systemPrompt,                // assembled by Prompter
    Messages:    []ChatMessage{...},          // conversation history
    Tools:       []LLMTool{...},              // from Registry
    Stream:      true,                        // always streaming
    SessionID:   "session-uuid",
}
```

## ConvertToToolMessages

Converts loop's `[]ToolResult` into `[]ChatMessage` for the next LLM call:

```
 Input:  []ToolResult{
           {ToolUseID: "call_1", Content: "hello world"},
           {ToolUseID: "call_2", Content: "Error: not found"},
         }
         │
         ▼
 Output: []ChatMessage{
           {Role: "tool", ToolCallID: "call_1", Content: "hello world"},
           {Role: "tool", ToolCallID: "call_2", Content: "Error: not found"},
         }
```
