# Hook System — Event-Driven Lifecycle Injection

> `pkg/hooks/` — Go callbacks + shell commands fired at 15 lifecycle events.
> Enables pre/post tool intervention, context injection, and output suppression.

## Hook Architecture

```
 ┌─────────────────────────────────────────────────────────────────────┐
 │                        hooks.Runner                                 │
 │                                                                     │
 │  hooks       map[HookEvent][]CallbackMatcher   // base hooks       │
 │  scopedHooks map[scopeID]map[HookEvent][]CM     // per-agent hooks │
 │  emitCh      chan<- SDKMessage                   // SDK emission    │
 │  mu          sync.RWMutex                        // thread safety   │
 │                                                                     │
 │  Fire(ctx, event, input) → []HookResult                            │
 │  RegisterScoped(scopeID, hooks)   // add per-agent hooks           │
 │  UnregisterScoped(scopeID)        // remove on agent death         │
 └─────────────────────────────────────────────────────────────────────┘
```

## Fire Execution Flow

```
 config.Hooks.Fire(ctx, HookEventPreToolUse, input)
         │
         ▼
 ┌───────────────────────────────────────────────────────────────┐
 │  1. Merge matchers: base hooks + all scoped hooks for event  │
 │     ┌─────────────┐   ┌───────────────────┐                 │
 │     │ base hooks  │ + │ scoped[agent-1]   │ = merged list   │
 │     │ [PreToolUse]│   │ [PreToolUse]      │                 │
 │     └─────────────┘   └───────────────────┘                 │
 │                                                               │
 │  2. For each CallbackMatcher:                                │
 │     ┌─────────────────────────────────────────────────────┐  │
 │     │  a. Match tool name pattern                          │  │
 │     │     Pattern: ""        → match all                   │  │
 │     │     Pattern: "Bash"    → exact match                 │  │
 │     │     Pattern: "mcp__*"  → glob match                  │  │
 │     │                                                      │  │
 │     │  b. Create timeout context (if Timeout > 0)          │  │
 │     │                                                      │  │
 │     │  c. Execute Go callbacks (in order):                 │  │
 │     │     ├── Emit HookStarted message                     │  │
 │     │     ├── Call hook(input, "", ctx)                     │  │
 │     │     ├── Handle async? → executeAsync() with timeout  │  │
 │     │     ├── Emit HookResponse message                    │  │
 │     │     ├── Convert output → HookResult                  │  │
 │     │     └── Continue=false? → STOP processing            │  │
 │     │                                                      │  │
 │     │  d. Execute shell commands (in order):               │  │
 │     │     ├── Emit HookStarted message                     │  │
 │     │     ├── Run "sh -c <cmd>" with JSON stdin            │  │
 │     │     ├── Parse JSON stdout                            │  │
 │     │     ├── Handle async? → re-execute with timeout      │  │
 │     │     ├── Emit HookResponse message                    │  │
 │     │     ├── Convert output → HookResult                  │  │
 │     │     └── Continue=false? → STOP processing            │  │
 │     └─────────────────────────────────────────────────────┘  │
 │                                                               │
 │  3. Return accumulated []HookResult                          │
 └───────────────────────────────────────────────────────────────┘
```

## 15 Hook Events — When They Fire

```
 Session Lifecycle:
 ┌──────────────────────────────────────────────────────────────────┐
 │                                                                  │
 │  SessionStart ──── loop initialization / after compaction        │
 │       │                                                          │
 │       ▼                                                          │
 │  ┌─── Loop Iteration ──────────────────────────────────────┐    │
 │  │                                                          │    │
 │  │  PreCompact ──── before context compaction               │    │
 │  │       │                                                  │    │
 │  │  Setup ──── setup phase                                  │    │
 │  │       │                                                  │    │
 │  │  UserPromptSubmit ──── when user sends input             │    │
 │  │       │                                                  │    │
 │  │  PermissionRequest ──── during permission check (L5)     │    │
 │  │       │                                                  │    │
 │  │  PreToolUse ──── before tool execution                   │    │
 │  │       │                 can: deny, modify input, context │    │
 │  │       ▼                                                  │    │
 │  │  [Tool Executes]                                         │    │
 │  │       │                                                  │    │
 │  │  PostToolUse ──── after successful tool execution        │    │
 │  │       │              can: suppress output, context       │    │
 │  │       │                                                  │    │
 │  │  PostToolUseFailure ──── after tool error                │    │
 │  │       │                                                  │    │
 │  │  Stop ──── on end_turn stop reason                       │    │
 │  │       │       can: force continue (override end_turn)    │    │
 │  │       │                                                  │    │
 │  │  Notification ──── system notifications                  │    │
 │  │                                                          │    │
 │  └──────────────────────────────────────────────────────────┘    │
 │                                                                  │
 │  SessionEnd ──── after loop exits                                │
 │                                                                  │
 │  Subagent Lifecycle:                                             │
 │  SubagentStart ──── when subagent spawns                        │
 │  SubagentStop  ──── when subagent terminates                    │
 │                                                                  │
 │  Team Lifecycle:                                                 │
 │  TeammateIdle   ──── when team member has no tasks              │
 │  TaskCompleted  ──── when team task finishes                    │
 └──────────────────────────────────────────────────────────────────┘
```

## Hook Capabilities Matrix

```
 ┌────────────────────┬───────┬──────────┬─────────┬──────────┬────────┐
 │ Event              │ Deny  │ Suppress │ Context │ Input    │Continue│
 │                    │ Tool  │ Output   │ Inject  │ Modify   │Override│
 ├────────────────────┼───────┼──────────┼─────────┼──────────┼────────┤
 │ PreToolUse         │  ✓    │          │   ✓     │   ✓      │        │
 │ PostToolUse        │       │    ✓     │   ✓     │          │        │
 │ PostToolUseFailure │       │          │   ✓     │          │        │
 │ Stop               │       │          │   ✓     │          │   ✓    │
 │ PermissionRequest  │  ✓    │          │         │          │   ✓*   │
 │ SessionStart       │       │          │   ✓     │          │        │
 │ SessionEnd         │       │          │         │          │        │
 │ Others             │       │          │   ✓     │          │        │
 └────────────────────┴───────┴──────────┴─────────┴──────────┴────────┘
 * PermissionRequest Continue=false triggers Interrupt (stops entire loop)
```

## Shell Hook Protocol (JSON stdin → JSON stdout)

```
 Agent Loop fires hook:
         │
         ▼
 ┌─────────────────────────────────────────────────────────────┐
 │  Shell command receives JSON on stdin:                       │
 │                                                              │
 │  {                                                           │
 │    "session_id": "abc-123",                                  │
 │    "transcript_path": "/sessions/abc-123/transcript.jsonl",  │
 │    "cwd": "/Users/dev/project",                              │
 │    "permission_mode": "default",                             │
 │    "tool_name": "Bash",          // PreToolUse only         │
 │    "tool_input": {"command":"ls"} // PreToolUse only        │
 │  }                                                           │
 │                                                              │
 │  Shell command writes JSON to stdout:                        │
 │                                                              │
 │  Sync response:                                              │
 │  {                                                           │
 │    "continue": true,                                         │
 │    "decision": "allow",         // or "deny"/"block"        │
 │    "reason": "...",                                          │
 │    "suppressOutput": false,                                  │
 │    "systemMessage": "Remember: always use --verbose"         │
 │  }                                                           │
 │                                                              │
 │  OR Async response (first call):                             │
 │  {                                                           │
 │    "async": true,                                            │
 │    "asyncTimeout": 60           // seconds, default 30      │
 │  }                                                           │
 │  → Runner will re-execute with timeout context               │
 │  → Second call should return sync response                   │
 │                                                              │
 │  OR empty stdout:                                            │
 │  → Treated as no-op (continue processing)                    │
 └─────────────────────────────────────────────────────────────┘
```

## Scoped Hooks — Per-Agent Lifecycle

```
 ┌───────────────────────────────────────────────────────────────────┐
 │  Subagent spawns:                                                 │
 │    runner.RegisterScoped("agent-xyz", {                          │
 │      PreToolUse: [{Matcher: "Bash", Hooks: [...]}],              │
 │      PostToolUse: [{...}],                                       │
 │    })                                                             │
 │                                                                   │
 │  During Fire():                                                   │
 │    merged = baseHooks[event] + scopedHooks["agent-xyz"][event]   │
 │    + scopedHooks["agent-abc"][event] + ...                       │
 │                                                                   │
 │  Subagent terminates:                                             │
 │    runner.UnregisterScoped("agent-xyz")                          │
 │    → Hooks for that scope no longer merge                        │
 │                                                                   │
 │  Key: Base hooks ALWAYS fire. Scoped hooks ADD to them.          │
 │  Multiple scopes can be active simultaneously (one per agent).   │
 └───────────────────────────────────────────────────────────────────┘
```

## Context Injection Mechanism

```
 Hook fires and returns SystemMessage:
         │
         ▼
 ┌──────────────────────────────────────────────────────────────┐
 │  collectAdditionalContext(state, hookResults):                │
 │    for each result:                                          │
 │      if result.SystemMessage != "":                          │
 │        state.PendingAdditionalContext.append(msg)            │
 └──────────────────────────────────────────────────────────────┘
         │
         │  (next LLM call)
         ▼
 ┌──────────────────────────────────────────────────────────────┐
 │  effectivePrompt = systemPrompt + "\n\n" +                   │
 │    strings.Join(state.PendingAdditionalContext, "\n")        │
 │  state.PendingAdditionalContext = nil  // consumed, cleared  │
 └──────────────────────────────────────────────────────────────┘

 This lets hooks inject instructions like:
 "Remember: the user prefers verbose output"
 "Warning: this file was recently modified by another tool"
 "Note: MCP server returned updated schema"
```

## SDK Emission During Hook Execution

```
 Time ──────────────────────────────────────────────▶

 │ HookStarted {hookID, hookName, event}            │
 │                                                    │
 │ HookProgress {stdout, stderr}  ← shell hooks only │
 │ HookProgress {stdout, stderr}                      │
 │   ...                                              │
 │                                                    │
 │ HookResponse {hookID, outcome:"success"|"error"}  │
```

## Comparison with Claude Code TS Hooks

```
 ┌────────────────────┬──────────────────────────┬───────────────────────┐
 │ Aspect             │ Claude Code TS            │ Goat Go               │
 ├────────────────────┼──────────────────────────┼───────────────────────┤
 │ Go callbacks       │ N/A (JS functions)        │ HookCallback func     │
 │ Shell hooks        │ ✓ (same JSON protocol)   │ ✓ (same JSON protocol)│
 │ Pattern matching   │ Glob + regex              │ Glob + exact          │
 │ Scoped hooks       │ N/A                       │ RegisterScoped()      │
 │ Async support      │ N/A                       │ Two-phase async       │
 │ Progress emission  │ N/A                       │ HookProgress messages │
 │ Thread safety      │ Single-threaded (Node.js) │ sync.RWMutex          │
 │ Decision compat    │ "approve"/"block"         │ "allow"/"deny" +      │
 │                    │                            │  legacy mapping       │
 └────────────────────┴──────────────────────────┴───────────────────────┘

 Key Go advantage: Scoped hooks enable per-subagent hook isolation
 without global state pollution. In Claude Code TS, all hooks are global.
```
