package session

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/llm"
)

// --- Full Lifecycle Integration Tests ---

func TestIntegration_CreateAppendLoadRoundtrip(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	meta := testMetadata("sess-int-1", "/tmp/project")
	if err := s.Create(meta); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Append a conversation
	msgs := []agent.MessageEntry{
		testMessageEntry("uuid-1", "user", "Hello, what's 2+2?"),
		testMessageEntry("uuid-2", "assistant", "The answer is 4."),
		testMessageEntry("uuid-3", "user", "And 3+3?"),
		testMessageEntry("uuid-4", "assistant", "That's 6."),
	}
	for _, m := range msgs {
		if err := s.AppendMessage("sess-int-1", m); err != nil {
			t.Fatalf("AppendMessage: %v", err)
		}
	}

	// Update metadata
	s.UpdateMetadata("sess-int-1", func(m *agent.SessionMetadata) {
		m.MessageCount = 4
		m.TurnCount = 2
		m.TotalCostUSD = 0.001
	})

	// Load and verify
	state, err := s.Load("sess-int-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(state.Messages) != 4 {
		t.Fatalf("loaded %d messages, want 4", len(state.Messages))
	}
	if state.Metadata.MessageCount != 4 {
		t.Errorf("metadata.MessageCount = %d, want 4", state.Metadata.MessageCount)
	}
	if state.Metadata.TurnCount != 2 {
		t.Errorf("metadata.TurnCount = %d, want 2", state.Metadata.TurnCount)
	}

	// Verify message content
	if state.Messages[0].Message.Content != "Hello, what's 2+2?" {
		t.Errorf("first message = %v, want 'Hello, what's 2+2?'", state.Messages[0].Message.Content)
	}
	if state.Messages[1].Message.Role != "assistant" {
		t.Errorf("second message role = %q, want assistant", state.Messages[1].Message.Role)
	}
}

func TestIntegration_CheckpointModifyRewind(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	meta := testMetadata("sess-int-2", "/tmp")
	s.Create(meta)

	// Create test files
	dir := t.TempDir()
	file1 := filepath.Join(dir, "main.go")
	file2 := filepath.Join(dir, "config.yaml")
	writeTestFile(t, file1, "package main\n")
	writeTestFile(t, file2, "key: value\n")

	// Checkpoint
	if err := s.CreateCheckpoint("sess-int-2", "user-msg-1", []string{file1, file2}); err != nil {
		t.Fatalf("CreateCheckpoint: %v", err)
	}

	// Modify files
	writeTestFile(t, file1, "package main\n\nfunc main() {}\n")
	writeTestFile(t, file2, "key: modified\n")

	// Rewind
	result, err := s.RewindFiles("sess-int-2", "user-msg-1", false)
	if err != nil {
		t.Fatalf("RewindFiles: %v", err)
	}
	if !result.CanRewind {
		t.Fatalf("CanRewind=false; error: %s", result.Error)
	}
	if len(result.FilesChanged) != 2 {
		t.Errorf("FilesChanged = %d, want 2", len(result.FilesChanged))
	}

	// Verify restored content
	data1, _ := os.ReadFile(file1)
	data2, _ := os.ReadFile(file2)
	if string(data1) != "package main\n" {
		t.Errorf("file1 after rewind = %q, want 'package main\\n'", data1)
	}
	if string(data2) != "key: value\n" {
		t.Errorf("file2 after rewind = %q, want 'key: value\\n'", data2)
	}
}

func TestIntegration_ForkIndependence(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	// Create source session
	meta := testMetadata("sess-src", "/tmp/project")
	s.Create(meta)
	s.AppendMessage("sess-src", testMessageEntry("uuid-1", "user", "First"))
	s.AppendMessage("sess-src", testMessageEntry("uuid-2", "assistant", "Response"))

	// Fork
	forked, err := s.Fork("sess-src", "sess-fork")
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if forked.Metadata.ParentSessionID != "sess-src" {
		t.Errorf("parent = %q, want sess-src", forked.Metadata.ParentSessionID)
	}

	// Add messages to fork
	s.AppendMessage("sess-fork", testMessageEntry("uuid-3", "user", "Fork-only message"))

	// Add message to source
	s.AppendMessage("sess-src", testMessageEntry("uuid-4", "user", "Source-only message"))

	// Verify independence
	srcMsgs, _ := s.LoadMessages("sess-src")
	forkMsgs, _ := s.LoadMessages("sess-fork")

	if len(srcMsgs) != 3 {
		t.Errorf("source messages = %d, want 3", len(srcMsgs))
	}
	if len(forkMsgs) != 3 {
		t.Errorf("fork messages = %d, want 3", len(forkMsgs))
	}

	// Source should have uuid-4, fork should have uuid-3
	if srcMsgs[2].UUID != "uuid-4" {
		t.Errorf("source last msg UUID = %q, want uuid-4", srcMsgs[2].UUID)
	}
	if forkMsgs[2].UUID != "uuid-3" {
		t.Errorf("fork last msg UUID = %q, want uuid-3", forkMsgs[2].UUID)
	}
}

func TestIntegration_LoadLatestMultipleSessions(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	cwd := "/tmp/my-project"

	// Create 3 sessions with increasing timestamps
	for i, id := range []string{"sess-old", "sess-mid", "sess-new"} {
		meta := testMetadata(id, cwd)
		meta.UpdatedAt = time.Now().Add(time.Duration(i) * time.Hour)
		s.Create(meta)
	}

	// LoadLatest should return the most recent
	state, err := s.LoadLatest(cwd)
	if err != nil {
		t.Fatalf("LoadLatest: %v", err)
	}
	if state.Metadata.ID != "sess-new" {
		t.Errorf("LoadLatest returned %q, want sess-new", state.Metadata.ID)
	}
}

func TestIntegration_ResumeAtSpecificUUID(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	meta := testMetadata("sess-resume", "/tmp")
	s.Create(meta)

	for i := 1; i <= 10; i++ {
		s.AppendMessage("sess-resume", testMessageEntry(
			fmt.Sprintf("uuid-%d", i),
			"user",
			fmt.Sprintf("Message %d", i),
		))
	}

	// Load up to uuid-5
	msgs, err := s.LoadMessagesUpTo("sess-resume", "uuid-5")
	if err != nil {
		t.Fatalf("LoadMessagesUpTo: %v", err)
	}
	if len(msgs) != 5 {
		t.Fatalf("loaded %d messages, want 5", len(msgs))
	}
	if msgs[4].UUID != "uuid-5" {
		t.Errorf("last loaded UUID = %q, want uuid-5", msgs[4].UUID)
	}
}

// --- Large Session Test ---

func TestIntegration_LargeSession(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	meta := testMetadata("sess-large", "/tmp")
	s.Create(meta)

	const numMessages = 1000

	start := time.Now()
	for i := 0; i < numMessages; i++ {
		entry := agent.MessageEntry{
			UUID:      fmt.Sprintf("uuid-%d", i),
			Timestamp: time.Now(),
			Message: llm.ChatMessage{
				Role:    "user",
				Content: fmt.Sprintf("This is message number %d with some padding to make it realistic in size for testing purposes", i),
			},
		}
		if err := s.AppendMessage("sess-large", entry); err != nil {
			t.Fatalf("AppendMessage(%d): %v", i, err)
		}
	}
	appendDuration := time.Since(start)

	// Load all messages
	loadStart := time.Now()
	msgs, err := s.LoadMessages("sess-large")
	loadDuration := time.Since(loadStart)

	if err != nil {
		t.Fatalf("LoadMessages: %v", err)
	}
	if len(msgs) != numMessages {
		t.Fatalf("loaded %d messages, want %d", len(msgs), numMessages)
	}

	// Verify first and last
	if msgs[0].UUID != "uuid-0" {
		t.Errorf("first UUID = %q, want uuid-0", msgs[0].UUID)
	}
	if msgs[numMessages-1].UUID != fmt.Sprintf("uuid-%d", numMessages-1) {
		t.Errorf("last UUID = %q, want uuid-%d", msgs[numMessages-1].UUID, numMessages-1)
	}

	// Log performance (not strict assertions, just visibility)
	t.Logf("Appended %d messages in %v (%.0f msgs/sec)", numMessages, appendDuration,
		float64(numMessages)/appendDuration.Seconds())
	t.Logf("Loaded %d messages in %v (%.0f msgs/sec)", numMessages, loadDuration,
		float64(numMessages)/loadDuration.Seconds())

	// Verify JSONL file size grows linearly
	info, err := os.Stat(s.messagesPath("sess-large"))
	if err != nil {
		t.Fatalf("stat messages file: %v", err)
	}
	bytesPerMessage := float64(info.Size()) / float64(numMessages)
	t.Logf("JSONL file size: %d bytes (%.0f bytes/msg)", info.Size(), bytesPerMessage)
}

// --- JSONL Human Readability Test ---

func TestIntegration_JSONLHumanReadable(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	meta := testMetadata("sess-readable", "/tmp")
	s.Create(meta)

	s.AppendMessage("sess-readable", testMessageEntry("uuid-1", "user", "Hello"))
	s.AppendMessage("sess-readable", testMessageEntry("uuid-2", "assistant", "World"))

	// Read raw JSONL and verify it's human-readable (one JSON object per line)
	data, err := os.ReadFile(s.messagesPath("sess-readable"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	if lines != 2 {
		t.Errorf("JSONL lines = %d, want 2 (one per message)", lines)
	}

	// Should contain recognizable content
	content := string(data)
	if !containsSubpath(content, "Hello") {
		t.Error("JSONL should contain 'Hello'")
	}
	if !containsSubpath(content, "World") {
		t.Error("JSONL should contain 'World'")
	}
}
