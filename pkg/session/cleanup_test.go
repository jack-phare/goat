package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jg-phare/goat/pkg/agent"
)

// writeSessionMetadata creates a session directory with metadata at the given UpdatedAt time.
func writeSessionMetadata(t *testing.T, baseDir, sessionID string, updatedAt time.Time) {
	t.Helper()
	dir := filepath.Join(baseDir, sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := agent.SessionMetadata{
		ID:        sessionID,
		CWD:       "/tmp/test",
		CreatedAt: updatedAt.Add(-time.Hour),
		UpdatedAt: updatedAt,
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, metadataFile), data, 0o644); err != nil {
		t.Fatal(err)
	}
	// Also write a dummy messages file so we can verify bytes freed
	if err := os.WriteFile(filepath.Join(dir, "messages.jsonl"), []byte("test data\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCleanup_DeletesOldSessions(t *testing.T) {
	baseDir := t.TempDir()

	now := time.Now()
	old := now.AddDate(0, 0, -60) // 60 days ago

	writeSessionMetadata(t, baseDir, "old-session", old)
	writeSessionMetadata(t, baseDir, "recent-session", now)

	stats, err := Cleanup(baseDir, CleanupConfig{RetentionDays: 30})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stats.SessionsDeleted != 1 {
		t.Errorf("SessionsDeleted = %d, want 1", stats.SessionsDeleted)
	}
	if stats.BytesFreed <= 0 {
		t.Error("BytesFreed should be > 0")
	}

	// old-session should be gone
	if _, err := os.Stat(filepath.Join(baseDir, "old-session")); !os.IsNotExist(err) {
		t.Error("old-session should have been deleted")
	}
	// recent-session should still exist
	if _, err := os.Stat(filepath.Join(baseDir, "recent-session")); os.IsNotExist(err) {
		t.Error("recent-session should still exist")
	}
}

func TestCleanup_RecentSessionsPreserved(t *testing.T) {
	baseDir := t.TempDir()

	now := time.Now()
	writeSessionMetadata(t, baseDir, "session-a", now.AddDate(0, 0, -5))
	writeSessionMetadata(t, baseDir, "session-b", now.AddDate(0, 0, -10))
	writeSessionMetadata(t, baseDir, "session-c", now.AddDate(0, 0, -29))

	stats, err := Cleanup(baseDir, CleanupConfig{RetentionDays: 30})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stats.SessionsDeleted != 0 {
		t.Errorf("SessionsDeleted = %d, want 0 (all within retention)", stats.SessionsDeleted)
	}

	// All sessions should remain
	for _, id := range []string{"session-a", "session-b", "session-c"} {
		if _, err := os.Stat(filepath.Join(baseDir, id)); os.IsNotExist(err) {
			t.Errorf("%s should not have been deleted", id)
		}
	}
}

func TestCleanup_PreservesAutoMemoryDir(t *testing.T) {
	baseDir := t.TempDir()

	// Create a "memory" directory (auto-memory)
	memDir := filepath.Join(baseDir, "memory")
	os.MkdirAll(memDir, 0o755)
	os.WriteFile(filepath.Join(memDir, "MEMORY.md"), []byte("# Memory"), 0o644)

	// Create an old session that should be deleted
	old := time.Now().AddDate(0, 0, -60)
	writeSessionMetadata(t, baseDir, "old-session", old)

	stats, err := Cleanup(baseDir, CleanupConfig{RetentionDays: 30})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stats.SessionsDeleted != 1 {
		t.Errorf("SessionsDeleted = %d, want 1", stats.SessionsDeleted)
	}

	// memory dir must still exist
	if _, err := os.Stat(memDir); os.IsNotExist(err) {
		t.Error("memory directory should be preserved (never deleted)")
	}
	content, _ := os.ReadFile(filepath.Join(memDir, "MEMORY.md"))
	if string(content) != "# Memory" {
		t.Error("MEMORY.md content should be preserved")
	}
}

func TestCleanup_PreservesAgentMemoryDir(t *testing.T) {
	baseDir := t.TempDir()

	// Create an "agent-memory" directory
	agentMemDir := filepath.Join(baseDir, "agent-memory")
	os.MkdirAll(agentMemDir, 0o755)
	os.WriteFile(filepath.Join(agentMemDir, "notes.md"), []byte("# Agent notes"), 0o644)

	// Also test custom memory dirs ending with "-memory"
	customMemDir := filepath.Join(baseDir, "explore-memory")
	os.MkdirAll(customMemDir, 0o755)
	os.WriteFile(filepath.Join(customMemDir, "MEMORY.md"), []byte("# Explore"), 0o644)

	// Create an old session
	old := time.Now().AddDate(0, 0, -60)
	writeSessionMetadata(t, baseDir, "old-session", old)

	stats, err := Cleanup(baseDir, CleanupConfig{RetentionDays: 30})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stats.SessionsDeleted != 1 {
		t.Errorf("SessionsDeleted = %d, want 1", stats.SessionsDeleted)
	}

	// Both memory dirs must be preserved
	if _, err := os.Stat(agentMemDir); os.IsNotExist(err) {
		t.Error("agent-memory directory should be preserved")
	}
	if _, err := os.Stat(customMemDir); os.IsNotExist(err) {
		t.Error("explore-memory directory should be preserved")
	}
}

func TestCleanup_DefaultRetention(t *testing.T) {
	baseDir := t.TempDir()

	// 31 days old — should be deleted with default 30-day retention
	old := time.Now().AddDate(0, 0, -31)
	writeSessionMetadata(t, baseDir, "old-session", old)

	stats, err := Cleanup(baseDir, CleanupConfig{}) // RetentionDays=0 → default 30
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stats.SessionsDeleted != 1 {
		t.Errorf("SessionsDeleted = %d, want 1 (default 30-day retention)", stats.SessionsDeleted)
	}
}

func TestCleanup_EmptyBaseDir(t *testing.T) {
	baseDir := t.TempDir()

	stats, err := Cleanup(baseDir, CleanupConfig{RetentionDays: 30})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stats.SessionsDeleted != 0 {
		t.Errorf("SessionsDeleted = %d, want 0", stats.SessionsDeleted)
	}
}

func TestCleanup_NonexistentBaseDir(t *testing.T) {
	stats, err := Cleanup("/nonexistent/path/sessions", CleanupConfig{RetentionDays: 30})
	if err != nil {
		t.Fatalf("unexpected error for nonexistent dir: %v", err)
	}
	if stats.SessionsDeleted != 0 {
		t.Errorf("SessionsDeleted = %d, want 0", stats.SessionsDeleted)
	}
}

func TestCleanup_CorruptMetadataFallback(t *testing.T) {
	baseDir := t.TempDir()

	// Create a dir with corrupt metadata
	dir := filepath.Join(baseDir, "corrupt-session")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, metadataFile), []byte("not json"), 0o644)

	// Set the directory modification time to be old
	oldTime := time.Now().AddDate(0, 0, -60)
	os.Chtimes(dir, oldTime, oldTime)

	stats, err := Cleanup(baseDir, CleanupConfig{RetentionDays: 30})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should still be deleted based on dir mod time fallback
	if stats.SessionsDeleted != 1 {
		t.Errorf("SessionsDeleted = %d, want 1 (corrupt metadata should use dir mtime fallback)", stats.SessionsDeleted)
	}
}

func TestCleanup_UsesUpdatedAtNotCreatedAt(t *testing.T) {
	baseDir := t.TempDir()

	// Session created 60 days ago but updated 5 days ago — should NOT be deleted
	now := time.Now()
	dir := filepath.Join(baseDir, "active-old-session")
	os.MkdirAll(dir, 0o755)
	meta := agent.SessionMetadata{
		ID:        "active-old-session",
		CWD:       "/tmp/test",
		CreatedAt: now.AddDate(0, 0, -60),
		UpdatedAt: now.AddDate(0, 0, -5),
	}
	data, _ := json.MarshalIndent(meta, "", "  ")
	os.WriteFile(filepath.Join(dir, metadataFile), data, 0o644)

	stats, err := Cleanup(baseDir, CleanupConfig{RetentionDays: 30})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stats.SessionsDeleted != 0 {
		t.Errorf("SessionsDeleted = %d, want 0 (UpdatedAt is recent)", stats.SessionsDeleted)
	}
}

func TestIsProtectedDir(t *testing.T) {
	tests := []struct {
		name      string
		protected bool
	}{
		{"memory", true},
		{"agent-memory", true},
		{"explore-memory", true},
		{"custom-memory", true},
		{"session-abc123", false},
		{"some-dir", false},
		{"memories", false}, // not a protected pattern
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isProtectedDir(tt.name)
			if got != tt.protected {
				t.Errorf("isProtectedDir(%q) = %v, want %v", tt.name, got, tt.protected)
			}
		})
	}
}
