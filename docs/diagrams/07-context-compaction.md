# Context Compaction — LLM-Powered Summarization

> `pkg/context/` — Manages context window limits via LLM-powered summarization
> with simple truncation fallback. Fires proactively and reactively.

## When Compaction Fires

```
 ┌─────────────────────────────────────────────────────────────────────┐
 │  Two triggers, one goal: keep context within model limits          │
 │                                                                     │
 │  ┌─── PROACTIVE (before each LLM call) ─────────────────────────┐ │
 │  │                                                               │ │
 │  │  budget = calculateTokenBudget(config, state, systemPrompt)  │ │
 │  │                                                               │ │
 │  │  if ShouldCompact(budget):   // utilization > 80%            │ │
 │  │    Compact(ctx, request) → new message list                  │ │
 │  │    state.Messages = compacted                                │ │
 │  │    Fire SessionStart hook (source="compact")                 │ │
 │  │    Emit CompactBoundaryMessage                               │ │
 │  │                                                               │ │
 │  └───────────────────────────────────────────────────────────────┘ │
 │                                                                     │
 │  ┌─── REACTIVE (on max_tokens stop reason) ─────────────────────┐ │
 │  │                                                               │ │
 │  │  LLM returned finish_reason="length" (hit output limit)     │ │
 │  │  discardTruncatedToolBlocks(resp)  ← remove incomplete JSON │ │
 │  │  Recalculate token budget                                    │ │
 │  │                                                               │ │
 │  │  if ShouldCompact(budget):                                   │ │
 │  │    Compact() → replace messages → continue loop              │ │
 │  │  else:                                                       │ │
 │  │    EXIT with ExitMaxTokens                                   │ │
 │  │                                                               │ │
 │  └───────────────────────────────────────────────────────────────┘ │
 └─────────────────────────────────────────────────────────────────────┘
```

## Token Budget Calculation

```
 ┌───────────────────────────────────────────────────────────────────┐
 │  calculateTokenBudget(config, state, systemPrompt)                │
 │                                                                   │
 │  contextLimit = 200,000 tokens (default)                         │
 │    └── or 1,000,000 with beta flag for Sonnet models             │
 │                                                                   │
 │  systemPromptTkns = len(systemPrompt) / 4   ← heuristic         │
 │  messageTkns = Σ (len(msg.content)/4 + 4)   ← 4-token overhead  │
 │  maxOutputTkns = 16,384                     ← from config       │
 │                                                                   │
 │  available = contextLimit - systemPromptTkns                      │
 │            - messageTkns - maxOutputTkns                          │
 │                                                                   │
 │  TokenBudget {                                                    │
 │    ContextLimit:    200000,                                      │
 │    SystemTokens:    systemPromptTkns,                            │
 │    MessageTokens:   messageTkns,                                 │
 │    MaxOutputTokens: 16384,                                       │
 │    Available:       available,     ← can be negative!            │
 │    Utilization:     messageTkns / (contextLimit - systemTkns     │
 │                     - maxOutputTkns),                             │
 │  }                                                               │
 └───────────────────────────────────────────────────────────────────┘
```

## Compaction Flow

```
 Compact(ctx, CompactRequest{Messages, SystemPrompt, Model})
         │
         ▼
 ┌───────────────────────────────────────────────────────────────────┐
 │  1. Fire PreCompact hook                                          │
 │                                                                   │
 │  2. Calculate split point                                         │
 │     └── Walk backward from end, preserve ~40% of messages        │
 │     └── adjustSplitForToolPairs: never split tool_use/tool_result│
 │         ┌────────────────────────────────────────────────────┐   │
 │         │  Messages: [user, assist+tool_use, tool_result,    │   │
 │         │            user, assist, user, assist+tool_use,    │   │
 │         │            tool_result, assist]                     │   │
 │         │                     ▲ split here? NO!               │   │
 │         │                     │ would orphan tool_use         │   │
 │         │                     ▼ adjust to include tool_result │   │
 │         └────────────────────────────────────────────────────┘   │
 │                                                                   │
 │  3. Extract "old" messages (before split) for summarization       │
 │                                                                   │
 │  4. Summarize via LLM                                             │
 │     ├── Build summarization prompt                                │
 │     ├── Call LLMClient.Complete() with old messages               │
 │     ├── stream.Accumulate() → extract text content                │
 │     │                                                             │
 │     ├── SUCCESS:                                                  │
 │     │   summary = extracted text from LLM response               │
 │     │                                                             │
 │     └── FAILURE (LLM error):                                     │
 │         Fallback to simple truncation                             │
 │         summary = "[Context truncated. Earlier conversation      │
 │                    contained N messages about: ...]"              │
 │                                                                   │
 │  5. Build compacted message list:                                 │
 │     [                                                             │
 │       {role: "user", content: "[COMPACT SUMMARY]\n" + summary}, │
 │       {role: "assistant", content: "Understood."},               │
 │       ...preserved messages (after split point)...               │
 │     ]                                                             │
 │                                                                   │
 │  6. Fire SessionStart hook (source="compact")                    │
 │  7. Emit CompactBoundaryMessage                                   │
 │  8. Return compacted messages                                     │
 └───────────────────────────────────────────────────────────────────┘
```

## Split Point Calculation — Visual

```
 Messages (20 total, ~80% utilization):

 [0] user: "Help me refactor"
 [1] assistant: "I'll look at the code" + tool_use(Read)
 [2] tool_result: "file contents..."
 [3] user: "Now fix the bug"           ← ─┐
 [4] assistant: "Let me check" + tool_use  │ These get
 [5] tool_result: "..."                    │ SUMMARIZED
 [6] user: "Also add tests"               │ by LLM
 [7] assistant: "Sure" + tool_use          │
 [8] tool_result: "..."                ← ─┘
 ────────────── split point ──────────────
 [9]  user: "Run the tests"           ← ─┐
 [10] assistant: tool_use(Bash)            │ These are
 [11] tool_result: "PASS"                 │ PRESERVED
 [12] user: "Now deploy"                   │ verbatim
 [13] assistant: "I'll push to git"    ← ─┘

 Result after compaction:
 [0] user: "[COMPACT SUMMARY] Earlier, I refactored code,
            fixed a bug, and added tests..."
 [1] assistant: "Understood."
 [2] user: "Run the tests"               ← preserved
 [3] assistant: tool_use(Bash)            ← preserved
 [4] tool_result: "PASS"                  ← preserved
 [5] user: "Now deploy"                   ← preserved
 [6] assistant: "I'll push to git"        ← preserved
```

## Graceful Degradation

```
 ┌───────────────────────────────────────────────────────────────────┐
 │                                                                   │
 │  LLM summarization succeeds:                                      │
 │    → Rich contextual summary preserving key decisions/facts      │
 │                                                                   │
 │  LLM summarization fails (timeout, error, empty):                 │
 │    → Simple truncation: keep last 40% of messages                │
 │    → Summary placeholder: "[Context truncated...]"               │
 │                                                                   │
 │  Both paths produce valid message lists that the loop can use.   │
 │  The loop NEVER crashes due to compaction failure.               │
 │                                                                   │
 │  Compactor.ShouldCompact() returns false?                         │
 │    → Proactive: skip compaction, proceed with LLM call           │
 │    → Reactive: exit with ExitMaxTokens (can't help)              │
 │                                                                   │
 └───────────────────────────────────────────────────────────────────┘
```

## Comparison with Claude Code TS

```
 ┌────────────────────────────┬──────────────────────────────────────┐
 │ Claude Code TS              │ Goat Go                              │
 ├────────────────────────────┼──────────────────────────────────────┤
 │ In-class method on loop    │ Separate interface + implementation  │
 │ Direct API call for summary│ Same LLM client for summary          │
 │ Anthropic token counting   │ len(text)/4 heuristic (no API)      │
 │ ~50% preservation          │ ~40% preservation                    │
 │ No tool pair safety        │ adjustSplitForToolPairs              │
 │ Hard error on failure      │ Graceful truncation fallback         │
 │ Single threshold           │ Proactive (80%) + reactive           │
 └────────────────────────────┴──────────────────────────────────────┘

 Go advantage: The tool pair safety check prevents orphaned tool_use
 blocks (which would confuse the LLM), and graceful degradation means
 the agent never crashes due to compaction failure.
```
