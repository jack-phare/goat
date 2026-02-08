package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadClaudeMD_AtCWD(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "project instructions")

	result := LoadClaudeMD(dir)
	if result != "project instructions" {
		t.Errorf("expected 'project instructions', got %q", result)
	}
}

func TestLoadClaudeMD_DotClaudeDir(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".claude"), 0o755)
	writeFile(t, filepath.Join(dir, ".claude", "CLAUDE.md"), "dot-claude instructions")

	result := LoadClaudeMD(dir)
	if result != "dot-claude instructions" {
		t.Errorf("expected 'dot-claude instructions', got %q", result)
	}
}

func TestLoadClaudeMD_LocalFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "CLAUDE.local.md"), "local overrides")

	result := LoadClaudeMD(dir)
	if result != "local overrides" {
		t.Errorf("expected 'local overrides', got %q", result)
	}
}

func TestLoadClaudeMD_MultipleFilesMerged(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".claude"), 0o755)

	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "main instructions")
	writeFile(t, filepath.Join(dir, ".claude", "CLAUDE.md"), "dot-claude instructions")
	writeFile(t, filepath.Join(dir, "CLAUDE.local.md"), "local overrides")

	result := LoadClaudeMD(dir)

	parts := strings.Split(result, "\n\n---\n\n")
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %d: %q", len(parts), result)
	}
	if parts[0] != "main instructions" {
		t.Errorf("part[0] = %q, want 'main instructions'", parts[0])
	}
	if parts[1] != "dot-claude instructions" {
		t.Errorf("part[1] = %q, want 'dot-claude instructions'", parts[1])
	}
	if parts[2] != "local overrides" {
		t.Errorf("part[2] = %q, want 'local overrides'", parts[2])
	}
}

func TestLoadClaudeMD_ParentDirectoryWalking(t *testing.T) {
	// Create a nested directory structure
	root := t.TempDir()
	child := filepath.Join(root, "project")
	grandchild := filepath.Join(child, "src")
	os.MkdirAll(grandchild, 0o755)

	writeFile(t, filepath.Join(root, "CLAUDE.md"), "root instructions")
	writeFile(t, filepath.Join(child, "CLAUDE.md"), "project instructions")

	// Load from grandchild — should find project and root
	result := LoadClaudeMD(grandchild)

	parts := strings.Split(result, "\n\n---\n\n")
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d: %q", len(parts), result)
	}
	// First found going up: project, then root
	if parts[0] != "project instructions" {
		t.Errorf("part[0] = %q, want 'project instructions'", parts[0])
	}
	if parts[1] != "root instructions" {
		t.Errorf("part[1] = %q, want 'root instructions'", parts[1])
	}
}

func TestLoadClaudeMD_NoFiles(t *testing.T) {
	dir := t.TempDir()
	result := LoadClaudeMD(dir)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestLoadClaudeMD_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "")

	result := LoadClaudeMD(dir)
	if result != "" {
		t.Errorf("expected empty string for empty file, got %q", result)
	}
}

func TestLoadClaudeMD_WhitespaceOnlyFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "   \n\n  ")

	result := LoadClaudeMD(dir)
	if result != "" {
		t.Errorf("expected empty string for whitespace-only file, got %q", result)
	}
}

func TestLoadClaudeMD_CWDAndParentCombined(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "project")
	os.MkdirAll(child, 0o755)
	os.MkdirAll(filepath.Join(child, ".claude"), 0o755)

	writeFile(t, filepath.Join(root, "CLAUDE.md"), "root")
	writeFile(t, filepath.Join(child, "CLAUDE.md"), "project")
	writeFile(t, filepath.Join(child, ".claude", "CLAUDE.md"), "dot-claude")
	writeFile(t, filepath.Join(child, "CLAUDE.local.md"), "local")

	result := LoadClaudeMD(child)
	parts := strings.Split(result, "\n\n---\n\n")
	if len(parts) != 4 {
		t.Fatalf("expected 4 parts, got %d: %q", len(parts), result)
	}
	if parts[0] != "project" {
		t.Errorf("part[0] = %q, want 'project'", parts[0])
	}
	if parts[1] != "dot-claude" {
		t.Errorf("part[1] = %q, want 'dot-claude'", parts[1])
	}
	if parts[2] != "local" {
		t.Errorf("part[2] = %q, want 'local'", parts[2])
	}
	if parts[3] != "root" {
		t.Errorf("part[3] = %q, want 'root'", parts[3])
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}
