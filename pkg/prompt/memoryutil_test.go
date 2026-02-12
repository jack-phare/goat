package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFirstNLines_BasicFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	writeFile(t, path, "line1\nline2\nline3")

	content, err := LoadFirstNLines(path, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "line1\nline2\nline3" {
		t.Errorf("got %q, want %q", content, "line1\nline2\nline3")
	}
}

func TestLoadFirstNLines_TruncatesAtLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")

	var lines []string
	for i := 0; i < 300; i++ {
		lines = append(lines, "line")
	}
	writeFile(t, path, strings.Join(lines, "\n"))

	content, err := LoadFirstNLines(path, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultLines := strings.Split(content, "\n")
	if len(resultLines) != 200 {
		t.Errorf("got %d lines, want 200", len(resultLines))
	}
}

func TestLoadFirstNLines_MissingFile(t *testing.T) {
	content, err := LoadFirstNLines("/nonexistent/path/MEMORY.md", 200)
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if content != "" {
		t.Errorf("expected empty string, got %q", content)
	}
}

func TestLoadFirstNLines_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	writeFile(t, path, "")

	content, err := LoadFirstNLines(path, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "" {
		t.Errorf("expected empty string, got %q", content)
	}
}

func TestProjectHash_ConsistentForSamePath(t *testing.T) {
	// Use a non-git directory so it falls back to sanitizePath
	dir := t.TempDir()
	h1 := ProjectHash(dir)
	h2 := ProjectHash(dir)
	if h1 != h2 {
		t.Errorf("ProjectHash should be consistent: %q != %q", h1, h2)
	}
	if h1 == "" {
		t.Error("ProjectHash should not be empty")
	}
}

func TestProjectHash_DifferentForDifferentPaths(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	h1 := ProjectHash(dir1)
	h2 := ProjectHash(dir2)
	if h1 == h2 {
		t.Errorf("different paths should produce different hashes: %q == %q", h1, h2)
	}
}

func TestResolveAutoMemoryDir_CreatesDir(t *testing.T) {
	// Use a temp dir as CWD (not in a git repo)
	dir := t.TempDir()
	memDir := ResolveAutoMemoryDir(dir)
	if memDir == "" {
		t.Fatal("expected non-empty memory dir")
	}

	// Verify directory was created
	info, err := os.Stat(memDir)
	if err != nil {
		t.Fatalf("memory dir should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("memory dir should be a directory")
	}
}

func TestLoadAutoMemory_NoFile(t *testing.T) {
	dir := t.TempDir()
	content := LoadAutoMemory(dir)
	if content != "" {
		t.Errorf("expected empty content when no MEMORY.md exists, got %q", content)
	}
}

func TestLoadAutoMemory_WithContent(t *testing.T) {
	// Create a fake auto-memory dir structure
	dir := t.TempDir()
	memDir := ResolveAutoMemoryDir(dir)
	if memDir == "" {
		t.Fatal("expected non-empty memory dir")
	}

	// Write a MEMORY.md file
	writeFile(t, filepath.Join(memDir, "MEMORY.md"), "# Project Memory\n\nSome notes here.")

	content := LoadAutoMemory(dir)
	if !strings.Contains(content, "# Project Memory") {
		t.Errorf("expected content to contain '# Project Memory', got %q", content)
	}
	if !strings.Contains(content, "Some notes here.") {
		t.Errorf("expected content to contain 'Some notes here.', got %q", content)
	}
}

func TestLoadAutoMemory_TruncatesAt200Lines(t *testing.T) {
	dir := t.TempDir()
	memDir := ResolveAutoMemoryDir(dir)
	if memDir == "" {
		t.Fatal("expected non-empty memory dir")
	}

	// Write a large MEMORY.md
	var lines []string
	for i := 0; i < 300; i++ {
		lines = append(lines, "line content")
	}
	writeFile(t, filepath.Join(memDir, "MEMORY.md"), strings.Join(lines, "\n"))

	content := LoadAutoMemory(dir)
	resultLines := strings.Split(content, "\n")
	if len(resultLines) != 200 {
		t.Errorf("expected 200 lines, got %d", len(resultLines))
	}
}

func TestSanitizePathForMemory(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/home/user/project", "home-user-project"},
		{"C:\\Users\\test", "C--Users-test"},
		{"/tmp/a b/c", "tmp-a-b-c"},
		{"simple", "simple"},
	}
	for _, tt := range tests {
		got := sanitizePathForMemory(tt.input)
		if got != tt.want {
			t.Errorf("sanitizePathForMemory(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
