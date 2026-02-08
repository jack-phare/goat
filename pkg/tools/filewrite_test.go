package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileWrite_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.txt")

	tool := &FileWriteTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"file_path": path,
		"content":   "hello\nworld\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "hello\nworld\n" {
		t.Errorf("file content = %q", string(data))
	}
	if !strings.Contains(out.Content, "2 lines") {
		t.Errorf("expected 2 lines in output, got %q", out.Content)
	}
}

func TestFileWrite_Overwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")
	os.WriteFile(path, []byte("old content"), 0o644)

	tool := &FileWriteTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"file_path": path,
		"content":   "new content",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "new content" {
		t.Errorf("file content = %q, want 'new content'", string(data))
	}
}

func TestFileWrite_CreateNestedDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "deep.txt")

	tool := &FileWriteTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"file_path": path,
		"content":   "deep file",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "deep file" {
		t.Errorf("file content = %q", string(data))
	}
}

func TestFileWrite_RelativePath(t *testing.T) {
	tool := &FileWriteTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"file_path": "relative/path.txt",
		"content":   "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for relative path")
	}
}
