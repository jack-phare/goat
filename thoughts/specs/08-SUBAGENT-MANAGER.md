# Spec 08: Subagent & Task Manager

**Go Package**: `pkg/subagent/`
**Source References**:
- `sdk.d.ts:33-67` — AgentDefinition (description, tools, disallowedTools, prompt, model, mcpServers, skills, maxTurns, criticalSystemReminder_EXPERIMENTAL)
- `sdk-tools.d.ts:32-73` — AgentInput (description, prompt, subagent_type, model, resume, run_in_background, max_turns, name, team_name, mode)
- `sdk-tools.d.ts:113-126` — TaskOutputInput (task_id, block, timeout)
- `sdk-tools.d.ts:272-279` — TaskStopInput (task_id)
- `sdk.d.ts:479-497` — Options.agent, Options.agents
- Piebald-AI: "Agent Prompt: Task tool (294 tks)", "Agent Prompt: Explore (516 tks)", "Agent Prompt: Plan mode (enhanced) (633 tks)"

---

## 1. Purpose

The subagent manager handles spawning, lifecycle, and communication for child agent loops. When the main agent uses the `Agent` (Task) tool, a new agentic loop is created with its own system prompt, tool restrictions, and model configuration.

---

## 2. Agent Definition (from `sdk.d.ts:33-67`)

```go
// AgentDefinition configures a named subagent type.
type AgentDefinition struct {
    Description          string   `json:"description"`
    Tools                []string `json:"tools,omitempty"`          // allowed tools (nil = inherit all)
    DisallowedTools      []string `json:"disallowedTools,omitempty"`
    Prompt               string   `json:"prompt"`                   // system prompt
    Model                string   `json:"model,omitempty"`          // "sonnet"|"opus"|"haiku"|"inherit"
    MCPServers           []AgentMCPServerSpec `json:"mcpServers,omitempty"`
    CriticalReminder     string   `json:"criticalSystemReminder_EXPERIMENTAL,omitempty"`
    Skills               []string `json:"skills,omitempty"`         // skill names to preload
    MaxTurns             *int     `json:"maxTurns,omitempty"`       // agentic turn limit
}

type AgentMCPServerSpec struct {
    ServerName string   `json:"serverName"`
    Tools      []string `json:"tools,omitempty"` // subset of server's tools
}
```

---

## 3. Built-in Subagent Types

### 3.1 Task Agent (default)
- **Prompt**: "Agent Prompt: Task tool (294 tks)" from Piebald
- **Purpose**: General delegated task execution
- **Tools**: Inherits from parent (unless restricted)
- **MaxTurns**: Inherited from parent or explicit

### 3.2 Explore Agent
- **Prompt**: "Agent Prompt: Explore (516 tks)"
- **Purpose**: Read-only codebase exploration
- **Tools**: `["FileRead", "Glob", "Grep"]` (read-only subset)
- **MaxTurns**: Lower limit

### 3.3 Plan Agent
- **Prompt**: "Agent Prompt: Plan mode (enhanced) (633 tks)"
- **Purpose**: Planning without execution
- **Tools**: None (plan mode)
- **MaxTurns**: Lower limit

### 3.4 Custom Agents
Defined via `Options.agents` map. Users define arbitrary agent types with custom prompts, tool restrictions, and models.

---

## 4. Go Types

### 4.1 Manager

```go
// Manager tracks active subagents and their lifecycles.
type Manager struct {
    mu       sync.RWMutex
    agents   map[string]AgentDefinition   // registered agent types
    active   map[string]*RunningAgent     // active agent instances by ID
    parent   *agent.LoopState             // parent loop state reference
    hooks    *hooks.Runner
}

func NewManager(agents map[string]AgentDefinition, hooks *hooks.Runner) *Manager

func (m *Manager) Spawn(ctx context.Context, input AgentInput) (string, error)  // returns agent ID
func (m *Manager) GetOutput(taskID string, block bool, timeout time.Duration) (string, error)
func (m *Manager) Stop(taskID string) error
func (m *Manager) List() []AgentStatus
```

### 4.2 Running Agent

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
```

---

## 5. Spawn Flow

```go
func (m *Manager) Spawn(ctx context.Context, input AgentInput) (string, error) {
    // 1. Resolve agent definition
    def, ok := m.agents[input.SubagentType]
    if !ok {
        return "", fmt.Errorf("unknown subagent_type: %s", input.SubagentType)
    }

    // 2. Generate agent ID
    agentID := uuid.New().String()
    if input.Resume != nil {
        agentID = *input.Resume // resume existing agent
    }

    // 3. Fire SubagentStart hook
    m.hooks.Fire(ctx, hooks.HookSubagentStart, &hooks.SubagentStartHookInput{
        AgentID:   agentID,
        AgentType: input.SubagentType,
    }, "")

    // 4. Resolve model
    model := resolveModel(def.Model, input.Model, m.parent.Model)

    // 5. Build subagent config
    config := agent.AgentConfig{
        Model:         model,
        SystemPrompt:  prompt.AssembleSubagentPrompt(def, m.parentConfig()),
        MaxTurns:      resolveMaxTurns(def.MaxTurns, input.MaxTurns),
        Tools:         resolveTools(def.Tools, def.DisallowedTools),
        PermissionMode: resolvePermissionMode(input.Mode),
        CWD:           m.parent.CWD,
        SessionID:     agentID,
        PersistSession: true,
    }

    // 6. Create running agent
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

    m.mu.Lock()
    m.active[agentID] = ra
    m.mu.Unlock()

    // 7. Run the subagent loop
    runFunc := func() {
        defer close(ra.Done)
        defer func() {
            // Fire SubagentStop hook
            m.hooks.Fire(ctx, hooks.HookSubagentStop, &hooks.SubagentStopHookInput{
                AgentID:   agentID,
                AgentType: input.SubagentType,
            }, "")
        }()

        query := agent.RunLoop(input.Prompt, config)
        for msg := range query.Messages {
            ra.Output.Append(msg)
        }
        ra.State = AgentCompleted
    }

    if input.RunInBackground != nil && *input.RunInBackground {
        go runFunc()
    } else {
        runFunc()
    }

    return agentID, nil
}
```

---

## 6. Model Resolution

```go
// resolveModel picks the concrete model string from agent definition and input.
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

## 7. Tool Resolution

```go
func resolveTools(allowed []string, disallowed []string) []string {
    if allowed == nil {
        // Inherit all tools, minus disallowed
        all := registry.AllToolNames()
        return filterOut(all, disallowed)
    }
    return filterOut(allowed, disallowed)
}
```

---

## 8. Background Task Management

### 8.1 TaskOutput (blocking read)

```go
func (m *Manager) GetOutput(taskID string, block bool, timeout time.Duration) (string, error) {
    m.mu.RLock()
    ra, ok := m.active[taskID]
    m.mu.RUnlock()
    if !ok {
        return "", fmt.Errorf("unknown task_id: %s", taskID)
    }

    if block {
        select {
        case <-ra.Done:
            // Agent completed
        case <-time.After(timeout):
            return ra.Output.String(), fmt.Errorf("timeout waiting for task %s", taskID)
        }
    }

    return ra.Output.String(), nil
}
```

### 8.2 TaskStop

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

## 9. Agent Output File (for background tasks)

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

The output file is written incrementally as the subagent produces messages.

---

## 10. Resource Limits

```go
const (
    MaxConcurrentAgents  = 10    // max simultaneous subagents
    DefaultMaxTurns      = 50    // default turn limit for subagents
    MaxSubagentDepth     = 3     // max nesting depth (agent spawning agent)
)

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

## 11. Verification Checklist

- [ ] **AgentDefinition completeness**: All fields from `sdk.d.ts:33-67` mapped
- [ ] **AgentInput completeness**: All fields from `sdk-tools.d.ts:32-73` mapped
- [ ] **Model resolution**: "sonnet"/"opus"/"haiku"/"inherit" expand correctly
- [ ] **Tool inheritance**: Nil tools → inherit parent; explicit tools → use those
- [ ] **Background execution**: `run_in_background` spawns goroutine, returns task ID
- [ ] **TaskOutput blocking**: `block: true` waits for completion with timeout
- [ ] **TaskStop**: Cancels running agent via context
- [ ] **Hook lifecycle**: SubagentStart fires before loop, SubagentStop fires after
- [ ] **Resource limits**: Max concurrent agents enforced
- [ ] **Resume support**: `resume` field restarts agent from previous transcript
- [ ] **Nesting depth**: Prevents infinite agent-spawning-agent chains
- [ ] **Output streaming**: Background agent output written incrementally to file
