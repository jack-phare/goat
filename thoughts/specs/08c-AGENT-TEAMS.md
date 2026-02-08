# Spec 08c: Agent Teams

**Go Package**: `pkg/teams/`
**Status**: Experimental (`CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1`)
**Depends on**: Spec 08 (subagent manager), Spec 07 (hooks)
**Source References**:
- [Agent teams docs](https://code.claude.com/docs/en/agent-teams.md)
- Claude Code CHANGELOG v2.1.32-2.1.33

---

## 1. Purpose

Agent teams coordinate multiple **independent Claude Code sessions** working together. Unlike subagents (which run within a single session and report back to the caller), teammates are full sessions with their own context windows that can communicate directly with each other.

**Key distinctions from subagents:**

|                   | Subagents (Spec 08)                              | Agent Teams (this spec)                             |
|-------------------|---------------------------------------------------|-----------------------------------------------------|
| **Context**       | Own context window; results return to caller      | Own context window; fully independent               |
| **Communication** | Report results back to main agent only            | Teammates message each other directly               |
| **Coordination**  | Main agent manages all work                       | Shared task list with self-coordination             |
| **Best for**      | Focused tasks where only the result matters       | Complex work requiring discussion and collaboration |
| **Token cost**    | Lower: results summarized back to main context    | Higher: each teammate is a separate Claude instance |
| **Nesting**       | Subagents cannot spawn subagents                  | Teammates cannot spawn teams (no nested teams)      |

---

## 2. Architecture

An agent team consists of four components:

| Component      | Description |
|----------------|-------------|
| **Team Lead**  | The main Claude Code session that creates the team, spawns teammates, coordinates work |
| **Teammates**  | Separate Claude Code instances, each with their own context window |
| **Task List**  | Shared work items with dependency tracking and state management |
| **Mailbox**    | Messaging system for inter-agent communication (direct + broadcast) |

### 2.1 Storage

```
~/.claude/teams/{team-name}/config.json    — team config (members array)
~/.claude/tasks/{team-name}/               — shared task list
```

---

## 3. Go Types

### 3.1 Team

```go
// Team represents an active agent team.
type Team struct {
    mu          sync.RWMutex
    Name        string
    Lead        *TeamMember
    Members     map[string]*TeamMember  // keyed by agent ID
    Tasks       *SharedTaskList
    Mailbox     *Mailbox
    Config      TeamConfig
    CreatedAt   time.Time
    configPath  string
    tasksDir    string
}

type TeamConfig struct {
    Name    string         `json:"name"`
    Members []MemberConfig `json:"members"`
}

type MemberConfig struct {
    Name      string `json:"name"`
    AgentID   string `json:"agentId"`
    AgentType string `json:"agentType"`
}
```

### 3.2 Team Member

```go
// TeamMember represents a teammate (a separate Claude Code session).
type TeamMember struct {
    Name      string
    AgentID   string
    AgentType string
    State     MemberState
    Session   SessionHandle  // handle to the underlying Claude Code session
}

type MemberState int
const (
    MemberActive  MemberState = iota
    MemberIdle
    MemberStopped
)

// SessionHandle abstracts the underlying session management.
// Implementation depends on display mode (in-process vs tmux).
type SessionHandle interface {
    SendMessage(msg string) error
    RequestShutdown() error
    IsAlive() bool
}
```

### 3.3 Shared Task List

```go
// SharedTaskList provides file-locked concurrent task management.
type SharedTaskList struct {
    dir string  // ~/.claude/tasks/{team-name}/
}

type TeamTask struct {
    ID          string      `json:"id"`
    Subject     string      `json:"subject"`
    Description string      `json:"description"`
    Status      TaskStatus  `json:"status"`       // pending, in_progress, completed
    AssignedTo  string      `json:"assignedTo"`   // agent ID
    ClaimedBy   string      `json:"claimedBy"`    // agent ID (self-claim)
    DependsOn   []string    `json:"dependsOn"`    // task IDs
    CreatedBy   string      `json:"createdBy"`    // agent ID (usually lead)
    CreatedAt   time.Time   `json:"createdAt"`
    UpdatedAt   time.Time   `json:"updatedAt"`
}

type TaskStatus string
const (
    TaskPending    TaskStatus = "pending"
    TaskInProgress TaskStatus = "in_progress"
    TaskCompleted  TaskStatus = "completed"
)

func NewSharedTaskList(dir string) *SharedTaskList

func (stl *SharedTaskList) Create(task TeamTask) error
func (stl *SharedTaskList) Claim(taskID, agentID string) error  // file-locked
func (stl *SharedTaskList) Complete(taskID string) error
func (stl *SharedTaskList) List() ([]TeamTask, error)
func (stl *SharedTaskList) GetUnblocked() ([]TeamTask, error)   // pending tasks with all deps completed
```

### 3.4 Mailbox

```go
// Mailbox handles inter-agent messaging.
type Mailbox struct {
    mu       sync.Mutex
    messages map[string][]Message  // keyed by recipient agent ID
}

type Message struct {
    From      string    `json:"from"`       // sender agent ID
    To        string    `json:"to"`         // recipient agent ID ("*" for broadcast)
    Content   string    `json:"content"`
    Timestamp time.Time `json:"timestamp"`
}

func NewMailbox() *Mailbox

func (mb *Mailbox) Send(msg Message) error
func (mb *Mailbox) Broadcast(from string, content string, members []string) error
func (mb *Mailbox) Receive(agentID string) []Message  // returns and clears pending messages
```

---

## 4. Team Manager

```go
// TeamManager handles team lifecycle.
type TeamManager struct {
    mu           sync.RWMutex
    activeTeam   *Team              // one team per session
    displayMode  DisplayMode
    hooks        *hooks.Runner
}

type DisplayMode string
const (
    DisplayAuto      DisplayMode = "auto"      // tmux if in tmux session, else in-process
    DisplayInProcess DisplayMode = "in-process" // all in main terminal
    DisplayTmux      DisplayMode = "tmux"       // each teammate in own pane (tmux or iTerm2)
)

func NewTeamManager(hooks *hooks.Runner, displayMode DisplayMode) *TeamManager

func (tm *TeamManager) CreateTeam(ctx context.Context, name string) (*Team, error)
func (tm *TeamManager) SpawnTeammate(ctx context.Context, name, agentType, prompt string) (*TeamMember, error)
func (tm *TeamManager) ShutdownTeammate(ctx context.Context, agentID string) error
func (tm *TeamManager) SendMessage(ctx context.Context, from, to, content string) error
func (tm *TeamManager) Broadcast(ctx context.Context, from, content string) error
func (tm *TeamManager) Cleanup(ctx context.Context) error
func (tm *TeamManager) GetTeam() *Team
```

---

## 5. Team Lifecycle

### 5.1 Create Team

```go
func (tm *TeamManager) CreateTeam(ctx context.Context, name string) (*Team, error) {
    tm.mu.Lock()
    defer tm.mu.Unlock()

    if tm.activeTeam != nil {
        return nil, fmt.Errorf("a team is already active; clean up the current team first")
    }

    configPath := filepath.Join(os.UserHomeDir(), ".claude", "teams", name, "config.json")
    tasksDir := filepath.Join(os.UserHomeDir(), ".claude", "tasks", name)

    // Create directories
    os.MkdirAll(filepath.Dir(configPath), 0o755)
    os.MkdirAll(tasksDir, 0o755)

    team := &Team{
        Name:       name,
        Members:    make(map[string]*TeamMember),
        Tasks:      NewSharedTaskList(tasksDir),
        Mailbox:    NewMailbox(),
        CreatedAt:  time.Now(),
        configPath: configPath,
        tasksDir:   tasksDir,
    }

    tm.activeTeam = team
    return team, team.saveConfig()
}
```

### 5.2 Spawn Teammate

```go
func (tm *TeamManager) SpawnTeammate(ctx context.Context, name, agentType, prompt string) (*TeamMember, error) {
    tm.mu.Lock()
    defer tm.mu.Unlock()

    if tm.activeTeam == nil {
        return nil, fmt.Errorf("no active team; create a team first")
    }

    // Create a new Claude Code session for this teammate
    agentID := uuid.New().String()
    session, err := tm.createSession(ctx, agentID, agentType, prompt)
    if err != nil {
        return nil, err
    }

    member := &TeamMember{
        Name:      name,
        AgentID:   agentID,
        AgentType: agentType,
        State:     MemberActive,
        Session:   session,
    }

    tm.activeTeam.Members[agentID] = member
    tm.activeTeam.saveConfig()

    return member, nil
}
```

### 5.3 Shutdown Teammate

The lead sends a shutdown request. The teammate can approve (exits gracefully) or reject with an explanation.

```go
func (tm *TeamManager) ShutdownTeammate(ctx context.Context, agentID string) error {
    tm.mu.RLock()
    member, ok := tm.activeTeam.Members[agentID]
    tm.mu.RUnlock()
    if !ok {
        return fmt.Errorf("unknown teammate: %s", agentID)
    }

    if err := member.Session.RequestShutdown(); err != nil {
        return err
    }

    member.State = MemberStopped

    // Auto-notify lead
    tm.activeTeam.Mailbox.Send(Message{
        From:      agentID,
        To:        tm.activeTeam.Lead.AgentID,
        Content:   fmt.Sprintf("Teammate %s has shut down.", member.Name),
        Timestamp: time.Now(),
    })

    return nil
}
```

### 5.4 Cleanup

Cleanup removes shared team resources. Fails if any teammates are still running.

```go
func (tm *TeamManager) Cleanup(ctx context.Context) error {
    tm.mu.Lock()
    defer tm.mu.Unlock()

    if tm.activeTeam == nil {
        return fmt.Errorf("no active team")
    }

    // Check for active teammates
    for _, member := range tm.activeTeam.Members {
        if member.State == MemberActive && member.Session.IsAlive() {
            return fmt.Errorf("teammate %s is still active; shut down all teammates before cleanup", member.Name)
        }
    }

    // Remove team config and task files
    os.RemoveAll(filepath.Dir(tm.activeTeam.configPath))
    os.RemoveAll(tm.activeTeam.tasksDir)

    tm.activeTeam = nil
    return nil
}
```

---

## 6. Task Claiming (File-Locked)

Task claiming uses file locking to prevent race conditions when multiple teammates try to claim the same task simultaneously.

```go
func (stl *SharedTaskList) Claim(taskID, agentID string) error {
    lockPath := filepath.Join(stl.dir, taskID+".lock")

    // Acquire file lock
    lock, err := acquireFileLock(lockPath)
    if err != nil {
        return fmt.Errorf("failed to acquire lock for task %s: %w", taskID, err)
    }
    defer lock.Release()

    // Read current state
    task, err := stl.readTask(taskID)
    if err != nil {
        return err
    }

    // Verify task is claimable
    if task.Status != TaskPending {
        return fmt.Errorf("task %s is not pending (status: %s)", taskID, task.Status)
    }
    if task.ClaimedBy != "" {
        return fmt.Errorf("task %s already claimed by %s", taskID, task.ClaimedBy)
    }

    // Check dependencies
    for _, depID := range task.DependsOn {
        dep, err := stl.readTask(depID)
        if err != nil {
            return fmt.Errorf("dependency %s: %w", depID, err)
        }
        if dep.Status != TaskCompleted {
            return fmt.Errorf("task %s blocked by incomplete dependency %s", taskID, depID)
        }
    }

    // Claim it
    task.ClaimedBy = agentID
    task.Status = TaskInProgress
    task.UpdatedAt = time.Now()
    return stl.writeTask(task)
}
```

---

## 7. Messaging

### 7.1 Direct Message

Send a message to one specific teammate:

```go
func (tm *TeamManager) SendMessage(ctx context.Context, from, to, content string) error {
    tm.mu.RLock()
    team := tm.activeTeam
    tm.mu.RUnlock()

    if team == nil {
        return fmt.Errorf("no active team")
    }

    msg := Message{
        From:      from,
        To:        to,
        Content:   content,
        Timestamp: time.Now(),
    }

    return team.Mailbox.Send(msg)
}
```

### 7.2 Broadcast

Send to all teammates simultaneously. Use sparingly — costs scale with team size.

```go
func (tm *TeamManager) Broadcast(ctx context.Context, from, content string) error {
    tm.mu.RLock()
    team := tm.activeTeam
    tm.mu.RUnlock()

    if team == nil {
        return fmt.Errorf("no active team")
    }

    var memberIDs []string
    for id := range team.Members {
        if id != from { // don't send to self
            memberIDs = append(memberIDs, id)
        }
    }

    return team.Mailbox.Broadcast(from, content, memberIDs)
}
```

### 7.3 Message Delivery

Messages are delivered automatically to recipients. The lead doesn't need to poll. When a teammate finishes and goes idle, it automatically notifies the lead.

---

## 8. Display Modes

### 8.1 In-Process (default)

All teammates run inside the main terminal:
- `Shift+Up/Down` — select a teammate
- `Enter` — view teammate's session
- `Escape` — interrupt current turn
- `Ctrl+T` — toggle task list

### 8.2 Split Pane (tmux / iTerm2)

Each teammate gets its own pane. Click into a pane to interact directly.

Configuration via `settings.json`:
```json
{
  "teammateMode": "in-process"  // or "auto" or "tmux"
}
```

Or CLI flag:
```bash
claude --teammate-mode in-process
```

`auto` (default) uses split panes if inside a tmux session, otherwise in-process.

### 8.3 Session Creation (display-mode dependent)

```go
func (tm *TeamManager) createSession(ctx context.Context, agentID, agentType, prompt string) (SessionHandle, error) {
    switch tm.displayMode {
    case DisplayInProcess:
        return newInProcessSession(ctx, agentID, agentType, prompt)
    case DisplayTmux:
        return newTmuxSession(ctx, agentID, agentType, prompt)
    default: // auto
        if insideTmux() {
            return newTmuxSession(ctx, agentID, agentType, prompt)
        }
        return newInProcessSession(ctx, agentID, agentType, prompt)
    }
}

func insideTmux() bool {
    return os.Getenv("TMUX") != ""
}
```

---

## 9. Delegate Mode

Restricts the lead to coordination-only tools. Prevents the lead from implementing tasks itself instead of delegating.

Available tools in delegate mode:
- Spawn/manage teammates
- Send/broadcast messages
- Create/assign/complete tasks
- Shut down teammates

Activated by pressing `Shift+Tab` after starting a team.

```go
var DelegateModeTools = []string{
    "SpawnTeammate",
    "ShutdownTeammate",
    "SendMessage",
    "Broadcast",
    "TaskCreate",
    "TaskUpdate",
    "TaskList",
    "CleanupTeam",
}
```

---

## 10. Permissions

- All teammates start with the lead's permission settings
- If lead uses `--dangerously-skip-permissions`, all teammates do too
- Individual teammate modes can be changed **after** spawning
- Cannot set per-teammate modes at spawn time

---

## 11. Hook Events

Two new hook events for agent teams:

### 11.1 TeammateIdle

Fires when a teammate is about to go idle (finished current work). Exit code 2 sends feedback and keeps the teammate working.

```json
{
  "hooks": {
    "TeammateIdle": [
      {
        "hooks": [
          { "type": "command", "command": "./scripts/check-remaining-work.sh" }
        ]
      }
    ]
  }
}
```

### 11.2 TaskCompleted

Fires when a task is being marked complete. Exit code 2 prevents completion and sends feedback.

```json
{
  "hooks": {
    "TaskCompleted": [
      {
        "hooks": [
          { "type": "command", "command": "./scripts/verify-task-quality.sh" }
        ]
      }
    ]
  }
}
```

---

## 12. Plan Approval for Teammates

Teammates can be required to plan before implementing. The lead controls approval:

1. Teammate enters plan mode (read-only)
2. Teammate finishes planning, sends plan approval request to lead
3. Lead reviews plan:
   - **Approves**: Teammate exits plan mode, begins implementation
   - **Rejects with feedback**: Teammate stays in plan mode, revises, resubmits

The lead makes approval decisions autonomously based on criteria in the prompt.

---

## 13. Constraints

| Constraint | Details |
|------------|---------|
| One team per session | Clean up current team before creating a new one |
| No nested teams | Teammates cannot spawn their own teams |
| Lead is fixed | Cannot promote a teammate to lead or transfer leadership |
| Permissions at spawn | All teammates inherit lead's mode; change individually after |
| No session resumption | `/resume` and `/rewind` do not restore in-process teammates |
| Split panes | Requires tmux or iTerm2; not supported in VS Code terminal, Windows Terminal, Ghostty |
| Cleanup must be from lead | Teammates should not run cleanup |

---

## 14. Context and Communication

- Each teammate loads the same project context as a regular session: CLAUDE.md, MCP servers, skills
- Teammate also receives the spawn prompt from the lead
- Lead's conversation history does **NOT** carry over
- Automatic message delivery (no polling needed)
- Idle notifications sent automatically to lead
- Shared task list visible to all agents
- Dependency auto-unblocking: completing a task unblocks dependents automatically

---

## 15. Token Usage

Agent teams use significantly more tokens than a single session. Each teammate has its own context window, and token usage scales with the number of active teammates.

**Guidelines:**
- Worth it for: research, review, new feature work with parallel exploration
- Not worth it for: routine tasks, sequential work
- Use subagents (Spec 08) when only the result matters, not the inter-agent discussion

---

## 16. Verification Checklist

- [ ] **Team creation**: Creates config.json and tasks directory
- [ ] **Teammate spawning**: Creates separate sessions (in-process or tmux)
- [ ] **Teammate shutdown**: Graceful request + confirmation; auto-notify lead
- [ ] **Team cleanup**: Fails if active teammates; removes config + tasks
- [ ] **One team per session**: Creating a second team fails
- [ ] **No nested teams**: Teammates cannot call CreateTeam
- [ ] **Shared task list**: Create, claim, complete with file locking
- [ ] **Task dependencies**: Blocked tasks can't be claimed; auto-unblock on completion
- [ ] **Task claiming**: File-locked to prevent race conditions
- [ ] **Messaging**: Direct send + broadcast; auto-delivery (no polling)
- [ ] **Idle notification**: Automatic notify to lead when teammate goes idle
- [ ] **Display modes**: auto/in-process/tmux all work
- [ ] **Delegate mode**: Restricts lead to coordination tools
- [ ] **Permissions**: Inherit from lead; changeable after spawn
- [ ] **Plan approval**: Plan → review → approve/reject loop works
- [ ] **TeammateIdle hook**: Fires correctly; exit code 2 keeps teammate working
- [ ] **TaskCompleted hook**: Fires correctly; exit code 2 prevents completion
- [ ] **Config persistence**: `~/.claude/teams/{name}/config.json` written correctly
- [ ] **CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS**: Feature gated behind env var
- [ ] **Context loading**: Teammates load CLAUDE.md, MCP servers, skills independently
- [ ] **Lead history isolation**: Spawn prompt only; no conversation history
