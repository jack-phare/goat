# Subagent Manager — Spawning & Controlling Child Agents

> `pkg/subagent/` — Manages the lifecycle of child agents. Implements the Agent
> tool's SubagentSpawner interface. 6 built-in + user-defined agent types.

## 12-Step Spawn Flow

```
 tools.AgentTool.Execute(ctx, {type:"general-purpose", prompt:"..."})
         │
         ▼
 subagent.Manager.Spawn(ctx, request)
         │
         ▼
 ┌───────────────────────────────────────────────────────────────────┐
 │  Step 1: LIMITS CHECK                                             │
 │    MaxConcurrent reached? → error "too many subagents"           │
 │                                                                   │
 │  Step 2: RESOLVE DEFINITION                                       │
 │    Merge: BuiltIn(0) < CLI(5) < File-based(10-30)               │
 │    Higher priority overwrites lower                               │
 │    ┌──────────────────────────────────────────────────────────┐  │
 │    │  Priority 0:  BuiltIn (6 agents compiled in)             │  │
 │    │  Priority 5:  CLI flags (--agent-type)                   │  │
 │    │  Priority 10: Plugin (~/.claude/plugins/*/agents/*.md)   │  │
 │    │  Priority 20: User   (~/.claude/agents/*.md)             │  │
 │    │  Priority 30: Project (.claude/agents/*.md)              │  │
 │    └──────────────────────────────────────────────────────────┘  │
 │                                                                   │
 │  Step 3: GENERATE AGENT ID                                        │
 │    uuid.New() → "agent-abc123"                                   │
 │                                                                   │
 │  Step 4: FIRE SubagentStart HOOK                                  │
 │    hooks.Fire(SubagentStart, {agentID, agentType, model})        │
 │                                                                   │
 │  Step 5: RESOLVE MODEL                                            │
 │    "haiku"  → "claude-haiku-4-5-20251001"                       │
 │    "sonnet" → "claude-sonnet-4-5-20250929"                      │
 │    "opus"   → "claude-opus-4-5-20250514"                        │
 │    ""       → use parent's model                                 │
 │                                                                   │
 │  Step 6: BUILD TOOL REGISTRY                                      │
 │    Clone parent registry                                          │
 │    Filter by def.Tools (if specified)                             │
 │    Filter by def.DisallowedTools                                  │
 │    REMOVE Agent tool (no nesting!)                                │
 │    CanSpawnSubagents = false                                      │
 │                                                                   │
 │  Step 7: BUILD PERMISSIONS                                        │
 │    BackgroundMode? → AllowAllChecker (auto-allow everything)     │
 │    Otherwise → clone parent's permission checker                  │
 │                                                                   │
 │  Step 8: LOAD MEMORY                                              │
 │    Auto mode: create ~/.claude/agents/{type}/memory/MEMORY.md    │
 │    Load first 200 lines of MEMORY.md if exists                   │
 │                                                                   │
 │  Step 9: ASSEMBLE PROMPT                                          │
 │    Agent-specific prompt from def.Prompt                          │
 │    + memory content                                               │
 │    + parent context (CWD, git state)                              │
 │                                                                   │
 │  Step 10: BUILD CONFIG                                            │
 │    AgentConfig with resolved model, tools, perms, prompt          │
 │    BackgroundMode flags for headless behavior                     │
 │                                                                   │
 │  Step 11: REGISTER SCOPED HOOKS                                   │
 │    hooks.RegisterScoped(agentID, agentHookMap)                   │
 │                                                                   │
 │  Step 12: LAUNCH                                                  │
 │    Background? → goroutine, return agentID immediately           │
 │    Foreground? → RunLoop(), block until complete                  │
 └───────────────────────────────────────────────────────────────────┘
```

## 6 Built-In Agent Definitions

```
 ┌──────────────────────┬──────────┬────────────────────────────────────┐
 │ Agent Type            │ Model    │ Purpose                            │
 ├──────────────────────┼──────────┼────────────────────────────────────┤
 │ general-purpose       │ default  │ Multi-step tasks, full tool access │
 │ Explore               │ haiku    │ Fast codebase exploration          │
 │ Plan                  │ default  │ Design implementation plans        │
 │ Bash                  │ default  │ Command execution specialist       │
 │ statusline-setup      │ sonnet   │ Configure status line settings     │
 │ claude-code-guide     │ haiku    │ Answer questions about Claude Code │
 └──────────────────────┴──────────┴────────────────────────────────────┘

 + User can define custom agents in .md files with YAML frontmatter:

 ---
 description: "Python code reviewer"
 model: haiku
 tools:
   - Read
   - Glob
   - Grep
 ---
 You are a Python code reviewer. Focus on...
```

## Agent Definition Loading — File-Based Discovery

```
 ┌───────────────────────────────────────────────────────────────────┐
 │  Loader scans directories in order:                               │
 │                                                                   │
 │  1. Plugin agents: ~/.claude/plugins/*/agents/*.md  (priority 10)│
 │  2. User agents:   ~/.claude/agents/*.md            (priority 20)│
 │  3. Project agents: .claude/agents/*.md             (priority 30)│
 │                                                                   │
 │  Each .md file parsed with YAML frontmatter:                     │
 │  ┌──────────────────────────────────────────────┐                │
 │  │  ---                                          │                │
 │  │  description: "Data pipeline builder"         │ ← YAML        │
 │  │  model: haiku                                 │                │
 │  │  tools:                                       │                │
 │  │    - Read                                     │                │
 │  │    - Bash                                     │                │
 │  │  maxTurns: 50                                 │                │
 │  │  ---                                          │                │
 │  │  You are a data pipeline builder.             │ ← Markdown    │
 │  │  Focus on efficient ETL processes...          │   (prompt)    │
 │  └──────────────────────────────────────────────┘                │
 │                                                                   │
 │  Higher priority overwrites lower for same name                  │
 │  Project agents (30) override user agents (20)                   │
 │  User agents (20) override plugin agents (10)                    │
 │  All override built-in agents (0)                                │
 └───────────────────────────────────────────────────────────────────┘
```

## RunningAgent — Concurrent-Safe State

```
 ┌───────────────────────────────────────────────────────────────────┐
 │  RunningAgent {                                                   │
 │    ID:       "agent-abc123"                                      │
 │    Type:     "Explore"                                            │
 │    State:    Running | Completed | Failed | Stopped              │
 │    Output:   []SDKMessage  (accumulated output)                  │
 │    Error:    error                                                │
 │    mu:       sync.Mutex    (protects all fields)                 │
 │  }                                                               │
 │                                                                   │
 │  Background agent goroutine:                                      │
 │    q := RunLoop(ctx, prompt, config)                             │
 │    for msg := range q.Messages() {                               │
 │      agent.mu.Lock()                                             │
 │      agent.Output = append(agent.Output, msg)                   │
 │      agent.mu.Unlock()                                           │
 │    }                                                             │
 │    agent.mu.Lock()                                               │
 │    agent.State = Completed                                       │
 │    agent.mu.Unlock()                                             │
 │                                                                   │
 │  GetOutput(agentID, block, timeout):                              │
 │    block=true:  wait for State != Running (or timeout)           │
 │    block=false: return current Output immediately                │
 └───────────────────────────────────────────────────────────────────┘
```

## No Nesting Rule

```
 ┌───────────────────────────────────────────────────────────────────┐
 │                                                                   │
 │  Parent Agent                                                     │
 │    Tools: [Bash, Read, Write, Edit, Glob, Grep, Agent, ...]     │
 │                          ▲                                        │
 │                          │ Agent tool present                     │
 │                          │                                        │
 │  Child Agent (spawned by Agent tool)                              │
 │    Tools: [Bash, Read, Write, Edit, Glob, Grep, ...]            │
 │                                            ▲                      │
 │                                            │ Agent tool REMOVED  │
 │                                            │ CanSpawnSubagents=  │
 │                                            │   false              │
 │                                                                   │
 │  Why: Prevents infinite recursion. A subagent cannot spawn       │
 │  another subagent. Only the lead agent can spawn subagents.      │
 │                                                                   │
 │  Claude Code TS does the same — subagents don't get the          │
 │  Task tool (their equivalent of Agent).                          │
 └───────────────────────────────────────────────────────────────────┘
```

## Scoped Hook Lifecycle

```
 ┌───────────────────────────────────────────────────────────────────┐
 │                                                                   │
 │  Spawn:                                                           │
 │    hooks.RegisterScoped("agent-abc123", {                        │
 │      PreToolUse:  [{Matcher: "Bash", Hooks: [limitBash]}],      │
 │      PostToolUse: [{Matcher: "", Hooks: [logAll]}],              │
 │    })                                                             │
 │    → Agent's hooks merge with base hooks during Fire()           │
 │                                                                   │
 │  Running:                                                         │
 │    Any Fire(PreToolUse, ...) includes agent's hooks              │
 │    Other agents' scoped hooks also merge (all active scopes)     │
 │                                                                   │
 │  Terminate:                                                       │
 │    hooks.UnregisterScoped("agent-abc123")                        │
 │    → Agent's hooks no longer participate in Fire()               │
 │    Fire SubagentStop hook                                        │
 │                                                                   │
 └───────────────────────────────────────────────────────────────────┘
```
