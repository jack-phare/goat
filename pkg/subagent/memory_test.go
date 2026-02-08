package subagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveMemoryDir_User(t *testing.T) {
	result := resolveMemoryDir("my-agent", "user", "/home/user/project")
	if result == "" {
		t.Fatal("expected non-empty dir for 'user'")
	}
	if !strings.Contains(result, "agent-memory") {
		t.Errorf("result = %q, should contain 'agent-memory'", result)
	}
	if !strings.Contains(result, "my-agent") {
		t.Errorf("result = %q, should contain agent name", result)
	}
	// Should be under ~/.claude/agent-memory/
	if !strings.Contains(result, ".claude") {
		t.Errorf("result = %q, should be under ~/.claude/", result)
	}
}

func TestResolveMemoryDir_Auto(t *testing.T) {
	// "auto" is an alias for "user"
	auto := resolveMemoryDir("my-agent", "auto", "/home/user/project")
	user := resolveMemoryDir("my-agent", "user", "/home/user/project")
	if auto != user {
		t.Errorf("auto = %q, user = %q; should be equal", auto, user)
	}
}

func TestResolveMemoryDir_Project(t *testing.T) {
	result := resolveMemoryDir("my-agent", "project", "/home/user/project")
	want := filepath.Join("/home/user/project", ".claude", "agent-memory", "my-agent")
	if result != want {
		t.Errorf("result = %q, want %q", result, want)
	}
}

func TestResolveMemoryDir_Local(t *testing.T) {
	result := resolveMemoryDir("my-agent", "local", "/home/user/project")
	want := filepath.Join("/home/user/project", ".claude", "agent-memory-local", "my-agent")
	if result != want {
		t.Errorf("result = %q, want %q", result, want)
	}
}

func TestResolveMemoryDir_Empty(t *testing.T) {
	result := resolveMemoryDir("agent", "", "/cwd")
	if result != "" {
		t.Errorf("expected empty for empty scope, got %q", result)
	}
}

func TestResolveMemoryDir_CustomPath(t *testing.T) {
	result := resolveMemoryDir("agent", "/custom/memory/path", "/cwd")
	if result != "/custom/memory/path" {
		t.Errorf("result = %q, want '/custom/memory/path'", result)
	}
}

func TestEnsureMemoryDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "memory")
	err := ensureMemoryDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("directory should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
}

func TestEnsureMemoryDir_Empty(t *testing.T) {
	err := ensureMemoryDir("")
	if err != nil {
		t.Fatalf("empty dir should not error: %v", err)
	}
}

func TestLoadMemoryContent_Exists(t *testing.T) {
	dir := t.TempDir()
	content := "# Memory\n\nSome notes here.\nLine 3.\n"
	os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte(content), 0o644)

	result, err := loadMemoryContent(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Some notes here.") {
		t.Errorf("result = %q, should contain memory content", result)
	}
}

func TestLoadMemoryContent_NotExists(t *testing.T) {
	dir := t.TempDir()
	result, err := loadMemoryContent(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty for missing file, got %q", result)
	}
}

func TestLoadMemoryContent_Empty(t *testing.T) {
	result, err := loadMemoryContent("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty for empty dir, got %q", result)
	}
}

func TestLoadMemoryContent_Truncated(t *testing.T) {
	dir := t.TempDir()
	// Write a file with more than maxMemoryLines lines
	var lines []string
	for i := 0; i < 300; i++ {
		lines = append(lines, "Line content")
	}
	os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte(strings.Join(lines, "\n")), 0o644)

	result, err := loadMemoryContent(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resultLines := strings.Split(result, "\n")
	if len(resultLines) > maxMemoryLines {
		t.Errorf("result has %d lines, should be <= %d", len(resultLines), maxMemoryLines)
	}
}

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/home/user/project", "home-user-project"},
		{"C:\\Users\\foo", "C--Users-foo"},
		{"simple", "simple"},
	}
	for _, tt := range tests {
		if got := sanitizePath(tt.input); got != tt.want {
			t.Errorf("sanitizePath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
