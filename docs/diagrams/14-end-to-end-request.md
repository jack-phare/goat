# End-to-End Request Lifecycle

> A complete trace of a single user request from input to final result,
> showing every component interaction.

## Scenario: User asks "Read main.go and tell me what it does"

```
 ┌─────────────────────────────────────────────────────────────────────┐
 │  PHASE 1: INPUT                                                     │
 │                                                                     │
 │  Consumer sends: "Read main.go and tell me what it does"           │
 │       │                                                             │
 │       ▼                                                             │
 │  Transport.ReadMessages() → Router → query.SendUserMessage()       │
 │       │                                                             │
 │       ▼                                                             │
 │  RunLoop() receives on inputCh (or initial prompt)                 │
 └──────────────────────────────────────┬──────────────────────────────┘
                                        │
 ┌──────────────────────────────────────▼──────────────────────────────┐
 │  PHASE 2: INITIALIZATION (first iteration only)                     │
 │                                                                     │
 │  1. Session: store.Create({sessionID, model, cwd})                 │
 │  2. Hooks: Fire(SessionStart, {source:"startup"})                  │
 │  3. Emit: SystemInitMessage → channel → transport → consumer      │
 │  4. Prompt: assembler.Assemble(config)                             │
 │     ├── Load system-prompt.md (embedded)                           │
 │     ├── Append tool instructions                                    │
 │     ├── Append git context                                         │
 │     ├── Append CLAUDE.md content                                   │
 │     ├── Interpolate ${CWD}, ${MODEL}, ${DATE}                     │
 │     └── Result: ~50KB system prompt string                         │
 │  5. User msg → state.Messages, persist to JSONL                    │
 └──────────────────────────────────────┬──────────────────────────────┘
                                        │
 ┌──────────────────────────────────────▼──────────────────────────────┐
 │  PHASE 3: FIRST LLM CALL                                           │
 │                                                                     │
 │  1. Check termination: ctx OK, turns < max, budget < max           │
 │  2. Proactive compaction: utilization < 80% → skip                 │
 │  3. Inject hook context: none pending → skip                       │
 │  4. Build request:                                                  │
 │     CompletionRequest{                                              │
 │       Model: "anthropic/claude-sonnet-4-5-20250929",               │
 │       MaxTokens: 16384,                                            │
 │       System: assembled_prompt,                                     │
 │       Messages: [{role:"user", content:"Read main.go..."}],        │
 │       Tools: [Read, Glob, Grep, Bash, Write, Edit, ...],          │
 │       Stream: true,                                                 │
 │     }                                                               │
 │  5. HTTP POST to LiteLLM proxy                                     │
 │  6. SSE stream begins:                                              │
 │     data: {choices:[{delta:{content:"I'll read"}}]}                │
 │     data: {choices:[{delta:{content:" the file."}}]}               │
 │     data: {choices:[{delta:{tool_calls:[{index:0,                  │
 │            id:"call_1", function:{name:"Read",                     │
 │            arguments:'{"file_path":"main.go"}'}}]}}]}              │
 │     data: {choices:[{finish_reason:"tool_calls"}]}                 │
 │     data: [DONE]                                                    │
 │  7. Accumulate:                                                     │
 │     CompletionResponse{                                             │
 │       StopReason: "tool_use",                                      │
 │       Content: [                                                    │
 │         {Type:"text", Text:"I'll read the file."},                │
 │         {Type:"tool_use", ID:"call_1", Name:"Read",               │
 │          Input:{file_path:"main.go"}}                              │
 │       ]                                                             │
 │     }                                                               │
 │  8. Update state: TurnCount=1, accumulate usage, cost              │
 │  9. Emit: AssistantMessage → channel → transport → consumer       │
 │ 10. Persist assistant message to JSONL                              │
 └──────────────────────────────────────┬──────────────────────────────┘
                                        │
 ┌──────────────────────────────────────▼──────────────────────────────┐
 │  PHASE 4: TOOL EXECUTION                                            │
 │                                                                     │
 │  StopReason = "tool_use" → extract tool blocks                     │
 │                                                                     │
 │  For tool "Read" with input {file_path:"main.go"}:                 │
 │                                                                     │
 │  1. Registry lookup: tools.Get("Read") → FileReadTool              │
 │                                                                     │
 │  2. Permission check:                                               │
 │     Checker.Check(ctx, "Read", {file_path:"main.go"})              │
 │     ├── Layer 1 (mode): default → fall through                     │
 │     ├── Layer 2 (disabled): no → fall through                      │
 │     ├── Layer 3 (allowed): "Read" in allowedTools → ALLOW!        │
 │     └── Result: {Behavior:"allow"}                                  │
 │                                                                     │
 │  3. PreToolUse hook:                                                │
 │     hooks.Fire(PreToolUse, {tool_name:"Read", tool_input:...})     │
 │     → no hooks registered → empty results                          │
 │                                                                     │
 │  4. Emit: ToolProgressMessage (start, elapsed=0)                   │
 │                                                                     │
 │  5. Execute:                                                        │
 │     FileReadTool.Execute(ctx, {file_path:"main.go"})               │
 │     → Read file, add line numbers                                   │
 │     → ToolOutput{Content: "  1 | package main\n  2 | ..."}        │
 │                                                                     │
 │  6. Emit: ToolProgressMessage (end, elapsed=0.003s)                │
 │                                                                     │
 │  7. PostToolUse hook:                                               │
 │     hooks.Fire(PostToolUse, {tool_name:"Read", tool_response:...}) │
 │     → no hooks → empty results → no suppression                    │
 │                                                                     │
 │  8. Result: ToolResult{ToolUseID:"call_1", Content:"1|package..."}│
 │                                                                     │
 │  9. Convert to tool message:                                        │
 │     ChatMessage{Role:"tool", ToolCallID:"call_1", Content:"..."}  │
 │                                                                     │
 │ 10. Append to state.Messages, persist to JSONL                     │
 └──────────────────────────────────────┬──────────────────────────────┘
                                        │
 ┌──────────────────────────────────────▼──────────────────────────────┐
 │  PHASE 5: SECOND LLM CALL                                          │
 │                                                                     │
 │  1. Loop continues (not interrupted)                                │
 │  2. Build request with updated Messages:                            │
 │     [{role:"user", content:"Read main.go..."},                     │
 │      {role:"assistant", content:"...", tool_calls:[...]},          │
 │      {role:"tool", tool_call_id:"call_1", content:"1|package..."}]│
 │  3. LLM sees file contents, generates explanation                   │
 │  4. SSE stream:                                                     │
 │     "This is a Go main package that..."                            │
 │     finish_reason: "stop"                                           │
 │  5. Accumulate → StopReason: "end_turn"                            │
 │  6. TurnCount=2, update cost                                       │
 │  7. Emit: AssistantMessage                                         │
 │  8. Persist to JSONL                                                │
 └──────────────────────────────────────┬──────────────────────────────┘
                                        │
 ┌──────────────────────────────────────▼──────────────────────────────┐
 │  PHASE 6: TERMINATION                                               │
 │                                                                     │
 │  StopReason = "end_turn":                                           │
 │  1. Fire Stop hook → no hooks → shouldContinue = false             │
 │  2. MultiTurn mode?                                                 │
 │     ├── yes: emit TurnResult, waitForInput()                       │
 │     └── no:  ExitReason = ExitEndTurn                              │
 │  3. Finalize session:                                               │
 │     store.UpdateMetadata({TurnCount:2, Cost:0.012, Exit:end_turn}) │
 │  4. Emit: ResultMessage{                                           │
 │       Subtype: "success",                                           │
 │       Result: "This is a Go main package that...",                 │
 │       TurnCount: 2,                                                 │
 │       TotalCostUSD: 0.012,                                         │
 │     }                                                               │
 │  5. Fire SessionEnd hook                                            │
 │  6. close(ch) → consumer sees channel close                        │
 │  7. close(done) → query.Wait() unblocks                           │
 │  8. Transport receives final messages, delivers to consumer        │
 └─────────────────────────────────────────────────────────────────────┘
```

## Sequence Diagram (Condensed)

```
 Consumer    Transport    Router    RunLoop    LLM     Registry   Perms   Hooks   Session
    │            │          │         │         │         │         │       │        │
    │──"Read..."─▶          │         │         │         │         │       │        │
    │            │──msg──▶  │         │         │         │         │       │        │
    │            │          │──input──▶         │         │         │       │        │
    │            │          │         │         │         │         │       │        │
    │            │          │         ├──Create──┼─────────┼─────────┼───────┼──────▶│
    │            │          │         ├─SessionStart──────┼─────────┼───────▶       │
    │            │          │         │         │         │         │       │        │
    │◀───SystemInit─────────┤◀────────┤         │         │         │       │        │
    │            │          │         │         │         │         │       │        │
    │            │          │         ├─Build───▶         │         │       │        │
    │            │          │         │  req    │         │         │       │        │
    │            │          │         │◀─stream─┤         │         │       │        │
    │            │          │         │◀─chunks─┤         │         │       │        │
    │            │          │         │◀─[DONE]─┤         │         │       │        │
    │            │          │         │         │         │         │       │        │
    │◀──AssistantMsg────────┤◀────────┤         │         │         │       │        │
    │            │          │         ├─────────┼──Get────▶         │       │        │
    │            │          │         ├─────────┼─────────┼──Check──▶       │        │
    │            │          │         │◀────────┼─────────┼──allow──┤       │        │
    │            │          │         ├─────────┼─────────┼─────────┼─PreTU─▶       │
    │◀──ToolProgress────────┤◀────────┤         │         │         │       │        │
    │            │          │         ├─────────┼─Execute──▶        │       │        │
    │            │          │         │◀────────┼─result───┤        │       │        │
    │◀──ToolProgress────────┤◀────────┤         │         │         │       │        │
    │            │          │         ├─────────┼─────────┼─────────┼─PostTU▶       │
    │            │          │         ├──Persist─┼─────────┼─────────┼───────┼──────▶│
    │            │          │         │         │         │         │       │        │
    │            │          │         ├─Build───▶         │         │       │        │
    │            │          │         │  req    │         │         │       │        │
    │            │          │         │◀─stream─┤         │         │       │        │
    │            │          │         │◀─[DONE]─┤         │         │       │        │
    │            │          │         │         │         │         │       │        │
    │◀──AssistantMsg────────┤◀────────┤         │         │         │       │        │
    │            │          │         ├─Stop────┼─────────┼─────────┼──────▶│        │
    │            │          │         ├─Finalize┼─────────┼─────────┼───────┼──────▶│
    │◀──ResultMsg───────────┤◀────────┤         │         │         │       │        │
    │            │          │         ├─SessionEnd────────┼─────────┼──────▶│        │
    │            │          │         │ close   │         │         │       │        │
    │            │          │         │         │         │         │       │        │
```

## Component Touch Count

For this single "Read main.go" request, here's how many times each component is invoked:

```
 ┌──────────────────────┬────────┬───────────────────────────────────┐
 │ Component            │ Calls  │ What it did                       │
 ├──────────────────────┼────────┼───────────────────────────────────┤
 │ Transport            │   ~8   │ Relay input + 6 output messages  │
 │ Router               │   ~8   │ Bridge each message               │
 │ RunLoop              │    2   │ 2 LLM call iterations            │
 │ LLM Client           │    2   │ 2 streaming completions          │
 │ SSE Parser           │   ~12  │ Parse ~6 chunks per call         │
 │ Accumulator          │    2   │ Build 2 CompletionResponses      │
 │ Tool Registry        │    1   │ 1 lookup (Read)                  │
 │ FileReadTool         │    1   │ 1 file read                      │
 │ Permission Checker   │    1   │ 1 check (auto-allowed)           │
 │ Hook Runner          │    4   │ SessionStart, PreTU, PostTU, Stop│
 │ Prompt Assembler     │    1   │ 1 assembly (cached for session)  │
 │ Session Store        │    5   │ Create, 3 persists, finalize     │
 │ Cost Tracker         │    2   │ 2 cost accumulations             │
 │ Context Compactor    │    0   │ Not needed (small context)        │
 └──────────────────────┴────────┴───────────────────────────────────┘
```
