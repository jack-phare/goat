package session

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/llm"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	return NewStore(dir)
}

func testMetadata(id, cwd string) agent.SessionMetadata {
	now := time.Now()
	return agent.SessionMetadata{
		ID:        id,
		CWD:       cwd,
		Model:     "claude-sonnet-4-5-20250929",
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func testMessageEntry(uuid, role, content string) agent.MessageEntry {
	return agent.MessageEntry{
		UUID:      uuid,
		Timestamp: time.Now(),
		Message: llm.ChatMessage{
			Role:    role,
			Content: content,
		},
	}
}

// --- CRUD Tests ---

func TestStore_CreateAndLoad(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	meta := testMetadata("sess-1", "/tmp/project")
	if err := s.Create(meta); err != nil {
		t.Fatalf("Create: %v", err)
	}

	state, err := s.Load("sess-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state.Metadata.ID != "sess-1" {
		t.Errorf("ID = %q, want sess-1", state.Metadata.ID)
	}
	if state.Metadata.CWD != "/tmp/project" {
		t.Errorf("CWD = %q, want /tmp/project", state.Metadata.CWD)
	}
	if len(state.Messages) != 0 {
		t.Errorf("Messages = %d, want 0 (new session)", len(state.Messages))
	}
}

func TestStore_Load_NotFound(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	_, err := s.Load("nonexistent")
	if err != ErrSessionNotFound {
		t.Errorf("err = %v, want ErrSessionNotFound", err)
	}
}

func TestStore_Delete(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	meta := testMetadata("sess-del", "/tmp")
	s.Create(meta)

	if err := s.Delete("sess-del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := s.Load("sess-del")
	if err != ErrSessionNotFound {
		t.Errorf("after delete, Load err = %v, want ErrSessionNotFound", err)
	}
}

func TestStore_Delete_NotFound(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	err := s.Delete("nonexistent")
	if err != ErrSessionNotFound {
		t.Errorf("err = %v, want ErrSessionNotFound", err)
	}
}

func TestStore_List(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	// Create 3 sessions with staggered timestamps
	for i, id := range []string{"sess-a", "sess-b", "sess-c"} {
		meta := testMetadata(id, "/tmp/project")
		meta.UpdatedAt = time.Now().Add(time.Duration(i) * time.Second)
		s.Create(meta)
	}

	sessions, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("List returned %d sessions, want 3", len(sessions))
	}

	// Should be sorted by UpdatedAt descending
	if sessions[0].ID != "sess-c" {
		t.Errorf("first session = %q, want sess-c (most recent)", sessions[0].ID)
	}
}

func TestStore_List_Empty(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	sessions, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("List returned %d sessions, want 0", len(sessions))
	}
}

// --- Message Tests ---

func TestStore_AppendAndLoadMessages(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	meta := testMetadata("sess-msg", "/tmp")
	s.Create(meta)

	entries := []agent.MessageEntry{
		testMessageEntry("uuid-1", "user", "Hello"),
		testMessageEntry("uuid-2", "assistant", "Hi there!"),
		testMessageEntry("uuid-3", "user", "How are you?"),
	}

	for _, e := range entries {
		if err := s.AppendMessage("sess-msg", e); err != nil {
			t.Fatalf("AppendMessage: %v", err)
		}
	}

	loaded, err := s.LoadMessages("sess-msg")
	if err != nil {
		t.Fatalf("LoadMessages: %v", err)
	}
	if len(loaded) != 3 {
		t.Fatalf("LoadMessages returned %d, want 3", len(loaded))
	}
	if loaded[0].UUID != "uuid-1" {
		t.Errorf("first message UUID = %q, want uuid-1", loaded[0].UUID)
	}
	if loaded[2].Message.Content != "How are you?" {
		t.Errorf("third message content = %v, want 'How are you?'", loaded[2].Message.Content)
	}
}

func TestStore_LoadMessages_EmptySession(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	meta := testMetadata("sess-empty", "/tmp")
	s.Create(meta)

	msgs, err := s.LoadMessages("sess-empty")
	if err != nil {
		t.Fatalf("LoadMessages: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("LoadMessages for empty session = %d, want 0", len(msgs))
	}
}

func TestStore_LoadMessagesUpTo(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	meta := testMetadata("sess-upto", "/tmp")
	s.Create(meta)

	for _, uuid := range []string{"uuid-1", "uuid-2", "uuid-3", "uuid-4", "uuid-5"} {
		s.AppendMessage("sess-upto", testMessageEntry(uuid, "user", "msg-"+uuid))
	}

	loaded, err := s.LoadMessagesUpTo("sess-upto", "uuid-3")
	if err != nil {
		t.Fatalf("LoadMessagesUpTo: %v", err)
	}
	if len(loaded) != 3 {
		t.Fatalf("LoadMessagesUpTo returned %d, want 3", len(loaded))
	}
	if loaded[2].UUID != "uuid-3" {
		t.Errorf("last message UUID = %q, want uuid-3", loaded[2].UUID)
	}
}

// --- LoadLatest Tests ---

func TestStore_LoadLatest(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	// Two sessions for the same CWD, one newer
	old := testMetadata("sess-old", "/tmp/project")
	old.UpdatedAt = time.Now().Add(-time.Hour)
	s.Create(old)

	recent := testMetadata("sess-new", "/tmp/project")
	recent.UpdatedAt = time.Now()
	s.Create(recent)

	// One for a different CWD
	other := testMetadata("sess-other", "/tmp/other")
	other.UpdatedAt = time.Now().Add(time.Hour) // even newer, but wrong CWD
	s.Create(other)

	state, err := s.LoadLatest("/tmp/project")
	if err != nil {
		t.Fatalf("LoadLatest: %v", err)
	}
	if state.Metadata.ID != "sess-new" {
		t.Errorf("LoadLatest returned session %q, want sess-new", state.Metadata.ID)
	}
}

func TestStore_LoadLatest_NotFound(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	_, err := s.LoadLatest("/nonexistent")
	if err != ErrSessionNotFound {
		t.Errorf("err = %v, want ErrSessionNotFound", err)
	}
}

// --- Fork Tests ---

func TestStore_Fork(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	// Create source session with messages
	meta := testMetadata("sess-src", "/tmp/project")
	s.Create(meta)
	s.AppendMessage("sess-src", testMessageEntry("uuid-1", "user", "Hello"))
	s.AppendMessage("sess-src", testMessageEntry("uuid-2", "assistant", "Hi"))

	// Fork
	forked, err := s.Fork("sess-src", "sess-fork")
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if forked.Metadata.ID != "sess-fork" {
		t.Errorf("forked ID = %q, want sess-fork", forked.Metadata.ID)
	}
	if forked.Metadata.ParentSessionID != "sess-src" {
		t.Errorf("ParentSessionID = %q, want sess-src", forked.Metadata.ParentSessionID)
	}
	if len(forked.Messages) != 2 {
		t.Fatalf("forked messages = %d, want 2", len(forked.Messages))
	}

	// Verify fork is independent: add message to fork, source unchanged
	s.AppendMessage("sess-fork", testMessageEntry("uuid-3", "user", "New in fork"))

	srcMsgs, _ := s.LoadMessages("sess-src")
	forkMsgs, _ := s.LoadMessages("sess-fork")

	if len(srcMsgs) != 2 {
		t.Errorf("source messages after fork append = %d, want 2", len(srcMsgs))
	}
	if len(forkMsgs) != 3 {
		t.Errorf("fork messages after append = %d, want 3", len(forkMsgs))
	}
}

func TestStore_Fork_NotFound(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	_, err := s.Fork("nonexistent", "new-id")
	if err == nil {
		t.Error("Fork of nonexistent session should return error")
	}
}

// --- UpdateMetadata Tests ---

func TestStore_UpdateMetadata(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	meta := testMetadata("sess-upd", "/tmp")
	s.Create(meta)

	err := s.UpdateMetadata("sess-upd", func(m *agent.SessionMetadata) {
		m.MessageCount = 42
		m.TurnCount = 10
		m.TotalCostUSD = 0.05
		m.LeafTitle = "Test Session"
	})
	if err != nil {
		t.Fatalf("UpdateMetadata: %v", err)
	}

	state, _ := s.Load("sess-upd")
	if state.Metadata.MessageCount != 42 {
		t.Errorf("MessageCount = %d, want 42", state.Metadata.MessageCount)
	}
	if state.Metadata.TurnCount != 10 {
		t.Errorf("TurnCount = %d, want 10", state.Metadata.TurnCount)
	}
	if state.Metadata.TotalCostUSD != 0.05 {
		t.Errorf("TotalCostUSD = %f, want 0.05", state.Metadata.TotalCostUSD)
	}
	if state.Metadata.LeafTitle != "Test Session" {
		t.Errorf("LeafTitle = %q, want 'Test Session'", state.Metadata.LeafTitle)
	}
}

// --- Async Writer Tests ---

func TestStore_ConcurrentAppend(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	meta := testMetadata("sess-conc", "/tmp")
	s.Create(meta)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			entry := testMessageEntry(
				fmt.Sprintf("uuid-%d", idx),
				"user",
				fmt.Sprintf("Message %d", idx),
			)
			if err := s.AppendMessage("sess-conc", entry); err != nil {
				t.Errorf("AppendMessage(%d): %v", idx, err)
			}
		}(i)
	}
	wg.Wait()

	msgs, err := s.LoadMessages("sess-conc")
	if err != nil {
		t.Fatalf("LoadMessages: %v", err)
	}
	if len(msgs) != 10 {
		t.Errorf("LoadMessages = %d, want 10", len(msgs))
	}
}

// --- JSONL Roundtrip Tests ---

func TestJSONL_AppendAndLoad_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	entries := []agent.MessageEntry{
		testMessageEntry("uuid-1", "user", "Hello world"),
		testMessageEntry("uuid-2", "assistant", "Hi! How can I help?"),
	}

	for _, e := range entries {
		if err := appendJSONL(path, e); err != nil {
			t.Fatalf("appendJSONL: %v", err)
		}
	}

	loaded, err := loadMessageEntries(path)
	if err != nil {
		t.Fatalf("loadMessageEntries: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("loaded %d entries, want 2", len(loaded))
	}
	if loaded[0].UUID != "uuid-1" {
		t.Errorf("first UUID = %q, want uuid-1", loaded[0].UUID)
	}
	if loaded[1].Message.Role != "assistant" {
		t.Errorf("second role = %q, want assistant", loaded[1].Message.Role)
	}
}

func TestJSONL_LoadNonexistent(t *testing.T) {
	entries, err := loadMessageEntries("/nonexistent/path.jsonl")
	if err != nil {
		t.Fatalf("loadMessageEntries should return nil for nonexistent: %v", err)
	}
	if entries != nil {
		t.Errorf("entries = %v, want nil", entries)
	}
}

func TestJSONL_CorruptLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.jsonl")

	// Write valid + corrupt + valid lines
	content := `{"uuid":"uuid-1","timestamp":"2024-01-01T00:00:00Z","message":{"role":"user","content":"hello"}}
this is not json
{"uuid":"uuid-2","timestamp":"2024-01-01T00:00:01Z","message":{"role":"assistant","content":"hi"}}
`
	os.WriteFile(path, []byte(content), 0644)

	entries, err := loadMessageEntries(path)
	if err != nil {
		t.Fatalf("loadMessageEntries with corrupt lines: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("loaded %d entries, want 2 (corrupt line skipped)", len(entries))
	}
}

func TestJSONL_LoadUpTo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "upto.jsonl")

	for _, uuid := range []string{"a", "b", "c", "d"} {
		appendJSONL(path, testMessageEntry(uuid, "user", "msg"))
	}

	entries, err := loadMessageEntriesUpTo(path, "b")
	if err != nil {
		t.Fatalf("loadMessageEntriesUpTo: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("loaded %d entries, want 2 (up to and including 'b')", len(entries))
	}
}

// --- Writer Tests ---

func TestWriter_ConcurrentWrites(t *testing.T) {
	w := newAsyncWriter()
	dir := t.TempDir()
	path := filepath.Join(dir, "concurrent.log")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			data := []byte(fmt.Sprintf("line %d\n", idx))
			errCh := make(chan error, 1)
			w.Write(path, data, errCh)
			if err := <-errCh; err != nil {
				t.Errorf("write %d: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Verify all lines were written
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	if lines != 50 {
		t.Errorf("written lines = %d, want 50", lines)
	}
}

func TestWriter_FlushOnClose(t *testing.T) {
	w := newAsyncWriter()
	dir := t.TempDir()
	path := filepath.Join(dir, "flush.log")

	// Fire-and-forget writes (no errCh)
	for i := 0; i < 10; i++ {
		w.Write(path, []byte(fmt.Sprintf("line %d\n", i)), nil)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	if lines != 10 {
		t.Errorf("written lines after close = %d, want 10", lines)
	}
}

func TestWriter_MultipleFiles(t *testing.T) {
	w := newAsyncWriter()
	dir := t.TempDir()

	path1 := filepath.Join(dir, "file1.log")
	path2 := filepath.Join(dir, "file2.log")

	errCh1 := make(chan error, 1)
	errCh2 := make(chan error, 1)

	w.Write(path1, []byte("hello\n"), errCh1)
	w.Write(path2, []byte("world\n"), errCh2)

	if err := <-errCh1; err != nil {
		t.Errorf("write to file1: %v", err)
	}
	if err := <-errCh2; err != nil {
		t.Errorf("write to file2: %v", err)
	}

	w.Close()

	data1, _ := os.ReadFile(path1)
	data2, _ := os.ReadFile(path2)

	if string(data1) != "hello\n" {
		t.Errorf("file1 = %q, want 'hello\\n'", data1)
	}
	if string(data2) != "world\n" {
		t.Errorf("file2 = %q, want 'world\\n'", data2)
	}
}

// --- Metadata Tests ---

func TestMetadata_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	meta := agent.SessionMetadata{
		ID:           "test-id",
		CWD:          "/tmp/project",
		Model:        "claude-sonnet-4-5-20250929",
		CreatedAt:    time.Now().Truncate(time.Millisecond),
		UpdatedAt:    time.Now().Truncate(time.Millisecond),
		MessageCount: 5,
		TurnCount:    3,
		TotalCostUSD: 0.01,
		LeafTitle:    "My Session",
	}

	if err := saveMetadata(dir, meta); err != nil {
		t.Fatalf("saveMetadata: %v", err)
	}

	loaded, err := loadMetadata(dir)
	if err != nil {
		t.Fatalf("loadMetadata: %v", err)
	}
	if loaded.ID != meta.ID {
		t.Errorf("ID = %q, want %q", loaded.ID, meta.ID)
	}
	if loaded.MessageCount != 5 {
		t.Errorf("MessageCount = %d, want 5", loaded.MessageCount)
	}
	if loaded.LeafTitle != "My Session" {
		t.Errorf("LeafTitle = %q, want 'My Session'", loaded.LeafTitle)
	}
}

// Ensure fmt is used
var _ = fmt.Sprintf

// --- Phase 5: Concurrency Safety & Edge Cases ---

func TestStore_PersistSessionFalse_NoFilesWritten(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir, WithPersistEnabled(false))
	defer s.Close()

	meta := testMetadata("sess-nop", "/tmp")
	if err := s.Create(meta); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Append should be a no-op
	if err := s.AppendMessage("sess-nop", testMessageEntry("uuid-1", "user", "hello")); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}

	// Update should be a no-op
	if err := s.UpdateMetadata("sess-nop", func(m *agent.SessionMetadata) { m.TurnCount = 5 }); err != nil {
		t.Fatalf("UpdateMetadata: %v", err)
	}

	// No files should have been written
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("expected no files written with PersistSession=false, got: %v", names)
	}
}

func TestStore_MissingDirectory_AutoCreated(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "nested", "deep", "sessions")
	s := NewStore(basePath)
	defer s.Close()

	meta := testMetadata("sess-auto", "/tmp")
	if err := s.Create(meta); err != nil {
		t.Fatalf("Create should auto-create directories: %v", err)
	}

	// Verify session dir was created
	info, err := os.Stat(filepath.Join(basePath, "sess-auto"))
	if err != nil {
		t.Fatalf("session dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("session path should be a directory")
	}
}

func TestStore_ConcurrentWriteHighContention(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	meta := testMetadata("sess-hc", "/tmp")
	s.Create(meta)

	var wg sync.WaitGroup
	const writers = 10
	const msgsPerWriter = 5

	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(writer int) {
			defer wg.Done()
			for m := 0; m < msgsPerWriter; m++ {
				entry := testMessageEntry(
					fmt.Sprintf("w%d-m%d", writer, m),
					"user",
					fmt.Sprintf("Writer %d message %d", writer, m),
				)
				s.AppendMessage("sess-hc", entry)
			}
		}(w)
	}
	wg.Wait()

	msgs, _ := s.LoadMessages("sess-hc")
	if len(msgs) != writers*msgsPerWriter {
		t.Errorf("total messages = %d, want %d", len(msgs), writers*msgsPerWriter)
	}
}

func TestStore_EmptySession_ReturnsEmptySlice(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	meta := testMetadata("sess-empty2", "/tmp")
	s.Create(meta)

	msgs, err := s.LoadMessages("sess-empty2")
	if err != nil {
		t.Fatalf("LoadMessages: %v", err)
	}
	// Should be nil (no file exists) not error
	if msgs != nil && len(msgs) != 0 {
		t.Errorf("messages = %v, want nil or empty", msgs)
	}
}

func TestJSONL_CorruptLines_PartialRecovery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "partial.jsonl")

	// Mix of valid, corrupt, empty, and valid entries
	content := `{"uuid":"uuid-1","timestamp":"2024-01-01T00:00:00Z","message":{"role":"user","content":"first"}}
{BADJSON}

{"uuid":"uuid-2","timestamp":"2024-01-01T00:00:01Z","message":{"role":"user","content":"second"}}
null
{"uuid":"uuid-3","timestamp":"2024-01-01T00:00:02Z","message":{"role":"assistant","content":"third"}}
`
	os.WriteFile(path, []byte(content), 0644)

	entries, err := loadMessageEntries(path)
	if err != nil {
		t.Fatalf("loadMessageEntries: %v", err)
	}
	// Should recover uuid-1, uuid-2, uuid-3, + null (zero-value entry)
	// Skip: {BADJSON}, empty line. null unmarshals as zero-value struct.
	if len(entries) != 4 {
		t.Errorf("recovered %d entries, want 4", len(entries))
	}
}
