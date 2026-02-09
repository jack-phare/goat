package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGlob_SimplePattern(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte(""), 0o644)

	tool := &GlobTool{CWD: dir}
	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "*.go",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	if !strings.Contains(out.Content, "a.go") || !strings.Contains(out.Content, "b.go") {
		t.Errorf("expected a.go and b.go, got %q", out.Content)
	}
	if strings.Contains(out.Content, "c.txt") {
		t.Error("c.txt should not match *.go")
	}
}

func TestGlob_RecursivePattern(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0o755)
	os.WriteFile(filepath.Join(dir, "root.go"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "sub", "mid.go"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "sub", "deep", "leaf.go"), []byte(""), 0o644)

	tool := &GlobTool{CWD: dir}
	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "**/*.go",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	// Should find files in subdirectories
	if !strings.Contains(out.Content, "mid.go") {
		t.Errorf("expected mid.go, got %q", out.Content)
	}
	if !strings.Contains(out.Content, "leaf.go") {
		t.Errorf("expected leaf.go, got %q", out.Content)
	}
}

func TestGlob_NoMatches(t *testing.T) {
	dir := t.TempDir()

	tool := &GlobTool{CWD: dir}
	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "*.xyz",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "No files") {
		t.Errorf("expected 'No files' message, got %q", out.Content)
	}
}

func TestGlobTool_OutputTruncation(t *testing.T) {
	dir := t.TempDir()

	// Create enough files with long names to exceed globMaxOutput (50K chars)
	// Each filename ~100 chars + full path, so we need ~500+ files
	for i := 0; i < 600; i++ {
		name := fmt.Sprintf("file_%04d_%s.txt", i, strings.Repeat("a", 80))
		os.WriteFile(filepath.Join(dir, name), []byte(""), 0o644)
	}

	tool := &GlobTool{CWD: dir}
	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "*.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	if strings.Contains(out.Content, "truncated") {
		if !strings.Contains(out.Content, "total characters") {
			t.Error("truncation message should include total character count")
		}
		if !strings.Contains(out.Content, "files matched") {
			t.Error("truncation message should include file count")
		}
	}
}

func TestGlob_PathOverride(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.MkdirAll(sub, 0o755)
	os.WriteFile(filepath.Join(dir, "root.txt"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(sub, "sub.txt"), []byte(""), 0o644)

	tool := &GlobTool{CWD: dir}
	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "*.txt",
		"path":    sub,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	if !strings.Contains(out.Content, "sub.txt") {
		t.Errorf("expected sub.txt, got %q", out.Content)
	}
	if strings.Contains(out.Content, "root.txt") {
		t.Error("root.txt should not be found when searching in sub/")
	}
}
