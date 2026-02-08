# Spec 08: Subagent Manager

**Go Package**: `pkg/subagent/`
**Source References**:
- `sdk.d.ts:33-67` — AgentDefinition
- `sdk-tools.d.ts:32-73` — AgentInput
- `sdk-tools.d.ts:113-126` — TaskOutputInput
- `sdk-tools.d.ts:272-279` — TaskStopInput
- `sdk.d.ts:479-497` — Options.agent, Options.agents
- Piebald-AI prompt files for built-in agent types
- [Sub-agents docs](https://code.claude.com/docs/en/sub-agents.md) (latest upstream)
- [Agent teams docs](https://code.claude.com/docs/en/agent-teams.md) (see Spec 08c)

**Related Specs**:
- **Spec 08b** — Agent Definition Loading (file-based definitions, scope resolution)
- **Spec 08c** — Agent Teams (multi-session coordination layer)

---

## 1. Purpose

The subagent manager handles spawning, lifecycle, and communication for child agent loops. When the main agent uses the `Agent` (Task) tool, a new agentic loop is created with its own system prompt, tool restrictions, and model configuration.

**Key constraint**: Subagents **cannot** spawn other subagents. Only the main agent (or a team lead in agent teams mode) can spawn subagents. This is a hard rule, not a configurable depth limit.

---

## 2. Agent Definition

```go
// AgentDefinition configures a named subagent type.
// Loaded from frontmatter files (see Spec 08b) or programmatic registration.
type AgentDefinition struct {
    Name                 string              `json:"name"`                    // unique identifier (lowercase + hyphens)
    Description          string              `json:"description"`             // when Claude should delegate to this agent
    Tools                []string            `json:"tools,omitempty"`         // allowed tools (nil = inherit all); supports Task(type) syntax
    DisallowedTools      []string            `json:"disallowedTools,omitempty"`
    Prompt               string              `json:"prompt"`                  // system prompt (markdown body in file-based defs)
    Model                string              `json:"model,omitempty"`         // "sonnet"|"opus"|"haiku"|"inherit" (default: "inherit")
    PermissionMode       string              `json:"permissionMode,omitempty"` // "default"|"acceptEdits"|"delegate"|"dontAsk"|"bypassPermissions"|"plan"
    MCPServers           []AgentMCPServerSpec `json:"mcpServers,omitempty"`
    CriticalReminder     string              `json:"criticalSystemReminder_EXPERIMENTAL,omitempty"`
    Skills               []string            `json:"skills,omitempty"`        // skill names to preload (full content injected, NOT inherited from parent)
    MaxTurns             *int                `json:"maxTurns,omitempty"`      // agentic turn limit
    Memory               string              `json:"memory,omitempty"`        // "user"|"project"|"local" — persistent memory scope
    Hooks                map[string][]HookRule `json:"hooks,omitempty"`       // scoped lifecycle hooks (PreToolUse, PostToolUse, Stop)

    // Metadata (set by loader, not in frontmatter)
    Source               AgentSource         `json:"-"` // where this definition was loaded from
    Priority             int                 `json:"-"` // resolution priority (see Spec 08b)
}

type AgentMCPServerSpec struct {
    ServerName string   `json:"serverName"`
    Tools      []string `json:"tools,omitempty"` // subset of server's tools
}

// HookRule defines a hook matcher + command within a subagent definition.
type HookRule struct {
    Matcher string      `json:"matcher,omitempty"` // regex pattern to match (tool name, agent type, etc.)
    Hooks   []HookEntry `json:"hooks"`
}

type HookEntry struct {
    Type    string `json:"type"`    // "command"
    Command string `json:"command"` // shell command to execute
}

type AgentSource int
const (
    AgentSourceBuiltIn AgentSource = iota
    AgentSourceCLIFlag             // --agents JSON (highest priority)
    AgentSourceProject             // .claude/agents/
    AgentSourceUser                // ~/.claude/agents/
    AgentSourcePlugin              // plugin agents/ dir (lowest priority)
)
```

---

## 3. Built-in Subagent Types

### 3.1 general-purpose (default)
- **Prompt**: "Agent Prompt: Task tool" from Piebald
- **Model**: Inherits from parent
- **Tools**: All tools (inherits from parent)
- **Purpose**: Complex research, multi-step operations, code modifications

### 3.2 Explore
- **Prompt**: "Agent Prompt: Explore" from Piebald
- **Model**: **Haiku** (fast, low-latency)
- **Tools**: Read-only subset — denied Write and Edit
- **Purpose**: Fast, read-only codebase exploration
- **Thoroughness levels**: quick, medium, very thorough (specified in spawn prompt)

### 3.3 Plan
- **Prompt**: "Agent Prompt: Plan mode (enhanced)" from Piebald
- **Model**: Inherits from parent
- **Tools**: Read-only subset — denied Write and Edit
- **Purpose**: Codebase research for planning mode

### 3.4 Bash
- **Model**: Inherits from parent
- **Tools**: Bash tool in separate context
- **Purpose**: Terminal commands isolated from main context

### 3.5 statusline-setup
- **Model**: Sonnet
- **Purpose**: `/statusline` configuration

### 3.6 claude-code-guide
- **Model**: Haiku
- **Purpose**: Questions about Claude Code features

### 3.7 Custom Agents
Defined via file-based definitions (see Spec 08b), `--agents` CLI flag, or `Options.agents` map.

---

## 4. Skills Injection

When `Skills` is set on an `AgentDefinition`:
- The **full content** of each named skill is injected into the subagent's system prompt at startup
- Subagents do **NOT** inherit skills from the parent conversation
- Skills must be listed explicitly in the definition
- This is distinct from "running a skill in a subagent" — here the subagent owns the system prompt and gets skill content injected

---

## 5. Persistent Memory

When `Memory` is set, the subagent gets a persistent directory that survives across conversations.

### 5.1 Memory Scopes

| Scope     | Path                                           | Use case |
|-----------|-------------------------------------------------|----------|
| `user`    | `~/.claude/agent-memory/<agent-name>/`          | Cross-project learnings |
| `project` | `.claude/agent-memory/<agent-name>/`            | Project-specific, shareable via VCS |
| `local`   | `.claude/agent-memory-local/<agent-name>/`      | Project-specific, not in VCS |

### 5.2 Memory Behavior

When memory is enabled:
1. System prompt includes instructions for reading/writing the memory directory
2. First 200 lines of `MEMORY.md` from the memory dir are injected into the system prompt
3. Read, Write, and Edit tools are **automatically enabled** regardless of tool restrictions

```go
func resolveMemoryDir(agentName, memoryScope, cwd string) string {
    switch memoryScope {
    case "user":
        return filepath.Join(os.UserHomeDir(), ".claude", "agent-memory", agentName)
    case "project":
        return filepath.Join(cwd, ".claude", "agent-memory", agentName)
    case "local":
        return filepath.Join(cwd, ".claude", "agent-memory-local", agentName)
    default:
        return ""
    }
}
```

---

## 6. Subagent-Scoped Hooks

When `Hooks` is set on an `AgentDefinition`, hooks are activated when the subagent starts and cleaned up when it finishes.

### 6.1 Supported Hook Events in Frontmatter

| Event         | Matcher input | When it fires |
|---------------|---------------|---------------|
| `PreToolUse`  | Tool name     | Before the subagent uses a tool |
| `PostToolUse` | Tool name     | After the subagent uses a tool |
| `Stop`        | (none)        | When the subagent finishes (auto-converted to `SubagentStop` at runtime) |

### 6.2 Hook Registration Lifecycle

```go
func (m *Manager) registerScopedHooks(ra *RunningAgent) func() {
    if len(ra.Definition.Hooks) == 0 {
        return func() {} // no-op cleanup
    }
    // Register hooks with the hook runner, scoped to this agent's context
    ids := m.hooks.RegisterScoped(ra.ID, ra.Definition.Hooks)
    // Return cleanup function
    return func() {
        m.hooks.UnregisterScoped(ids)
    }
}
```

---

## 7. Tool Resolution with Task(type) Restrictions

### 7.1 `Task(agent_type)` Syntax

The `Tools` field supports special syntax for restricting which subagent types can be spawned:

| Syntax | Meaning |
|--------|---------|
| `Task` | Can spawn any subagent type |
| `Task(worker, researcher)` | Can only spawn `worker` and `researcher` types |
| Task omitted entirely | Cannot spawn subagents |

**Only applies** to agents running as the main thread via `claude --agent`. Subagents cannot spawn other subagents regardless.

### 7.2 Tool Resolution

```go
// TaskRestriction parsed from "Task(type1, type2)" syntax.
type TaskRestriction struct {
    Unrestricted bool     // true if just "Task" with no parens
    AllowedTypes []string // nil if Task omitted; populated if Task(types)
}

func parseTaskRestriction(tools []string) (*TaskRestriction, []string) {
    var restriction *TaskRestriction
    var remaining []string

    for _, t := range tools {
        if t == "Task" {
            restriction = &TaskRestriction{Unrestricted: true}
        } else if strings.HasPrefix(t, "Task(") && strings.HasSuffix(t, ")") {
            inner := t[5 : len(t)-1]
            types := strings.Split(inner, ",")
            for i := range types {
                types[i] = strings.TrimSpace(types[i])
            }
            restriction = &TaskRestriction{AllowedTypes: types}
        } else {
            remaining = append(remaining, t)
        }
    }
    return restriction, remaining
}

func resolveTools(allowed []string, disallowed []string) ([]string, *TaskRestriction) {
    if allowed == nil {
        // Inherit all tools, minus disallowed
        all := registry.AllToolNames()
        return filterOut(all, disallowed), nil // nil = no Task restriction info (inherit)
    }
    restriction, plainTools := parseTaskRestriction(allowed)
    return filterOut(plainTools, disallowed), restriction
}
```

---

## 8. Go Types

### 8.1 Manager

```go
// Manager tracks active subagents and their lifecycles.
type Manager struct {
    mu          sync.RWMutex
    agents      map[string]AgentDefinition   // registered agent types (from all sources)
    active      map[string]*RunningAgent     // active agent instances by ID
    parent      *agent.LoopState             // parent loop state reference
    hooks       *hooks.Runner
    outputDir   string                       // directory for background task output files
    transcripts string                       // directory for subagent transcript JSONL files
}

func NewManager(agents map[string]AgentDefinition, hooks *hooks.Runner, opts ManagerOpts) *Manager

type ManagerOpts struct {
    OutputDir      string // background task output dir
    TranscriptDir  string // subagent transcript dir (e.g. ~/.claude/projects/{proj}/{session}/subagents/)
    ParentState    *agent.LoopState
}

func (m *Manager) Spawn(ctx context.Context, input AgentInput) (string, error)  // returns agent ID
func (m *Manager) GetOutput(taskID string, block bool, timeout time.Duration) (*TaskResult, error)
func (m *Manager) Stop(taskID string) error
func (m *Manager) List() []AgentStatus
func (m *Manager) RegisterAgents(defs map[string]AgentDefinition) // merge in new definitions
```

### 8.2 Running Agent

```go
// RunningAgent represents a spawned subagent loop.
type RunningAgent struct {
    ID          string
    Type        string            // subagent_type from AgentInput
    Name        string            // display name
    Definition  AgentDefinition
    State       AgentState
    StartedAt   time.Time
    Output      *AgentOutput      // accumulated output
    Loop        *agent.LoopState  // reference to the subagent's loop state
    Cancel      context.CancelFunc
    Done        chan struct{}
    Metrics     *TaskMetrics      // collected after completion
    cleanupFn   func()            // scoped hook cleanup
}

type AgentState int
const (
    AgentRunning    AgentState = iota
    AgentCompleted
    AgentFailed
    AgentStopped
)

type AgentOutput struct {
    mu      sync.Mutex
    content strings.Builder
    result  *types.ResultMessage
}

// TaskMetrics returned in tool results after subagent completes.
type TaskMetrics struct {
    TokensUsed  int           `json:"tokens_used"`
    ToolUses    int           `json:"tool_uses"`
    Duration    time.Duration `json:"duration"`
}

// TaskResult returned by GetOutput.
type TaskResult struct {
    Content string       `json:"content"`
    Metrics *TaskMetrics `json:"metrics,omitempty"`
    State   AgentState   `json:"state"`
}
```

---

## 9. Spawn Flow

```go
func (m *Manager) Spawn(ctx context.Context, input AgentInput) (string, error) {
    // 0. Check resource limits
    if err := m.checkLimits(); err != nil {
        return "", err
    }

    // 1. Resolve agent definition
    def, ok := m.agents[input.SubagentType]
    if !ok {
        return "", fmt.Errorf("unknown subagent_type: %s", input.SubagentType)
    }

    // 2. Generate agent ID (or resume existing)
    agentID := uuid.New().String()
    if input.Resume != nil {
        agentID = *input.Resume
    }

    // 3. Fire SubagentStart hook
    m.hooks.Fire(ctx, hooks.HookSubagentStart, &hooks.SubagentStartHookInput{
        AgentID:   agentID,
        AgentType: input.SubagentType,
    }, input.SubagentType) // matcher = agent type name

    // 4. Resolve model
    model := resolveModel(def.Model, input.Model, m.parent.Model)

    // 5. Resolve tools + Task restrictions
    tools, taskRestriction := resolveTools(def.Tools, def.DisallowedTools)
    _ = taskRestriction // stored but not enforced on subagents (they can't spawn)

    // 6. Resolve permission mode (definition takes precedence, input overrides)
    permMode := resolvePermissionMode(def.PermissionMode, input.Mode)

    // 7. Build memory config if enabled
    var memoryDir string
    if def.Memory != "" {
        memoryDir = resolveMemoryDir(def.Name, def.Memory, m.parent.CWD)
        // Auto-enable Read/Write/Edit for memory access
        tools = ensureTools(tools, "FileRead", "FileWrite", "FileEdit")
    }

    // 8. Inject skill content into system prompt
    systemPrompt := prompt.AssembleSubagentPrompt(def, m.parentConfig(), prompt.SubagentOpts{
        Skills:    def.Skills,
        MemoryDir: memoryDir,
    })

    // 9. Build subagent config
    config := agent.AgentConfig{
        Model:          model,
        SystemPrompt:   systemPrompt,
        MaxTurns:       resolveMaxTurns(def.MaxTurns, input.MaxTurns),
        Tools:          tools,
        PermissionMode: permMode,
        CWD:            m.parent.CWD,
        SessionID:      agentID,
        PersistSession: true,
        TranscriptPath: filepath.Join(m.transcripts, "agent-"+agentID+".jsonl"),
        CanSpawnSubagents: false, // HARD RULE: subagents cannot spawn subagents
    }

    // 10. Create running agent
    agentCtx, cancel := context.WithCancel(ctx)
    ra := &RunningAgent{
        ID:         agentID,
        Type:       input.SubagentType,
        Name:       input.Name,
        Definition: def,
        State:      AgentRunning,
        StartedAt:  time.Now(),
        Output:     &AgentOutput{},
        Cancel:     cancel,
        Done:       make(chan struct{}),
    }

    // 11. Register scoped hooks from definition
    ra.cleanupFn = m.registerScopedHooks(ra)

    m.mu.Lock()
    m.active[agentID] = ra
    m.mu.Unlock()

    // 12. Run the subagent loop
    runFunc := func() {
        defer close(ra.Done)
        defer ra.cleanupFn()
        defer func() {
            ra.Metrics = &TaskMetrics{
                Duration: time.Since(ra.StartedAt),
                // TokensUsed and ToolUses populated from loop state
            }
            if ra.Loop != nil {
                ra.Metrics.TokensUsed = ra.Loop.TotalTokens()
                ra.Metrics.ToolUses = ra.Loop.TotalToolUses()
            }
            // Fire SubagentStop hook
            m.hooks.Fire(ctx, hooks.HookSubagentStop, &hooks.SubagentStopHookInput{
                AgentID:   agentID,
                AgentType: input.SubagentType,
            }, input.SubagentType)
        }()

        query := agent.RunLoop(agentCtx, input.Prompt, config)
        ra.Loop = query.LoopState
        for msg := range query.Messages {
            ra.Output.Append(msg)
        }
        if agentCtx.Err() != nil {
            ra.State = AgentStopped
        } else {
            ra.State = AgentCompleted
        }
    }

    if input.RunInBackground != nil && *input.RunInBackground {
        // Pre-approve permissions before launching background agent
        // Background agents auto-deny anything not pre-approved
        // AskUserQuestion tool calls fail (but agent continues)
        // MCP tools are NOT available in background agents
        config.BackgroundMode = true
        config.Tools = filterOut(config.Tools, mcpToolNames(config.Tools))
        go runFunc()
    } else {
        runFunc()
    }

    return agentID, nil
}
```

---

## 10. Background Task Behavior

### 10.1 Permission Pre-Approval

Before launching a background subagent, the caller should prompt for tool permissions upfront:
- Background agents inherit pre-approved permissions and **auto-deny** anything not approved
- `AskUserQuestion` tool calls **fail** (but the agent continues working)
- **MCP tools are not available** in background subagents

If a background subagent fails due to missing permissions, it can be **resumed in the foreground** via the `resume` field to retry with interactive prompts.

### 10.2 Output File

When `run_in_background: true`, the tool result includes an output file path:

```go
func (m *Manager) backgroundToolResult(agentID string) ToolOutput {
    outputPath := filepath.Join(m.outputDir, agentID+".output")
    return ToolOutput{
        Content: fmt.Sprintf("Background task started. Agent ID: %s\nOutput file: %s\nUse Read tool or `tail -f %s` to check output.",
            agentID, outputPath, outputPath),
    }
}
```

### 10.3 Ctrl+B Backgrounding

A foreground subagent can be moved to background mid-execution via `Ctrl+B` in the UI layer. This transitions the agent to background mode (auto-deny permissions, disable MCP tools).

### 10.4 Disable Background Tasks

Set `CLAUDE_CODE_DISABLE_BACKGROUND_TASKS=1` to disable all background task functionality.

---

## 11. Model Resolution

```go
func resolveModel(defModel string, inputModel *string, parentModel string) string {
    // Input takes precedence
    if inputModel != nil {
        return expandModelAlias(*inputModel)
    }
    // Then definition
    if defModel != "" && defModel != "inherit" {
        return expandModelAlias(defModel)
    }
    // Default: inherit parent
    return parentModel
}

func expandModelAlias(alias string) string {
    switch alias {
    case "sonnet": return "claude-sonnet-4-5-20250929"
    case "opus":   return "claude-opus-4-5-20250514"
    case "haiku":  return "claude-haiku-4-5-20251001"
    default:       return alias // already a full model string
    }
}
```

---

## 12. Permission Mode Resolution

```go
func resolvePermissionMode(defMode string, inputMode *string) string {
    // Input overrides definition
    if inputMode != nil && *inputMode != "" {
        return *inputMode
    }
    // Definition default
    if defMode != "" {
        return defMode
    }
    // Default: inherit parent's mode (handled by caller)
    return "default"
}
```

**Special case**: If the parent uses `bypassPermissions`, this takes precedence and **cannot** be overridden by the subagent definition.

---

## 13. Transcript Persistence

### 13.1 Storage

Subagent transcripts are stored as JSONL files:
```
~/.claude/projects/{project}/{sessionId}/subagents/agent-{agentId}.jsonl
```

### 13.2 Behavior

- Transcripts persist **independently** of main conversation compaction
- Resume loads full conversation history from transcript
- Automatic cleanup based on `cleanupPeriodDays` setting (default: 30 days)
- Compact boundary events are logged in transcript:

```json
{
  "type": "system",
  "subtype": "compact_boundary",
  "compactMetadata": {
    "trigger": "auto",
    "preTokens": 167189
  }
}
```

### 13.3 Auto-Compaction

Subagents use the same compaction logic as the main conversation (see Spec 09):
- Triggers at ~95% capacity by default
- Configurable via `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE` env var

---

## 14. Task Result Metrics

When a subagent completes, the Task tool result includes metrics:

```go
type TaskResult struct {
    Content    string       `json:"content"`     // final output text
    Metrics    *TaskMetrics `json:"metrics"`
    State      AgentState   `json:"state"`
    AgentID    string       `json:"agent_id"`    // for potential resume
}

type TaskMetrics struct {
    TokensUsed  int           `json:"tokens_used"`
    ToolUses    int           `json:"tool_uses"`
    Duration    time.Duration `json:"duration"`
}
```

---

## 15. TaskOutput (blocking read)

```go
func (m *Manager) GetOutput(taskID string, block bool, timeout time.Duration) (*TaskResult, error) {
    m.mu.RLock()
    ra, ok := m.active[taskID]
    m.mu.RUnlock()
    if !ok {
        return nil, fmt.Errorf("unknown task_id: %s", taskID)
    }

    if block {
        select {
        case <-ra.Done:
            // Agent completed
        case <-time.After(timeout):
            return &TaskResult{
                Content: ra.Output.String(),
                State:   ra.State,
            }, fmt.Errorf("timeout waiting for task %s", taskID)
        }
    }

    return &TaskResult{
        Content: ra.Output.String(),
        Metrics: ra.Metrics,
        State:   ra.State,
        AgentID: ra.ID,
    }, nil
}
```

---

## 16. TaskStop

```go
func (m *Manager) Stop(taskID string) error {
    m.mu.RLock()
    ra, ok := m.active[taskID]
    m.mu.RUnlock()
    if !ok {
        return fmt.Errorf("unknown task_id: %s", taskID)
    }

    ra.Cancel()
    ra.State = AgentStopped
    return nil
}
```

---

## 17. Resource Limits

```go
const (
    MaxConcurrentAgents  = 10    // max simultaneous subagents
    DefaultMaxTurns      = 50    // default turn limit for subagents
)

// NOTE: No MaxSubagentDepth — subagents CANNOT spawn other subagents (hard rule).

func (m *Manager) checkLimits() error {
    m.mu.RLock()
    defer m.mu.RUnlock()

    active := 0
    for _, ra := range m.active {
        if ra.State == AgentRunning { active++ }
    }
    if active >= MaxConcurrentAgents {
        return fmt.Errorf("max concurrent agents reached (%d)", MaxConcurrentAgents)
    }
    return nil
}
```

---

## 18. Disabling Specific Subagents

Subagents can be disabled via permission deny rules using `Task(subagent-name)` format:

```json
{
  "permissions": {
    "deny": ["Task(Explore)", "Task(my-custom-agent)"]
  }
}
```

Or via CLI: `--disallowedTools "Task(Explore)"`

---

## 19. Verification Checklist

- [ ] **AgentDefinition completeness**: All fields mapped (Name, Description, Tools, DisallowedTools, Prompt, Model, PermissionMode, MCPServers, CriticalReminder, Skills, MaxTurns, Memory, Hooks)
- [ ] **AgentInput completeness**: All fields from `sdk-tools.d.ts:32-73` mapped
- [ ] **Model resolution**: "sonnet"/"opus"/"haiku"/"inherit" expand correctly
- [ ] **Tool inheritance**: Nil tools -> inherit parent; explicit tools -> use those
- [ ] **Task(type) parsing**: `Task`, `Task(t1,t2)`, omitted Task all handled correctly
- [ ] **No nesting**: `CanSpawnSubagents: false` enforced on all subagent configs
- [ ] **Background execution**: `run_in_background` spawns goroutine, returns task ID
- [ ] **Background permissions**: Pre-approved only; AskUserQuestion fails; no MCP tools
- [ ] **TaskOutput blocking**: `block: true` waits for completion with timeout
- [ ] **TaskStop**: Cancels running agent via context
- [ ] **Task metrics**: TokensUsed, ToolUses, Duration returned in result
- [ ] **Hook lifecycle**: SubagentStart/SubagentStop fire; scoped hooks registered/cleaned up
- [ ] **Resource limits**: Max concurrent agents enforced
- [ ] **Resume support**: `resume` field loads transcript and continues
- [ ] **Output streaming**: Background agent output written incrementally to file
- [ ] **Memory**: Directory created, MEMORY.md loaded, Read/Write/Edit auto-enabled
- [ ] **Skills**: Full content injected into system prompt; not inherited from parent
- [ ] **Transcript persistence**: JSONL written, independent of main compaction, cleanup works
- [ ] **Auto-compaction**: Triggers at 95% capacity in subagent context
- [ ] **Disable deny**: `Task(name)` in permissions.deny blocks specific subagents
- [ ] **CLAUDE_CODE_DISABLE_BACKGROUND_TASKS**: Env var disables background functionality
