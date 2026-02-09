# Session Persistence — JSONL Storage & Checkpointing

> `pkg/session/` — File-based session persistence with async background writer,
> cross-process locking, and content-addressed file checkpoints.

## Storage Layout

```
 {baseDir}/
 └── {session-id}/                    ← one dir per session
     ├── metadata.json                 ← session metadata (JSON)
     ├── messages.jsonl                ← conversation history (JSONL)
     ├── transcript.jsonl              ← SDK messages for replay (JSONL)
     └── checkpoints/                  ← file snapshots
         └── {checkpoint-id}/
             ├── manifest.json         ← file list + SHA256 hashes
             └── blobs/
                 └── {sha256-hash}     ← content-addressed file data

 metadata.json example:
 {
   "session_id": "abc-123",
   "created_at": "2026-02-09T...",
   "model": "claude-sonnet-4-5-20250929",
   "cwd": "/Users/dev/project",
   "turn_count": 5,
   "total_cost_usd": 0.0234,
   "exit_reason": "end_turn",
   "agent_name": ""
 }

 messages.jsonl (one JSON object per line):
 {"uuid":"...","timestamp":"...","role":"user","content":"Hello"}
 {"uuid":"...","timestamp":"...","role":"assistant","content":"Hi!","tool_calls":[...]}
 {"uuid":"...","timestamp":"...","role":"tool","tool_call_id":"call_1","content":"..."}
```

## Async Background Writer

```
 ┌───────────────────────────────────────────────────────────────────┐
 │  Store                                                            │
 │                                                                   │
 │  opCh   chan writeOp (buffer: 256)   ← buffered operation queue  │
 │  writer goroutine                     ← single background writer │
 │  flock  *flock.Flock                  ← cross-process file lock  │
 │                                                                   │
 │  ┌─── Write Path (non-blocking) ──────────────────────────────┐ │
 │  │                                                             │ │
 │  │  AppendMessage(sessionID, entry):                           │ │
 │  │    opCh <- writeOp{type: appendMsg, data: entry}           │ │
 │  │    └── returns immediately (non-blocking)                   │ │
 │  │                                                             │ │
 │  │  AppendSDKMessage(sessionID, msg):                          │ │
 │  │    opCh <- writeOp{type: appendSDK, data: msg}             │ │
 │  │                                                             │ │
 │  │  UpdateMetadata(sessionID, fn):                             │ │
 │  │    opCh <- writeOp{type: updateMeta, fn: fn}               │ │
 │  │                                                             │ │
 │  └─────────────────────────────────────────────────────────────┘ │
 │                                                                   │
 │  ┌─── Background Writer Goroutine ────────────────────────────┐ │
 │  │                                                             │ │
 │  │  for op := range opCh:                                      │ │
 │  │    flock.Lock()           ← cross-process exclusive lock   │ │
 │  │    switch op.type:                                          │ │
 │  │      appendMsg:  appendJSONL(messages.jsonl, entry)        │ │
 │  │      appendSDK:  appendJSONL(transcript.jsonl, msg)        │ │
 │  │      updateMeta: readJSON → fn(meta) → writeJSON           │ │
 │  │    flock.Unlock()                                           │ │
 │  │                                                             │ │
 │  │  On channel close:                                          │ │
 │  │    flush remaining ops, exit goroutine                      │ │
 │  │                                                             │ │
 │  └─────────────────────────────────────────────────────────────┘ │
 └───────────────────────────────────────────────────────────────────┘

 Key: All file I/O happens in the background writer goroutine.
 The agent loop never blocks on disk writes. The 256-item buffer
 absorbs bursts of rapid tool executions.
```

## Checkpoint System — Content-Addressed File Snapshots

```
 ┌───────────────────────────────────────────────────────────────────┐
 │  CreateCheckpoint(sessionID, checkpointID, filePaths)             │
 │                                                                   │
 │  For each file path:                                              │
 │    1. Read file contents                                          │
 │    2. Compute SHA256 hash                                         │
 │    3. Write to blobs/{hash} (if not already exists)              │
 │    4. Add to manifest: {path: "src/main.go", hash: "abc123..."}  │
 │                                                                   │
 │  manifest.json:                                                   │
 │  {                                                                │
 │    "checkpoint_id": "cp-001",                                    │
 │    "created_at": "2026-02-09T...",                               │
 │    "files": [                                                     │
 │      {"path": "src/main.go", "hash": "abc123..."},              │
 │      {"path": "src/util.go", "hash": "def456..."},              │
 │    ]                                                              │
 │  }                                                                │
 │                                                                   │
 │  Content-addressed storage:                                       │
 │    Same file content across checkpoints → shared blob            │
 │    Only changed files get new blobs                               │
 │    Efficient for incremental changes                              │
 └───────────────────────────────────────────────────────────────────┘

 RewindFiles(sessionID, checkpointID, dryRun):
   Read manifest → restore files from blobs → return diff list
   dryRun=true: report what would change without writing
```

## Session Store Interface

```go
type SessionStore interface {
    Create(meta SessionMetadata) error
    Load(sessionID string) (*SessionState, error)
    LoadLatest(projectPath string) (*SessionState, error)
    Delete(sessionID string) error
    List() ([]SessionMetadata, error)
    Fork(sourceID, newID string) (*SessionState, error)

    AppendMessage(sessionID string, entry MessageEntry) error
    AppendSDKMessage(sessionID string, msg SDKMessage) error
    LoadMessages(sessionID string) ([]MessageEntry, error)
    LoadMessagesUpTo(sessionID, messageID string) ([]MessageEntry, error)
    UpdateMetadata(sessionID string, fn func(*SessionMetadata)) error

    CreateCheckpoint(sessionID, checkpointID string, files []string) error
    RewindFiles(sessionID, checkpointID string, dryRun bool) (*RewindFilesResult, error)

    Close() error
}
```

## Loop Integration Points

```
 ┌───────────────────────────────────────────────────────────────────┐
 │  RunLoop():                                                       │
 │                                                                   │
 │  INITIALIZE:                                                      │
 │    initializeSession(config) → store.Create(metadata)            │
 │                                                                   │
 │  USER INPUT:                                                      │
 │    persistMessage(store, sessionID, userMsg)                     │
 │                                                                   │
 │  AFTER LLM RESPONSE:                                              │
 │    persistMessage(store, sessionID, assistantMsg)                │
 │                                                                   │
 │  AFTER TOOL EXECUTION:                                            │
 │    for each toolMsg:                                              │
 │      persistMessage(store, sessionID, toolMsg)                   │
 │                                                                   │
 │  MULTI-TURN NEW INPUT:                                            │
 │    persistMessage(store, sessionID, followUpMsg)                 │
 │                                                                   │
 │  FINALIZE:                                                        │
 │    finalizeSession(config, state) →                              │
 │      store.UpdateMetadata(sessionID, func(meta) {                │
 │        meta.TurnCount = state.TurnCount                          │
 │        meta.TotalCostUSD = state.TotalCostUSD                   │
 │        meta.ExitReason = string(state.ExitReason)                │
 │      })                                                           │
 │                                                                   │
 │  Note: All persist calls fire-and-forget (errors discarded).     │
 │  Session persistence is best-effort. The loop never crashes      │
 │  due to a storage failure.                                       │
 └───────────────────────────────────────────────────────────────────┘
```

## Session Restoration

```
 RestoreSession(config, state, opts):
         │
         ├── opts.Resume (resume existing session):
         │     state.Messages = store.LoadMessages(sessionID)
         │     state.TurnCount, Cost = metadata values
         │
         ├── opts.Continue (continue from last session):
         │     latest = store.LoadLatest(projectPath)
         │     Fork into new session with copied messages
         │
         └── opts.Fork (fork from specific message):
               state.Messages = store.LoadMessagesUpTo(sessionID, msgID)
               Create new session with subset of messages

 All paths result in populated state.Messages for the loop to use.
```

## NoOpSessionStore — Disabled Persistence

```
 When config.SessionStore is nil or persistence is disabled:
   All methods return nil/empty values
   No file I/O occurs
   Loop works identically, just without persistence

 Used in:
   - Testing (default in test configs)
   - Subagent execution (background agents don't persist)
   - Embedded usage where persistence isn't wanted
```
