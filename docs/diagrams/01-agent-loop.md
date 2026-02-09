# Agent Loop — State Machine & Lifecycle

> `pkg/agent/` — The heart of Goat. A channel-based agentic loop that drives
> LLM inference, tool execution, and multi-turn conversation.

## State Machine

The loop is a `for {}` state machine running in a goroutine. It doesn't use
explicit phase enums — instead, control flows through branches on `StopReason`.

```
                    ┌──────────────────┐
                    │   RunLoop()      │
                    │   goroutine      │
                    │   starts here    │
                    └────────┬─────────┘
                             │
                    ┌────────▼─────────┐
                    │   INITIALIZE     │
                    │                  │
                    │ 1. Restore/create│
                    │    session       │
                    │ 2. Fire          │
                    │    SessionStart  │
                    │    hook          │
                    │ 3. Append user   │
                    │    message       │
                    │ 4. Emit          │
                    │    SystemInit    │
                    │ 5. Assemble      │
                    │    system prompt │
                    └────────┬─────────┘
                             │
              ┌──────────────▼──────────────┐
              │                             │
              │     ┌────────────────┐      │
              │     │  LOOP START    │◄─────┼──────────────────────────┐
              │     │                │      │                          │
              │     │ • Drain ctrl   │      │                          │
              │     │ • Check term   │      │   ┌──────────────────┐   │
              │     │   conditions   │      │   │   tool_use       │   │
              │     │ • Proactive    │      │   │                  │   │
              │     │   compaction   │      │   │ Execute tools    │   │
              │     │ • Inject hook  │      │   │ (serial, perms)  │   │
              │     │   context      │      │   │ Append results   │   │
              │     │ • Build req    │      │   │ to history       │   │
              │     └───────┬────────┘      │   └────────┬─────────┘   │
              │             │               │            │             │
              │     ┌───────▼────────┐      │            │             │
              │     │   LLM CALL     │      │            │             │
              │     │                │      │            │             │
              │     │ Complete()     │      │     ┌──────▼──────┐      │
              │     │ Accumulate()   │      │     │ Continue?   │      │
              │     │ Emit assistant │      │     │  (if not    │──────┘
              │     │ Update stats   │      │     │  interrupted)
              │     │ Persist msg    │      │     └─────────────┘
              │     └───────┬────────┘      │
              │             │               │
              │     ┌───────▼────────┐      │
              │     │  STOP REASON   │      │
              │     │  BRANCHING     │      │
              │     └───────┬────────┘      │
              │             │               │
              │   ┌─────────┼─────────┐     │
              │   │         │         │     │
              ▼   ▼         ▼         ▼     │
        ┌──────┐ ┌───────┐ ┌──────┐ ┌──────▼──────┐
        │end_  │ │max_   │ │stop_ │ │  tool_use   │
        │turn  │ │tokens │ │seq   │ │             │
        └──┬───┘ └───┬───┘ └──┬───┘ └─────────────┘
           │         │        │
           ▼         ▼        ▼
     ┌──────────┐ ┌──────┐ ┌──────┐
     │Fire Stop │ │Try   │ │EXIT: │
     │hook      │ │react │ │stop_ │
     │          │ │comp- │ │seq   │
     │Continue? │ │action│ │      │
     │  yes→loop│ │      │ └──────┘
     │  no↓     │ │ok→   │
     └──────┬───┘ │  loop│
            │     │fail↓ │
       Multi│     └──┬───┘
       turn?│        │
        yes│     ┌───▼───┐
         │  │     │EXIT:  │
         ▼  │     │max_   │
     ┌──────▼──┐  │tokens │
     │ Wait    │  └───────┘
     │ for     │
     │ input   │
     │(block)  │
     │         │
     │ input→  │
     │   loop  │
     │ close→  │
     │   exit  │
     └────┬────┘
          │
     ┌────▼────┐
     │  EXIT:  │
     │ end_    │
     │  turn   │
     └─────────┘
```

## Termination Conditions

Checked at the start of each iteration via `checkTermination()`:

```
┌────────────────────────────────────────────────────────────────────┐
│                    checkTermination()                               │
│                                                                    │
│  1. ctx.Err() != nil?                                             │
│     ├── state.IsInterrupted → ExitInterrupted                    │
│     └── else → ExitAborted                                       │
│                                                                    │
│  2. config.MaxTurns > 0 && state.TurnCount >= config.MaxTurns?   │
│     └── ExitMaxTurns                                              │
│                                                                    │
│  3. config.MaxBudgetUSD > 0 && state.TotalCostUSD >= MaxBudget?  │
│     └── ExitMaxBudget                                             │
│                                                                    │
│  4. None of above → "" (continue)                                 │
└────────────────────────────────────────────────────────────────────┘
```

## Exit Reasons

```
┌──────────────────┬──────────────────┬─────────────────────────────────┐
│ ExitReason       │ ResultMessage    │ Trigger                         │
│                  │ Subtype          │                                 │
├──────────────────┼──────────────────┼─────────────────────────────────┤
│ ExitEndTurn      │ success          │ LLM says "stop" + no Continue  │
│ ExitMaxTurns     │ error_max_turns  │ TurnCount >= MaxTurns          │
│ ExitMaxBudget    │ error_max_budget │ TotalCostUSD >= MaxBudgetUSD   │
│ ExitMaxTokens    │ error_during_    │ max_tokens + compaction failed │
│                  │   execution      │                                 │
│ ExitInterrupted  │ error_during_    │ User called Interrupt() or     │
│                  │   execution      │ PermissionResult.Interrupt=true │
│ ExitAborted      │ error_during_    │ Context cancelled externally   │
│                  │   execution      │                                 │
│ ExitStopSequence │ error_during_    │ LLM hit stop_sequence          │
│                  │   execution      │                                 │
└──────────────────┴──────────────────┴─────────────────────────────────┘
```

## Query Object — Channel API

The `Query` is the consumer-facing handle returned by `RunLoop()`.
Claude Code TS uses async iterators; Goat uses typed Go channels.

```
                  ┌─────────────────────────────────┐
                  │           agent.Query            │
                  │                                  │
                  │  Messages() <-chan SDKMessage     │──▶ Consumer reads
                  │  Wait()                          │    all emitted
                  │  GetExitReason() ExitReason      │    messages
                  │  TurnCount() int                 │
                  │  TotalCostUSD() float64          │
                  │                                  │
                  │  // Multi-turn only:             │
                  │  SendUserMessage(string)          │◀── Consumer sends
                  │  SendControl(ControlCommand)      │    follow-up input
                  │  Close()                          │
                  │  Interrupt()                      │
                  └─────────────────────────────────┘

  Internal channels (created by RunLoop):
  ┌──────────────────────────────────────────────────────┐
  │                                                      │
  │  ch (messages)  ─── chan SDKMessage (buffer: 64)      │
  │  inputCh        ─── chan string     (buffer: 1)       │  multi-turn
  │  controlCh      ─── chan ControlCmd (buffer: 1)       │  multi-turn
  │  closeCh        ─── chan struct{}                      │
  │  done           ─── chan struct{}   (closed on exit)  │
  │                                                      │
  └──────────────────────────────────────────────────────┘
```

## SDK Message Emission Timeline

```
Time ──────────────────────────────────────────────────────────────▶

│ SystemInitMessage                                               │
│ (model, tools, CWD, permissions)                                │
│                                                                  │
│ ┌─ LLM Call 1 ─────────────────────────────────┐               │
│ │ [PartialAssistantMessage] × N chunks          │ (if partial) │
│ │ AssistantMessage (full response)               │               │
│ │ ToolProgressMessage (start, elapsed=0)         │               │
│ │ ToolProgressMessage (end, elapsed=1.2s)        │               │
│ └────────────────────────────────────────────────┘               │
│                                                                  │
│ ┌─ LLM Call 2 ─────────────────────────────────┐               │
│ │ AssistantMessage (with text)                   │               │
│ └────────────────────────────────────────────────┘               │
│                                                                  │
│ ResultMessage                                                    │
│ (success/error, turn_count, cost, duration)                      │
│                                                                  │
│ ──── channel closed ────                                         │
```

## Full Single-Iteration Lifecycle (Detailed)

```
┌─────────────────────────────────────────────────────────────────┐
│  ITERATION START                                                 │
│                                                                  │
│  1. Drain controlCh (non-blocking)                              │
│     └── SetPermissionMode, SetModel, etc.                       │
│                                                                  │
│  2. checkTermination()                                           │
│     └── ctx cancelled? MaxTurns? MaxBudget?                     │
│                                                                  │
│  3. Proactive compaction                                         │
│     ├── budget = calculateTokenBudget(config, state, prompt)     │
│     ├── ShouldCompact(budget)?                                   │
│     │   └── yes: Compact() → replace state.Messages              │
│     └── Hook context from compact result                         │
│                                                                  │
│  4. Gather tools: config.ToolRegistry.LLMTools()                │
│                                                                  │
│  5. Inject pending hook context into system prompt               │
│     effectivePrompt = systemPrompt + PendingAdditionalContext   │
│     Clear PendingAdditionalContext                               │
│                                                                  │
│  6. Select model: state.Model || config.Model                    │
│                                                                  │
│  7. Build request: llm.BuildCompletionRequest(...)              │
│     └── model, max_tokens=16384, system, messages, tools, sid   │
├─────────────────────────────────────────────────────────────────┤
│  LLM CALL                                                        │
│                                                                  │
│  8. stream, err = config.LLMClient.Complete(ctx, req)           │
│     └── On error: check ctx, set exit reason, break             │
│                                                                  │
│  9. Register streaming callback (if IncludePartial=true)         │
│     └── Each chunk → emitStreamEvent → PartialAssistantMessage  │
│                                                                  │
│ 10. resp, err = stream.AccumulateWithCallback(onChunk)          │
│     └── On error: check ctx, set exit reason, break             │
├─────────────────────────────────────────────────────────────────┤
│  STATE UPDATE                                                    │
│                                                                  │
│ 11. assistantMsg = responseToAssistantMessage(resp)             │
│     └── Convert CompletionResponse → ChatMessage(role=assistant)│
│                                                                  │
│ 12. state.Messages = append(state.Messages, assistantMsg)       │
│                                                                  │
│ 13. state.TurnCount++                                            │
│     state.addUsage(resp.Usage)                                   │
│     state.TotalCostUSD = config.CostTracker.Add(...)            │
│                                                                  │
│ 14. Emit AssistantMessage to channel                             │
│                                                                  │
│ 15. Persist assistant message to SessionStore                    │
├─────────────────────────────────────────────────────────────────┤
│  STOP REASON BRANCHING                                           │
│                                                                  │
│ 16. Switch on resp.StopReason:                                   │
│                                                                  │
│     "end_turn":                                                  │
│       ├── Fire Stop hook                                         │
│       ├── shouldContinue(stopResults)?                           │
│       │   └── yes: inject context, continue loop                │
│       ├── MultiTurn?                                             │
│       │   ├── yes: emit TurnResult, waitForInput()              │
│       │   │   ├── got input: append user msg, continue          │
│       │   │   └── closed/cancelled: exit                        │
│       │   └── no: exit with ExitEndTurn                         │
│       └── exit                                                   │
│                                                                  │
│     "max_tokens":                                                │
│       ├── discardTruncatedToolBlocks(resp)                      │
│       ├── calculateTokenBudget()                                 │
│       ├── ShouldCompact()?                                       │
│       │   ├── yes: Compact() → continue loop                   │
│       │   └── error: exit with ExitMaxTokens                   │
│       └── exit with ExitMaxTokens                               │
│                                                                  │
│     "tool_use":                                                  │
│       ├── Extract tool_use blocks from response                  │
│       ├── executeTools(ctx, blocks, config, state, ch)          │
│       │   └── Serial: perm check → hooks → execute → hooks     │
│       ├── Convert results to tool messages                       │
│       ├── Append to history, persist each                        │
│       ├── interrupted? → exit with ExitInterrupted              │
│       └── continue loop                                          │
│                                                                  │
│     "stop_sequence":                                             │
│       └── exit with ExitStopSequence                            │
├─────────────────────────────────────────────────────────────────┤
│  FINALIZATION (after loop exits)                                 │
│                                                                  │
│ 17. finalizeSession() → update metadata with stats              │
│ 18. emitResult() → ResultMessage with exit reason               │
│ 19. Fire SessionEnd hook                                         │
│ 20. close(ch) → consumers see channel close                     │
│ 21. close(done) → Wait() unblocks                               │
└─────────────────────────────────────────────────────────────────┘
```

## Comparison with Claude Code TS Loop

```
┌────────────────────┬────────────────────────┬────────────────────────────┐
│ Aspect             │ Claude Code TS          │ Goat Go                    │
├────────────────────┼────────────────────────┼────────────────────────────┤
│ Loop structure     │ async function with     │ for{} in goroutine         │
│                    │ while loop              │                            │
│ Message delivery   │ async generator yield   │ chan SDKMessage (buf: 64)   │
│ Multi-turn         │ readline/terminal input │ inputCh channel            │
│ Cancellation       │ AbortController         │ context.Context            │
│ State              │ class properties        │ LoopState struct           │
│ Tool exec          │ Promise.all (parallel)  │ Serial for{} (by design)  │
│ Streaming          │ on('data') callback     │ AccumulateWithCallback     │
│ Compaction         │ In-class method         │ ContextCompactor interface │
│ Config             │ Object spread           │ Functional options pattern │
│ Error handling     │ try/catch + throw       │ (result, error) tuples     │
└────────────────────┴────────────────────────┴────────────────────────────┘

Note: Claude Code TS executes tools in parallel (Promise.all), while Goat
executes them serially. This is a deliberate simplification — Go's goroutines
make parallel execution easy to add later, but serial execution simplifies
permission/hook/state management significantly.
```

## LoopState — What Lives Across Iterations

```go
type LoopState struct {
    SessionID                string
    Messages                 []llm.ChatMessage       // conversation history
    TurnCount                int                      // LLM calls made
    TotalCostUSD             float64                  // accumulated cost
    TotalUsage               types.CumulativeUsage    // input/output tokens
    ExitReason               ExitReason               // why loop stopped
    Model                    string                   // runtime override
    IsInterrupted            bool                     // user interrupted
    PendingAdditionalContext []string                 // hook-injected context
    StopSequenceValue        string                   // if stop_sequence hit
    LastError                error                    // last error encountered
}
```
