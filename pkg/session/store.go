package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/types"
)

// Store implements agent.SessionStore with file-based JSONL persistence.
type Store struct {
	baseDir        string
	writer         *asyncWriter
	persistEnabled bool // false = all writes are no-ops
}

// StoreOption configures a Store.
type StoreOption func(*Store)

// WithPersistEnabled controls whether the store actually writes to disk.
func WithPersistEnabled(enabled bool) StoreOption {
	return func(s *Store) { s.persistEnabled = enabled }
}

// NewStore creates a new session store rooted at baseDir.
// baseDir is typically ~/.claude/projects/{sanitized-cwd}/sessions/
func NewStore(baseDir string, opts ...StoreOption) *Store {
	s := &Store{
		baseDir:        baseDir,
		writer:         newAsyncWriter(),
		persistEnabled: true, // default: writes are enabled
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Store) sessionDir(sessionID string) string {
	return filepath.Join(s.baseDir, sessionID)
}

func (s *Store) messagesPath(sessionID string) string {
	return filepath.Join(s.sessionDir(sessionID), messagesFile)
}

func (s *Store) transcriptPath(sessionID string) string {
	return filepath.Join(s.sessionDir(sessionID), transcriptFile)
}

// Create persists a new session with its metadata.
func (s *Store) Create(meta agent.SessionMetadata) error {
	if !s.persistEnabled {
		return nil
	}
	dir := s.sessionDir(meta.ID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}
	return saveMetadata(dir, meta)
}

// Load retrieves a session by ID with all its messages.
func (s *Store) Load(sessionID string) (*agent.SessionState, error) {
	dir := s.sessionDir(sessionID)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, ErrSessionNotFound
	}

	meta, err := loadMetadata(dir)
	if err != nil {
		return nil, fmt.Errorf("load metadata: %w", err)
	}

	entries, err := loadMessageEntries(s.messagesPath(sessionID))
	if err != nil {
		return nil, fmt.Errorf("load messages: %w", err)
	}

	return &agent.SessionState{
		Metadata: meta,
		Messages: entries,
	}, nil
}

// LoadLatest finds the most recently updated session for the given CWD.
func (s *Store) LoadLatest(cwd string) (*agent.SessionState, error) {
	sessions, err := s.List()
	if err != nil {
		return nil, err
	}

	var latest *agent.SessionMetadata
	for i := range sessions {
		if sessions[i].CWD != cwd {
			continue
		}
		if latest == nil || sessions[i].UpdatedAt.After(latest.UpdatedAt) {
			latest = &sessions[i]
		}
	}

	if latest == nil {
		return nil, ErrSessionNotFound
	}
	return s.Load(latest.ID)
}

// Delete removes a session and all its files.
func (s *Store) Delete(sessionID string) error {
	dir := s.sessionDir(sessionID)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return ErrSessionNotFound
	}
	return os.RemoveAll(dir)
}

// List returns metadata for all sessions.
func (s *Store) List() ([]agent.SessionMetadata, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var sessions []agent.SessionMetadata
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		meta, err := loadMetadata(filepath.Join(s.baseDir, entry.Name()))
		if err != nil {
			continue // skip corrupt sessions
		}
		sessions = append(sessions, meta)
	}

	// Sort by UpdatedAt descending (most recent first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	return sessions, nil
}

// Fork creates a new session as a copy of an existing one.
func (s *Store) Fork(sourceID, newID string) (*agent.SessionState, error) {
	source, err := s.Load(sourceID)
	if err != nil {
		return nil, fmt.Errorf("load source session: %w", err)
	}

	now := time.Now()
	newMeta := source.Metadata
	newMeta.ID = newID
	newMeta.ParentSessionID = sourceID
	newMeta.CreatedAt = now
	newMeta.UpdatedAt = now

	if err := s.Create(newMeta); err != nil {
		return nil, fmt.Errorf("create forked session: %w", err)
	}

	// Copy messages
	for _, entry := range source.Messages {
		if err := appendJSONL(s.messagesPath(newID), entry); err != nil {
			return nil, fmt.Errorf("copy message to fork: %w", err)
		}
	}

	return &agent.SessionState{
		Metadata: newMeta,
		Messages: source.Messages,
	}, nil
}

// AppendMessage writes a MessageEntry to the session's JSONL log via the async writer.
func (s *Store) AppendMessage(sessionID string, entry agent.MessageEntry) error {
	if !s.persistEnabled {
		return nil
	}
	dir := s.sessionDir(sessionID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	errCh := make(chan error, 1)
	s.writer.Write(s.messagesPath(sessionID), data, errCh)
	return <-errCh
}

// AppendSDKMessage writes an SDKMessage to the session's transcript log.
func (s *Store) AppendSDKMessage(sessionID string, msg types.SDKMessage) error {
	if !s.persistEnabled {
		return nil
	}
	dir := s.sessionDir(sessionID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	// Transcript writes are fire-and-forget
	s.writer.Write(s.transcriptPath(sessionID), data, nil)
	return nil
}

// LoadMessages reads all MessageEntry records for a session.
func (s *Store) LoadMessages(sessionID string) ([]agent.MessageEntry, error) {
	return loadMessageEntries(s.messagesPath(sessionID))
}

// LoadMessagesUpTo reads messages up to and including the specified UUID.
func (s *Store) LoadMessagesUpTo(sessionID string, messageUUID string) ([]agent.MessageEntry, error) {
	return loadMessageEntriesUpTo(s.messagesPath(sessionID), messageUUID)
}

// UpdateMetadata atomically updates the session's metadata using fn.
func (s *Store) UpdateMetadata(sessionID string, fn func(*agent.SessionMetadata)) error {
	if !s.persistEnabled {
		return nil
	}
	dir := s.sessionDir(sessionID)
	meta, err := loadMetadata(dir)
	if err != nil {
		return fmt.Errorf("load metadata for update: %w", err)
	}

	fn(&meta)
	meta.UpdatedAt = time.Now()
	return saveMetadata(dir, meta)
}

// CreateCheckpoint snapshots the specified files for the given session/message.
func (s *Store) CreateCheckpoint(sessionID, userMsgUUID string, filePaths []string) error {
	if !s.persistEnabled {
		return nil
	}
	cm := newCheckpointManager(s.sessionDir(sessionID))
	return cm.CreateCheckpoint(userMsgUUID, filePaths)
}

// RewindFiles restores files to a previous checkpoint state.
func (s *Store) RewindFiles(sessionID, userMsgUUID string, dryRun bool) (*agent.RewindFilesResult, error) {
	cm := newCheckpointManager(s.sessionDir(sessionID))
	return cm.RewindFiles(userMsgUUID, dryRun)
}

// Close flushes the async writer and releases resources.
func (s *Store) Close() error {
	return s.writer.Close()
}
