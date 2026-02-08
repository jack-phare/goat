# Agent Teams Implementation Plan

## Overview

Implement `pkg/teams/` — a coordination layer for multiple independent Claude Code sessions (OS processes) working together on a shared project. Unlike subagents (which run within a single session), teammates are separate `goat` processes that communicate via a file-based mailbox and coordinate via a file-locked shared task list.

**Architecture**: All processes run inside a single sandbox. The team lead spawns teammates via `exec.Command` self-invocation (`os.Args[0] --teammate`). Shared state lives on the filesystem at `~/.claude/teams/{name}/` and `~/.claude/tasks/{name}/`.

## Current State

### Exists:
- `pkg/subagent/` — Complete subagent manager (87 tests), provides lifecycle patterns
- `pkg/agent/` — Agentic loop with `RunLoop`, `Query`, `AgentConfig` (24 tests)
- `pkg/hooks/` — Hook system with `TeammateIdle` and `TaskCompleted` events already defined
- `pkg/hooks/types.go` — `TeammateIdleHookInput` and `TaskCompletedHookInput` already defined
- `pkg/types/options.go` — `HookEventTeammateIdle` and `HookEventTaskCompleted` constants
- `pkg/prompt/prompts/` — 6 team-related prompt files already embedded:
  - `tools/tool-description-teammatetool.md` (TeamCreate)
  - `tools/tool-description-sendmessagetool.md` (SendMessage)
  - `tools/tool-description-teamdelete.md` (TeamDelete)
  - `reminders/system-reminder-team-coordination.md`
  - `reminders/system-reminder-team-shutdown.md`
  - `system/system-prompt-teammate-communication.md`

### Does NOT exist:
- `pkg/teams/` package
- Team tools (TeamCreate, SendMessage, TeamDelete) in `pkg/tools/`
- File-locking infrastructure
- File-based mailbox system
- Teammate process spawning (exec.Command)
- Delegate mode enforcement
- Feature gate (`CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1`)

## Desired End State

- `pkg/teams/` package with full team lifecycle management
- File-locked shared task list for cross-process coordination
- File-based mailbox with fsnotify-powered message delivery
- Teammate spawning via `exec.Command` self-invocation
- 3 new tools: TeamCreate, SendMessage, TeamDelete
- TeammateIdle and TaskCompleted hook integration
- Delegate mode restricting lead to coordination-only tools
- Feature gated behind `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS` env var
- 100+ tests with `-race` flag passing

## Phases

### Phase 1: Core Types and Shared Task List

**Goal**: Implement the file-based shared task list with file locking — the foundation that all teammates interact with.

**New dependency**: `github.com/gofrs/flock`

**Changes**:

1. **`pkg/teams/task.go`** — Shared task list types and operations
   ```go
   type TeamTask struct {
       ID          string     `json:"id"`
       Subject     string     `json:"subject"`
       Description string     `json:"description"`
       Status      TaskStatus `json:"status"`
       AssignedTo  string     `json:"assignedTo"`
       ClaimedBy   string     `json:"claimedBy"`
       DependsOn   []string   `json:"dependsOn"`
       CreatedBy   string     `json:"createdBy"`
       CreatedAt   time.Time  `json:"createdAt"`
       UpdatedAt   time.Time  `json:"updatedAt"`
   }

   type SharedTaskList struct {
       dir string
   }

   func NewSharedTaskList(dir string) *SharedTaskList
   func (stl *SharedTaskList) Create(task TeamTask) error        // write task JSON file
   func (stl *SharedTaskList) Claim(taskID, agentID string) error // file-locked claim
   func (stl *SharedTaskList) Complete(taskID string) error       // mark completed
   func (stl *SharedTaskList) List() ([]TeamTask, error)          // read all tasks
   func (stl *SharedTaskList) GetUnblocked() ([]TeamTask, error)  // pending + deps met
   ```
   - Each task stored as `{dir}/{taskID}.json`
   - File locking via `gofrs/flock` on `{dir}/{taskID}.lock`
   - `Claim()` acquires lock → reads → validates pending+unclaimed+deps → writes → releases
   - `Complete()` acquires lock → reads → sets completed → writes → releases

2. **`pkg/teams/task_test.go`** — Tests for shared task list
   - Create/List/Complete lifecycle
   - Claim race condition (two goroutines claim same task)
   - Dependency blocking (can't claim if deps incomplete)
   - Claim idempotency (already-claimed returns error)
   - GetUnblocked filtering

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/teams/... -race -run TestTask` passes (15 tests)
  - [x] `go vet ./pkg/teams/...` clean
- Manual:
  - [x] File locking verified with concurrent goroutines (TestTaskConcurrentClaim)

---

### Phase 2: File-Based Mailbox

**Goal**: Implement the file-based mailbox system for inter-process messaging.

**Changes**:

1. **`pkg/teams/mailbox.go`** — File-based mailbox
   ```go
   type Message struct {
       ID        string    `json:"id"`
       From      string    `json:"from"`
       To        string    `json:"to"`
       Content   string    `json:"content"`
       Summary   string    `json:"summary"`
       Type      string    `json:"type"` // message, broadcast, shutdown_request, etc.
       Timestamp time.Time `json:"timestamp"`
   }

   type Mailbox struct {
       dir string // ~/.claude/teams/{team-name}/mailbox/
   }

   func NewMailbox(dir string) *Mailbox
   func (mb *Mailbox) Send(msg Message) error               // write to recipient's inbox dir
   func (mb *Mailbox) Broadcast(from, content string, recipients []string) error
   func (mb *Mailbox) Receive(agentName string) ([]Message, error) // read + delete from inbox
   func (mb *Mailbox) Watch(ctx context.Context, agentName string) (<-chan Message, error) // fsnotify
   ```
   - Directory structure: `{dir}/{agentName}/{timestamp}-{uuid}.json`
   - `Send()`: write JSON file to recipient's directory
   - `Receive()`: read all files, parse, delete, return sorted by timestamp
   - `Watch()`: use `fsnotify` to watch recipient directory, emit on channel
   - Broadcast: `Send()` to each recipient

2. **`pkg/teams/mailbox_test.go`** — Tests for mailbox
   - Send/Receive lifecycle
   - Broadcast to multiple recipients
   - Watch delivers new messages via channel
   - Concurrent send from multiple goroutines
   - Receive clears messages (no double-delivery)

**New dependency**: `github.com/fsnotify/fsnotify` (for `Watch()`)

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/teams/... -race -run TestMailbox` passes (11 tests)
- Manual:
  - [x] fsnotify watcher verified with file creation (TestMailboxWatch, TestMailboxWatchMultipleMessages)

---

### Phase 3: Team Config and Team Manager Core

**Goal**: Implement team lifecycle management — create, track members, cleanup.

**Changes**:

1. **`pkg/teams/team.go`** — Team and config types
   ```go
   type Team struct {
       mu         sync.RWMutex
       Name       string
       Lead       *TeamMember
       Members    map[string]*TeamMember // keyed by name
       Tasks      *SharedTaskList
       Mailbox    *Mailbox
       Config     TeamConfig
       CreatedAt  time.Time
       configPath string
       tasksDir   string
       mailboxDir string
   }

   type TeamConfig struct {
       Name    string         `json:"name"`
       Members []MemberConfig `json:"members"`
   }

   type MemberConfig struct {
       Name      string `json:"name"`
       AgentID   string `json:"agentId"`
       AgentType string `json:"agentType"`
       PID       int    `json:"pid,omitempty"` // OS process ID
   }

   type TeamMember struct {
       Name      string
       AgentID   string
       AgentType string
       State     MemberState
       PID       int    // OS process ID
       Process   *os.Process
   }

   type MemberState int // MemberActive, MemberIdle, MemberStopped
   ```

2. **`pkg/teams/manager.go`** — Team manager
   ```go
   type TeamManager struct {
       mu          sync.RWMutex
       activeTeam  *Team
       hooks       agent.HookRunner
       baseDir     string  // ~/.claude/
   }

   func NewTeamManager(hooks agent.HookRunner, baseDir string) *TeamManager
   func (tm *TeamManager) CreateTeam(ctx context.Context, name string) (*Team, error)
   func (tm *TeamManager) GetTeam() *Team
   func (tm *TeamManager) Cleanup(ctx context.Context) error
   ```
   - `CreateTeam()`: creates directories, writes config.json, initializes task list + mailbox
   - `Cleanup()`: verifies no active members, removes directories
   - One team per manager instance

3. **`pkg/teams/manager_test.go`** — Tests
   - Create team creates directories and config
   - One team per session constraint
   - Cleanup fails with active members
   - Cleanup removes directories

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/teams/... -race -run TestManager` passes (10 tests + 5 Team/Member tests)
- Manual:
  - [x] Config file written correctly to temp directory (TestManagerCreateTeam, TestTeamAddMember)

---

### Phase 4: Teammate Spawning (exec.Command)

**Goal**: Spawn teammate processes via `exec.Command` self-invocation.

**Changes**:

1. **`pkg/teams/spawn.go`** — Process spawning
   ```go
   func (tm *TeamManager) SpawnTeammate(ctx context.Context, name, agentType, prompt string) (*TeamMember, error)
   ```
   - Builds command: `os.Args[0] --teammate --team-name {name} --agent-name {agentName} --agent-type {type}`
   - Sets environment: `CLAUDE_CODE_TEAM={teamName}`, `CLAUDE_CODE_AGENT_NAME={name}`
   - Starts process, captures PID
   - Writes initial prompt to teammate's mailbox as first message
   - Updates team config with new member
   - Returns `*TeamMember` with `PID` and `Process`

2. **`pkg/teams/spawn_test.go`** — Tests
   - Uses a test helper binary (TestMain pattern) for subprocess testing
   - Spawn creates process and updates config
   - Spawn with unknown agent type fails
   - Environment variables set correctly

3. **`pkg/teams/shutdown.go`** — Teammate shutdown
   ```go
   func (tm *TeamManager) ShutdownTeammate(ctx context.Context, name string) error
   func (tm *TeamManager) RequestShutdown(ctx context.Context, name string) error
   ```
   - `RequestShutdown()`: sends shutdown_request message via mailbox
   - `ShutdownTeammate()`: sends SIGTERM, waits with timeout, SIGKILL if needed
   - Updates member state and config
   - Auto-notifies lead via mailbox

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/teams/... -race -run TestSpawn` passes (8 tests)
  - [x] `go test ./pkg/teams/... -race -run TestShutdown` passes (8 tests)
- Manual:
  - [x] Verify subprocess spawned and PID tracked (via SpawnTeammateWithFunc + fakeSpawnFunc)

---

### Phase 5: Team Tools

**Goal**: Implement the 3 team tools and register them in the tool registry.

**Changes**:

1. **`pkg/tools/teamcreate.go`** — TeamCreate tool
   - Input: `team_name`, `description`
   - Calls `TeamManager.CreateTeam()`
   - Returns confirmation with team config path
   - Feature-gated: returns error if `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS` not set

2. **`pkg/tools/sendmessage.go`** — SendMessage tool
   - Input: `type` (message|broadcast|shutdown_request|shutdown_response|plan_approval_response), `recipient`, `content`, `summary`, `request_id`, `approve`
   - Routes to appropriate `TeamManager` method based on type
   - Validates recipient exists in team
   - For shutdown_response with approve=true: signals process to exit

3. **`pkg/tools/teamdelete.go`** — TeamDelete tool
   - Input: none (uses active team from manager)
   - Calls `TeamManager.Cleanup()`
   - Returns confirmation

4. **`pkg/tools/teamcreate_test.go`**, **`pkg/tools/sendmessage_test.go`**, **`pkg/tools/teamdelete_test.go`** — Tests for each tool

5. **`pkg/tools/registry.go`** — Register team tools
   - Add TeamCreate, SendMessage, TeamDelete to default registration
   - Only register when feature flag is set

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/tools/... -race -run TestTeam` passes (13 tests)
  - [x] `go test ./pkg/tools/... -race -run TestSendMessage` passes (11 tests)
  - [x] `go test ./pkg/tools/... -race -run TestTeamDelete` passes (6 tests)
- Manual:
  - [x] Feature gate prevents tool registration when env var not set (conditional in defaults.go)

---

### Phase 6: Hook Integration and Delegate Mode

**Goal**: Wire up TeammateIdle and TaskCompleted hooks, implement delegate mode.

**Changes**:

1. **`pkg/teams/hooks.go`** — Hook firing helpers
   ```go
   func (tm *TeamManager) fireTeammateIdle(ctx context.Context, teammateName string) ([]agent.HookResult, error)
   func (tm *TeamManager) fireTaskCompleted(ctx context.Context, taskID, taskSubject, teammateName string) ([]agent.HookResult, error)
   ```
   - `fireTeammateIdle()`: fires `HookEventTeammateIdle` with `TeammateIdleHookInput`
   - `fireTaskCompleted()`: fires `HookEventTaskCompleted` with `TaskCompletedHookInput`
   - Exit code 2 handling: TeammateIdle keeps teammate working, TaskCompleted prevents completion

2. **`pkg/teams/delegate.go`** — Delegate mode
   ```go
   var DelegateModeTools = []string{
       "TeamCreate", "SendMessage", "TeamDelete",
       "TaskCreate", "TaskUpdate", "TaskList", "TaskGet",
   }

   func (tm *TeamManager) EnableDelegateMode() []string  // returns tool whitelist
   func (tm *TeamManager) DisableDelegateMode()
   func (tm *TeamManager) IsDelegateMode() bool
   ```
   - When enabled, only DelegateModeTools are available to the lead
   - Integrated with tool registry filtering

3. **`pkg/teams/hooks_test.go`**, **`pkg/teams/delegate_test.go`** — Tests

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/teams/... -race -run TestHook` passes (6 tests)
  - [x] `go test ./pkg/teams/... -race -run TestDelegate` passes (8 tests)
- Manual:
  - [x] Delegate mode restricts available tools (TestDelegateModeFilterToolsActive)

---

### Phase 7: Teammate Mode Entry Point

**Goal**: Support `--teammate` flag for the `goat` binary so it can be self-invoked as a teammate.

**Changes**:

1. **`pkg/teams/teammate.go`** — Teammate mode runtime
   ```go
   type TeammateRuntime struct {
       teamName  string
       agentName string
       agentType string
       mailbox   *Mailbox
       tasks     *SharedTaskList
       baseDir   string
   }

   func NewTeammateRuntime(teamName, agentName, agentType, baseDir string) *TeammateRuntime
   func (tr *TeammateRuntime) Run(ctx context.Context) error
   ```
   - Reads team config to discover other members
   - Starts mailbox watcher for incoming messages
   - Runs agentic loop with team-aware system prompt
   - Messages from mailbox injected as user turns
   - On idle: fires TeammateIdle hook, sends idle notification to lead
   - On shutdown request: responds via SendMessage tool
   - Process exits when shutdown approved or context cancelled

2. **`pkg/teams/teammate_test.go`** — Tests
   - Teammate runtime starts and reads config
   - Incoming messages delivered to loop
   - Idle notification sent on turn completion
   - Shutdown request/response lifecycle

3. **Integration point**: The actual CLI flag parsing (`--teammate`, `--team-name`, `--agent-name`, `--agent-type`) will be handled wherever the `goat` binary's main function lives. This phase provides the `TeammateRuntime` that main would call.

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/teams/... -race -run TestTeammate` passes (11 tests)
- Manual:
  - [x] TeammateRuntime can be started and receives messages (TestTeammateRuntimeWatchMessages, TestTeammateRuntimeReceiveMessages)

---

### Phase 8: Integration and Feature Gate

**Goal**: Wire everything together, add feature gate, verify full lifecycle.

**Changes**:

1. **`pkg/teams/gate.go`** — Feature gate
   ```go
   func IsEnabled() bool {
       return os.Getenv("CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS") == "1"
   }
   ```

2. **Agent config integration** — Add team context to `AgentConfig`
   - `pkg/agent/config.go`: Add `TeamName`, `AgentName` fields for team-aware sessions
   - Prompt assembly uses team context when available

3. **`pkg/teams/integration_test.go`** — End-to-end integration tests
   - Full lifecycle: create team → spawn teammates → create tasks → claim → complete → cleanup
   - Message delivery between teammates (using test helper processes)
   - Hook firing on idle and task completion
   - Feature gate prevents use when env var not set

4. **`go.mod`** — Add dependencies
   - `github.com/gofrs/flock`
   - `github.com/fsnotify/fsnotify`

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/teams/... -race` all pass (92 tests)
  - [x] `go test ./... -race` all pass (578 total, no regressions)
  - [x] `go vet ./...` clean
- Manual:
  - [x] Feature gate works (TestIntegrationFeatureGate, TestTeamCreateToolFeatureGate)
  - [ ] Team lifecycle works end-to-end in sandbox environment

## Out of Scope

- **Tmux/iTerm2 display modes** — In-process display only for now
- **Session persistence/resumption** — Spec 11 (not yet implemented)
- **Plan approval flow** — The SendMessage tool types are defined but the plan mode state machine is not yet implemented
- **Real CLI integration** — The `cmd/` entrypoint for the goat binary; this plan provides the library
- **MCP server integration** — Spec 10 (not yet implemented)
- **Cross-sandbox communication** — All processes assumed to share one filesystem

## Open Questions

(None — all resolved through discussion)

## References

- Spec: `thoughts/specs/08c-AGENT-TEAMS.md`
- Subagent manager (pattern reference): `pkg/subagent/manager.go`
- Hook types: `pkg/hooks/types.go:115-132` (TeammateIdle, TaskCompleted inputs)
- Hook events: `pkg/types/options.go:148-149` (event constants)
- Prompt files: `pkg/prompt/prompts/tools/tool-description-teammatetool.md`, `tool-description-sendmessagetool.md`, `tool-description-teamdelete.md`
- Existing patterns: `pkg/tools/taskmanager.go` (background task lifecycle), `pkg/subagent/running.go` (agent state tracking)
