# Agent Teams — Multi-Agent Coordination

> `pkg/teams/` — File-based multi-agent coordination. One team per session,
> shared task list with file locking, mailbox messaging with fsnotify.
> Feature-gated behind `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1`.

## Team Architecture

```
 ┌───────────────────────────────────────────────────────────────────┐
 │                        Lead Agent                                  │
 │  (the agent that created the team)                                 │
 │                                                                   │
 │  In DELEGATE mode:                                                 │
 │    Only 7 coordination tools available:                           │
 │    TeamCreate, SendMessage, TeamDelete,                           │
 │    TodoWrite, AskUserQuestion, Config, ExitPlanMode              │
 │                                                                   │
 │  Responsibilities:                                                 │
 │    - Create team + define tasks                                   │
 │    - Spawn teammates                                              │
 │    - Monitor progress via messages                                │
 │    - Shut down team when done                                     │
 └──────────┬─────────────────────┬──────────────────────────────────┘
            │                     │
     ┌──────▼──────┐       ┌──────▼──────┐
     │ Teammate A  │       │ Teammate B  │
     │             │       │             │
     │ Full tools  │       │ Full tools  │
     │ Claims tasks│       │ Claims tasks│
     │ Sends msgs  │       │ Sends msgs  │
     └──────┬──────┘       └──────┬──────┘
            │                     │
     ┌──────▼─────────────────────▼──────┐
     │         Shared State               │
     │                                    │
     │  ┌─ Task List ────────────────┐   │
     │  │  {baseDir}/tasks/{team}/   │   │
     │  │  task-001.json             │   │
     │  │  task-002.json             │   │
     │  │  (file locking per task)   │   │
     │  └────────────────────────────┘   │
     │                                    │
     │  ┌─ Mailboxes ───────────────┐   │
     │  │  {baseDir}/teams/{team}/  │   │
     │  │  inbox/                    │   │
     │  │    lead/                   │   │
     │  │      msg-001.json          │   │
     │  │    teammate-a/             │   │
     │  │      msg-002.json          │   │
     │  │    teammate-b/             │   │
     │  │      msg-003.json          │   │
     │  │  (fsnotify watchers)       │   │
     │  └────────────────────────────┘   │
     │                                    │
     │  ┌─ Team Config ─────────────┐   │
     │  │  config.json               │   │
     │  │  {name, members, created}  │   │
     │  └────────────────────────────┘   │
     └────────────────────────────────────┘
```

## SharedTaskList — Concurrent Task Claiming

```
 ┌───────────────────────────────────────────────────────────────────┐
 │  SharedTaskList                                                    │
 │                                                                   │
 │  Each task = one JSON file:                                       │
 │    tasks/{team}/task-001.json                                    │
 │    {                                                              │
 │      "id": "task-001",                                           │
 │      "subject": "Implement user auth",                           │
 │      "description": "...",                                        │
 │      "status": "pending",          // pending|claimed|done|failed│
 │      "claimed_by": "",             // agent name when claimed    │
 │      "created_at": "2026-02-09",                                 │
 │      "completed_at": ""                                           │
 │    }                                                              │
 │                                                                   │
 │  Claiming a task (atomic with file locking):                      │
 │    1. flock.Lock(task-001.json.lock)   ← exclusive lock          │
 │    2. Read task-001.json                                          │
 │    3. Check status == "pending"                                   │
 │    4. Set status = "claimed", claimed_by = "teammate-a"          │
 │    5. Write task-001.json                                         │
 │    6. flock.Unlock()                                              │
 │                                                                   │
 │  If another agent tries to claim simultaneously:                  │
 │    Their flock.Lock() blocks until first agent finishes          │
 │    They read status="claimed" → skip to next task                │
 │                                                                   │
 │  Uses gofrs/flock for cross-process file locking                 │
 └───────────────────────────────────────────────────────────────────┘
```

## Mailbox — File-Based Messaging

```
 ┌───────────────────────────────────────────────────────────────────┐
 │  Mailbox (per agent)                                              │
 │                                                                   │
 │  Directory: teams/{team}/inbox/{agent-name}/                     │
 │  Watcher: fsnotify.Watcher on inbox directory                    │
 │                                                                   │
 │  Send message:                                                    │
 │    Write JSON file to recipient's inbox:                          │
 │    teams/{team}/inbox/teammate-a/msg-{uuid}.json                 │
 │    {                                                              │
 │      "id": "msg-uuid",                                           │
 │      "from": "lead",                                              │
 │      "to": "teammate-a",                                          │
 │      "type": "direct",             // direct|broadcast|shutdown  │
 │      "content": "Focus on auth",                                 │
 │      "timestamp": "2026-02-09T..."                                │
 │    }                                                              │
 │                                                                   │
 │  Receive message:                                                  │
 │    fsnotify detects new file → read → process → delete file      │
 │                                                                   │
 │  Message types:                                                   │
 │    direct              ─ point-to-point message                   │
 │    broadcast           ─ sent to all team members                 │
 │    shutdown_request    ─ lead asks agent to stop                  │
 │    shutdown_response   ─ agent acknowledges shutdown              │
 │    plan_approval_response ─ response to plan approval request    │
 └───────────────────────────────────────────────────────────────────┘
```

## SpawnTeammate — Process Self-Invocation

```
 ┌───────────────────────────────────────────────────────────────────┐
 │  Lead Agent calls TeamCreate:                                     │
 │    1. Create team directory + config                              │
 │    2. For each teammate:                                          │
 │       SpawnTeammate(name, config)                                 │
 │                                                                   │
 │  SpawnTeammate:                                                   │
 │    exec.Command(os.Args[0], "--teammate",                        │
 │      "--team", teamName,                                          │
 │      "--agent", agentName,                                        │
 │      "--base-dir", baseDir)                                      │
 │                                                                   │
 │    ▲ Self-invocation: the same binary restarts as a teammate     │
 │                                                                   │
 │  Teammate process starts TeammateRuntime:                         │
 │    1. Read team config                                            │
 │    2. Set up mailbox watcher                                      │
 │    3. Loop:                                                       │
 │       a. Check mailbox for messages                               │
 │       b. Claim next pending task                                  │
 │       c. Execute task (RunLoop with task prompt)                  │
 │       d. Mark task done/failed                                    │
 │       e. Fire TaskCompleted hook                                  │
 │       f. Check ShouldKeepWorking                                  │
 │          └── no more tasks? Fire TeammateIdle hook               │
 │          └── ShouldPreventCompletion? wait for messages           │
 │       g. Check for shutdown_request                               │
 │          └── yes: send shutdown_response, exit                    │
 │                                                                   │
 │  For testing: SpawnTeammateWithFunc(fn) runs inline              │
 └───────────────────────────────────────────────────────────────────┘
```

## Shutdown Flow

```
 Lead decides to shut down team:
         │
         ▼
 ┌─────────────────┐     ┌─────────────────────────────────────┐
 │ RequestShutdown  │────▶│  Send shutdown_request message      │
 │ (graceful)       │     │  to each teammate's mailbox         │
 └─────────────────┘     └──────────────────┬──────────────────┘
                                             │
                          Teammate receives shutdown_request
                                             │
                          ┌──────────────────▼──────────────────┐
                          │  Finish current task (if any)        │
                          │  Send shutdown_response message      │
                          │  Exit process                         │
                          └──────────────────────────────────────┘

 If graceful shutdown times out:
         │
         ▼
 ┌─────────────────┐
 │ ShutdownTeammate│──── SIGTERM → wait → SIGKILL
 │ (forceful)      │
 └─────────────────┘
```

## DelegateModeState — Lead Agent Restriction

```
 When team is active, lead enters DELEGATE mode:

 ┌─────────────────────────────────────────────────────────────────┐
 │  Available tools (7 only):                                      │
 │                                                                  │
 │  TeamCreate        ─ spawn team members                         │
 │  SendMessage       ─ communicate with teammates                 │
 │  TeamDelete        ─ shut down team                             │
 │  TodoWrite         ─ manage team-level task list                │
 │  AskUserQuestion   ─ ask user for clarification                 │
 │  Config            ─ read/write configuration                   │
 │  ExitPlanMode      ─ transition out of plan mode                │
 │                                                                  │
 │  ALL other tools are DENIED by delegate permission mode.        │
 │  The lead agent can only coordinate, not execute directly.      │
 │                                                                  │
 │  This forces proper delegation: the lead tells teammates        │
 │  what to do via tasks, teammates do the actual work.            │
 └─────────────────────────────────────────────────────────────────┘
```

## Feature Gate

```
 if os.Getenv("CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS") == "1" {
     registry.Register(&tools.TeamCreateTool{})
     registry.Register(&tools.SendMessageTool{})
     registry.Register(&tools.TeamDeleteTool{})
 }

 Without the env var: team tools are not registered.
 The LLM never sees them. Teams are completely invisible.
```
