# Permission System — 8-Layer Checker & 6 Modes

> `pkg/permission/` — Layered access control for tool execution. Implements
> the same permission model as Claude Code TS with compile-time type safety.

## The 8-Layer Waterfall

Each tool execution goes through this waterfall. Each layer can short-circuit
with `allow` or `deny`, or fall through to the next layer.

```
 ┌─────────────────────────────────────────────────────────────────────┐
 │                    Checker.Check(ctx, toolName, input)              │
 │                                                                     │
 │  ┌─── Layer 1: MODE CHECK ──────────────────────────────────────┐  │
 │  │                                                               │  │
 │  │  bypassPermissions → ALLOW (if flag set, else ERROR)         │  │
 │  │  plan             → DENY ("not allowed in plan mode")        │  │
 │  │  delegate         → toolName=="Agent" ? ALLOW : DENY         │  │
 │  │  others           → fall through                              │  │
 │  └───────────────────────────────────────────────────────────────┘  │
 │                              │                                      │
 │  ┌─── Layer 2: DISABLED CHECK ──────────────────────────────────┐  │
 │  │                                                               │  │
 │  │  toolName in disabledTools? → DENY ("tool is disabled")      │  │
 │  └───────────────────────────────────────────────────────────────┘  │
 │                              │                                      │
 │  ┌─── Layer 3: ALLOWED CHECK ───────────────────────────────────┐  │
 │  │                                                               │  │
 │  │  toolName in allowedTools? → ALLOW                            │  │
 │  └───────────────────────────────────────────────────────────────┘  │
 │                              │                                      │
 │  ┌─── Layer 4: RULES CHECK ─────────────────────────────────────┐  │
 │  │                                                               │  │
 │  │  Search configRules first (priority)                          │  │
 │  │  Then sessionRules                                            │  │
 │  │  First matching rule wins:                                    │  │
 │  │    "allow" → ALLOW                                            │  │
 │  │    "deny"  → DENY                                             │  │
 │  │    "ask"   → fall through                                     │  │
 │  └───────────────────────────────────────────────────────────────┘  │
 │                              │                                      │
 │  ┌─── Layer 5: HOOK CHECK ──────────────────────────────────────┐  │
 │  │                                                               │  │
 │  │  Fire PermissionRequest hook                                  │  │
 │  │  Check decision field:                                        │  │
 │  │    "allow" → ALLOW                                            │  │
 │  │    "deny"  → DENY                                             │  │
 │  │  Check continue field:                                        │  │
 │  │    continue=false + no decision → DENY + Interrupt            │  │
 │  └───────────────────────────────────────────────────────────────┘  │
 │                              │                                      │
 │  ┌─── Layer 6: CALLBACK CHECK ──────────────────────────────────┐  │
 │  │                                                               │  │
 │  │  Call canUseTool callback (types.CanUseToolFunc)              │  │
 │  │  Returns PermissionResult with:                               │  │
 │  │    Behavior, UpdatedInput, Message                            │  │
 │  │  "allow"/"deny" → short-circuit                               │  │
 │  │  error/nil → fall through                                     │  │
 │  └───────────────────────────────────────────────────────────────┘  │
 │                              │                                      │
 │  ┌─── Layer 7: MODE DEFAULT ─────────────────────────────────────┐  │
 │  │                                                               │  │
 │  │  DefaultBehaviorForTool(mode, toolName)                       │  │
 │  │  Uses risk level matrix (see below)                           │  │
 │  │    "allow" → ALLOW                                            │  │
 │  │    "deny"  → DENY                                             │  │
 │  │    "ask"   → fall through to prompter                         │  │
 │  └───────────────────────────────────────────────────────────────┘  │
 │                              │                                      │
 │  ┌─── Layer 8: PROMPTER ────────────────────────────────────────┐  │
 │  │                                                               │  │
 │  │  userPrompter != nil?                                         │  │
 │  │    yes → PromptForPermission(toolName, input, suggestions)   │  │
 │  │    no  → DENY ("no interactive prompter available")           │  │
 │  │           ▲ This is "headless behavior"                       │  │
 │  └───────────────────────────────────────────────────────────────┘  │
 └─────────────────────────────────────────────────────────────────────┘
```

## Permission Modes × Risk Level Matrix

```
             │ RiskNone  │ RiskLow   │ RiskMedium │ RiskHigh  │ RiskCrit  │
             │ (Read,    │ (Config,  │ (Write,    │ (Bash,    │ (Agent)   │
             │  Glob,    │  TaskOut, │  Edit,     │  WebFetch,│           │
             │  Grep)    │  AskUser) │  Notebook) │  mcp__*)  │           │
─────────────┼───────────┼───────────┼────────────┼───────────┼───────────┤
 default     │  ALLOW    │  ALLOW    │   ASK      │   ASK     │   ASK     │
─────────────┼───────────┼───────────┼────────────┼───────────┼───────────┤
 acceptEdits │  ALLOW    │  ALLOW    │  ALLOW     │   ASK     │   ASK     │
─────────────┼───────────┼───────────┼────────────┼───────────┼───────────┤
 bypass      │  ALLOW    │  ALLOW    │  ALLOW     │  ALLOW    │  ALLOW    │
─────────────┼───────────┼───────────┼────────────┼───────────┼───────────┤
 dontAsk     │  ALLOW    │  ALLOW    │  DENY      │  DENY     │  DENY     │
─────────────┼───────────┼───────────┼────────────┼───────────┼───────────┤
 plan        │  DENY     │  DENY     │  DENY      │  DENY     │  DENY     │
─────────────┼───────────┼───────────┼────────────┼───────────┼───────────┤
 delegate    │  DENY     │  DENY     │  DENY      │  DENY     │  DENY*    │
             │           │           │            │           │ *Agent=   │
             │           │           │            │           │  ALLOW    │
─────────────┴───────────┴───────────┴────────────┴───────────┴───────────┘

 ASK = falls through to prompter (Layer 8)
       In headless mode: ASK resolves to DENY
```

## Rule Matching — Pattern Types

```
 ┌──────────────────────────────────────────────────────────────────┐
 │  Rule: {ToolName: "Bash", RuleContent: "npm test"}              │
 │                                                                  │
 │  Step 1: Exact tool name match                                  │
 │    "Bash" == "Bash" ✓                                           │
 │                                                                  │
 │  Step 2: Extract matching field from input                       │
 │    Tool "Bash" → matches against input["command"]               │
 │    Tool "Write" → matches against input["file_path"]            │
 │    Tool "Glob" → matches against input["pattern"] or ["path"]  │
 │                                                                  │
 │  Step 3: Pattern matching                                        │
 │    Contains *, ?, [, { ?                                         │
 │      yes → doublestar.Match(pattern, value)                     │
 │      no  → strings.Contains(lower(value), lower(pattern))      │
 │                                                                  │
 │  Examples:                                                       │
 │    Rule "npm test"     + cmd "npm test --watch" → ✓ (substring)│
 │    Rule "/src/**"      + path "/src/pkg/foo.go" → ✓ (glob)    │
 │    Rule "NPM TEST"     + cmd "npm test"         → ✓ (case-i)  │
 │    Rule "/src/**/*.go" + path "/src/main.go"    → ✓ (glob)    │
 └──────────────────────────────────────────────────────────────────┘
```

## ApplyUpdate — Session Rule Accumulation

```
 ┌───────────────────────────────────────────────────────────────────┐
 │  ApplyUpdate(update types.PermissionUpdate)                       │
 │                                                                   │
 │  update.Type = "addRules":                                        │
 │    Create PermissionRule{ToolName, RuleContent, Behavior: allow} │
 │    Destination == "session"?                                      │
 │      yes → append to checker.sessionRules                        │
 │      no  → append to checker.configRules                         │
 │                                                                   │
 │  update.Type = "replaceRules":                                    │
 │    Remove ALL rules for this tool name                            │
 │    Add the new rule                                               │
 │                                                                   │
 │  update.Type = "removeRules":                                     │
 │    Find and remove matching rule (tool + content)                 │
 │                                                                   │
 │  update.Type = "setMode":                                         │
 │    checker.mode = *update.Mode                                    │
 │                                                                   │
 │  Thread safety: Full mutex lock during updates                    │
 │  (RLock during Check, Lock during ApplyUpdate)                   │
 └───────────────────────────────────────────────────────────────────┘
```

## PermissionResult — What Flows Back to the Loop

```
 ┌───────────────────────────────────────────────────────────────────┐
 │  PermissionResult {                                               │
 │    Behavior:    "allow" | "deny" | "ask"                         │
 │    UpdatedInput: map[string]any      // nil = no change          │
 │    Message:      string              // deny reason              │
 │    Interrupt:    bool                // stop entire loop         │
 │    ToolUseID:    string              // correlation              │
 │  }                                                               │
 │                                                                   │
 │  In the loop (tools.go):                                         │
 │    if Behavior != "allow":                                       │
 │      return error to LLM ("Error: " + Message)                   │
 │      if Interrupt: stop loop entirely                             │
 │                                                                   │
 │    if UpdatedInput != nil:                                        │
 │      replace tool input with UpdatedInput                         │
 │      (e.g., sanitize command before execution)                   │
 │                                                                   │
 │  Key difference from Claude Code TS:                              │
 │    TS uses an Allowed boolean + updatedInput                     │
 │    Go uses Behavior string for 3-state (allow/deny/ask)          │
 │    The Interrupt flag is Go-specific — enables hooks to          │
 │    completely stop the loop, not just deny one tool.             │
 └───────────────────────────────────────────────────────────────────┘
```

## MCP Tool Risk with Annotations

```
 MCP server declares annotations in capabilities:
   {readOnly: true}  or  {destructive: true}

 ┌────────────────────────────────┬──────────────────────────────────┐
 │  Annotation                    │  Risk Level                      │
 ├────────────────────────────────┼──────────────────────────────────┤
 │  destructive: true             │  RiskCritical                    │
 │  readOnly: true                │  RiskLow                         │
 │  destructive + readOnly        │  RiskCritical (destructive wins) │
 │  no annotations                │  RiskHigh (conservative default) │
 │  non-MCP tool                  │  Looked up by tool name          │
 └────────────────────────────────┴──────────────────────────────────┘
```

## Concurrency Model

```
 ┌───────────────────────────────────────────────────────────────────┐
 │  Checker uses sync.RWMutex:                                       │
 │                                                                   │
 │  Check()       → RLock  (multiple concurrent reads OK)           │
 │  ApplyUpdate() → Lock   (exclusive write)                        │
 │                                                                   │
 │  This allows:                                                     │
 │  - Multiple subagents checking permissions concurrently           │
 │  - Rule updates don't race with permission checks                 │
 │  - All 55 tests pass with -race flag                             │
 └───────────────────────────────────────────────────────────────────┘
```
