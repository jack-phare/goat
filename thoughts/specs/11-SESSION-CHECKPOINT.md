# Spec 11: Session & Checkpoint Store

**Go Package**: `pkg/session/`
**Source References**:
- `sdk.d.ts:1444-1459` — SDKSession interface (sessionId, send, stream, close, asyncDispose)
- `sdk.d.ts:1086-1094` — RewindFilesResult (canRewind, error, filesChanged, insertions, deletions)
- `sdk.d.ts:1042-1054` — Query.rewindFiles(userMessageId, options)
- `sdk.d.ts:563-568` — Options: enableFileCheckpointing, forkSession
- `sdk.d.ts:701-720` — Options: resume, sessionId, resumeSessionAt
- `sdk.d.ts:600` — Options: persistSession

---

## 1. Purpose

The session store manages conversation persistence, enabling:
1. Session save/load for conversation continuity (`--continue`, `--resume`)
2. File checkpointing to track and rewind file changes
3. Session forking for branching conversations
4. Transcript storage for subagent audit trails

---

## 2. Session Storage Layout

```
~/.claude/projects/{project-hash}/
├── sessions/
│   ├── {session-id-1}/
│   │   ├── metadata.json       # session metadata
│   │   ├── messages.jsonl      # conversation messages (one per line)
│   │   ├── transcript.jsonl    # full SDKMessage stream (for replay)
│   │   └── checkpoints/        # file checkpoints
│   │       ├── {user-msg-uuid-1}/
│   │       │   ├── manifest.json
│   │       │   └── files/
│   │       │       ├── {hash1}  # file content snapshots
│   │       │       └── {hash2}
│   │       └── {user-msg-uuid-2}/
│   │           └── ...
│   └── {session-id-2}/
│       └── ...
└── memory/
    └── summary.md              # session memory (for Piebald-AI memory system)
```

---

## 3. Go Types

### 3.1 Session Metadata

```go
type SessionMetadata struct {
    ID           string    `json:"id"`            // UUID
    ProjectHash  string    `json:"project_hash"`
    CWD          string    `json:"cwd"`
    Model        string    `json:"model"`
    CreatedAt    time.Time `json:"created_at"`
    UpdatedAt    time.Time `json:"updated_at"`
    MessageCount int       `json:"message_count"`
    TurnCount    int       `json:"turn_count"`
    TotalTokens  int       `json:"total_tokens"`
    ExitReason   string    `json:"exit_reason,omitempty"`
    ParentID     string    `json:"parent_id,omitempty"` // for forked sessions
    AgentName    string    `json:"agent_name,omitempty"`
}
```

### 3.2 Store Interface

```go
// Store manages session persistence.
type Store struct {
    baseDir string // ~/.claude/projects/{hash}/sessions
}

func NewStore(projectDir string) *Store

// Session lifecycle
func (s *Store) Create(meta SessionMetadata) error
func (s *Store) Load(sessionID string) (*Session, error)
func (s *Store) LoadLatest(cwd string) (*Session, error)  // for --continue
func (s *Store) Fork(sourceID string, newID string) (*Session, error)
func (s *Store) Delete(sessionID string) error
func (s *Store) List() ([]SessionMetadata, error)

// Message persistence
func (s *Store) AppendMessage(sessionID string, msg Message) error
func (s *Store) AppendSDKMessage(sessionID string, msg SDKMessage) error
func (s *Store) LoadMessages(sessionID string) ([]Message, error)
func (s *Store) LoadMessagesUpTo(sessionID string, messageUUID string) ([]Message, error)
```

### 3.3 Session (loaded state)

```go
type Session struct {
    Metadata   SessionMetadata
    Messages   []Message
    store      *Store
}

// Save persists current state.
func (s *Session) Save() error

// AppendAndSave atomically appends a message and persists.
func (s *Session) AppendAndSave(msg Message) error
```

---

## 4. Resume Modes

### 4.1 Continue (`--continue` / `Options.continue`)

Loads the most recent session in the current working directory:

```go
func (s *Store) LoadLatest(cwd string) (*Session, error) {
    sessions, err := s.List()
    if err != nil { return nil, err }

    // Find most recent session for this CWD
    var latest *SessionMetadata
    for _, meta := range sessions {
        if meta.CWD == cwd {
            if latest == nil || meta.UpdatedAt.After(latest.UpdatedAt) {
                latest = &meta
            }
        }
    }
    if latest == nil {
        return nil, fmt.Errorf("no previous session found in %s", cwd)
    }
    return s.Load(latest.ID)
}
```

### 4.2 Resume (`--resume` / `Options.resume`)

Loads a specific session by ID:

```go
func (s *Store) Resume(sessionID string, resumeAt string) (*Session, error) {
    if resumeAt != "" {
        // Load messages up to a specific point (Options.resumeSessionAt)
        messages, err := s.LoadMessagesUpTo(sessionID, resumeAt)
        if err != nil { return nil, err }
        return &Session{Messages: messages, ...}, nil
    }
    return s.Load(sessionID)
}
```

### 4.3 Fork (`Options.forkSession`)

Creates a new session branching from an existing one:

```go
func (s *Store) Fork(sourceID string, newID string) (*Session, error) {
    source, err := s.Load(sourceID)
    if err != nil { return nil, err }

    // Copy messages to new session
    newMeta := source.Metadata
    newMeta.ID = newID
    newMeta.ParentID = sourceID
    newMeta.CreatedAt = time.Now()

    s.Create(newMeta)
    for _, msg := range source.Messages {
        s.AppendMessage(newID, msg)
    }

    return s.Load(newID)
}
```

---

## 5. File Checkpointing

### 5.1 Checkpoint Creation

Before each tool that modifies files, snapshot the affected files:

```go
type CheckpointManager struct {
    enabled    bool
    sessionDir string
}

func (cm *CheckpointManager) CreateCheckpoint(userMsgUUID string, filePaths []string) error {
    cpDir := filepath.Join(cm.sessionDir, "checkpoints", userMsgUUID)
    os.MkdirAll(filepath.Join(cpDir, "files"), 0755)

    manifest := CheckpointManifest{
        UserMessageUUID: userMsgUUID,
        CreatedAt:       time.Now(),
        Files:           make([]FileSnapshot, 0),
    }

    for _, path := range filePaths {
        content, err := os.ReadFile(path)
        if err != nil {
            // File doesn't exist yet — record as "new file"
            manifest.Files = append(manifest.Files, FileSnapshot{
                Path:   path,
                Exists: false,
            })
            continue
        }

        hash := sha256.Sum256(content)
        hashStr := hex.EncodeToString(hash[:])
        os.WriteFile(filepath.Join(cpDir, "files", hashStr), content, 0644)

        manifest.Files = append(manifest.Files, FileSnapshot{
            Path:     path,
            Exists:   true,
            Hash:     hashStr,
            Size:     len(content),
        })
    }

    return writeJSON(filepath.Join(cpDir, "manifest.json"), manifest)
}
```

### 5.2 File Rewind (from `sdk.d.ts:1042-1054`)

```go
// RewindFilesResult matches sdk.d.ts:1086-1094.
type RewindFilesResult struct {
    CanRewind    bool     `json:"canRewind"`
    Error        string   `json:"error,omitempty"`
    FilesChanged []string `json:"filesChanged,omitempty"`
    Insertions   int      `json:"insertions,omitempty"`
    Deletions    int      `json:"deletions,omitempty"`
}

func (cm *CheckpointManager) RewindFiles(userMsgUUID string, dryRun bool) (*RewindFilesResult, error) {
    cpDir := filepath.Join(cm.sessionDir, "checkpoints", userMsgUUID)
    manifest, err := loadManifest(cpDir)
    if err != nil {
        return &RewindFilesResult{CanRewind: false, Error: "checkpoint not found"}, nil
    }

    result := &RewindFilesResult{CanRewind: true}

    for _, snap := range manifest.Files {
        if dryRun {
            // Just calculate diff stats
            result.FilesChanged = append(result.FilesChanged, snap.Path)
            continue
        }

        if !snap.Exists {
            // File was created after checkpoint — remove it
            os.Remove(snap.Path)
            result.FilesChanged = append(result.FilesChanged, snap.Path)
            result.Deletions++
        } else {
            // Restore file to checkpoint state
            content, err := os.ReadFile(filepath.Join(cpDir, "files", snap.Hash))
            if err != nil {
                result.Error = fmt.Sprintf("checkpoint data missing for %s", snap.Path)
                continue
            }
            os.WriteFile(snap.Path, content, 0644)
            result.FilesChanged = append(result.FilesChanged, snap.Path)
            result.Insertions++
        }
    }

    return result, nil
}
```

---

## 6. Message Serialization Format

Messages are stored as JSONL (one JSON object per line) for efficient append-only writes:

```go
func (s *Store) AppendMessage(sessionID string, msg Message) error {
    path := filepath.Join(s.baseDir, sessionID, "messages.jsonl")
    f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil { return err }
    defer f.Close()

    data, err := json.Marshal(msg)
    if err != nil { return err }

    _, err = fmt.Fprintf(f, "%s\n", data)
    return err
}

func (s *Store) LoadMessages(sessionID string) ([]Message, error) {
    path := filepath.Join(s.baseDir, sessionID, "messages.jsonl")
    f, err := os.Open(path)
    if err != nil { return nil, err }
    defer f.Close()

    var messages []Message
    scanner := bufio.NewScanner(f)
    scanner.Buffer(make([]byte, 0), 10*1024*1024) // 10MB max line
    for scanner.Scan() {
        var msg Message
        if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
            continue // skip corrupt lines
        }
        messages = append(messages, msg)
    }
    return messages, nil
}
```

---

## 7. Transcript Storage

The full SDKMessage stream is stored separately for debugging and subagent audit trails:

```go
func (s *Store) AppendSDKMessage(sessionID string, msg SDKMessage) error {
    path := filepath.Join(s.baseDir, sessionID, "transcript.jsonl")
    // Same JSONL append pattern as messages
    return appendJSONL(path, msg)
}
```

This is referenced by hook inputs as `transcript_path` in BaseHookInput.

---

## 8. Concurrency Safety

Session writes must be safe for concurrent access (background subagents writing simultaneously):

```go
type SessionWriter struct {
    mu   sync.Mutex
    file *os.File
}

func (w *SessionWriter) Append(data []byte) error {
    w.mu.Lock()
    defer w.mu.Unlock()
    _, err := w.file.Write(append(data, '\n'))
    return err
}
```

---

## 9. Verification Checklist

- [ ] **Session CRUD**: Create, load, list, delete all work correctly
- [ ] **Continue mode**: Loads most recent session for CWD
- [ ] **Resume mode**: Loads specific session by ID
- [ ] **Resume at**: Loads messages up to specific UUID
- [ ] **Fork**: Creates independent copy with parent reference
- [ ] **JSONL format**: Messages append-only, one per line, survives partial writes
- [ ] **Checkpoint creation**: File snapshots taken before mutating tool execution
- [ ] **Rewind correctness**: Files restored to exact checkpoint state
- [ ] **Rewind dry-run**: Returns diff stats without modifying files
- [ ] **New file handling**: Files created after checkpoint are deleted on rewind
- [ ] **Concurrent safety**: Multiple writers (subagents) don't corrupt session data
- [ ] **Persist flag**: `persistSession: false` skips all disk writes
- [ ] **Transcript storage**: Full SDKMessage stream stored for audit
- [ ] **Large sessions**: Handles sessions with 1000+ messages efficiently
