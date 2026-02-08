package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileRead_FullFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0o644)

	tool := &FileReadTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"file_path": path,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	// Should contain line numbers
	if !strings.Contains(out.Content, "1\tline1") {
		t.Errorf("expected line 1, got %q", out.Content)
	}
	if !strings.Contains(out.Content, "3\tline3") {
		t.Errorf("expected line 3, got %q", out.Content)
	}
}

func TestFileRead_OffsetAndLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("a\nb\nc\nd\ne\n"), 0o644)

	tool := &FileReadTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"file_path": path,
		"offset":    float64(2),
		"limit":     float64(2),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	lines := strings.Split(out.Content, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), out.Content)
	}
	if !strings.Contains(lines[0], "b") {
		t.Errorf("expected 'b' on first line, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "c") {
		t.Errorf("expected 'c' on second line, got %q", lines[1])
	}
}

func TestFileRead_NonexistentFile(t *testing.T) {
	tool := &FileReadTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"file_path": "/nonexistent/path/file.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for nonexistent file")
	}
}

func TestFileRead_RelativePath(t *testing.T) {
	tool := &FileReadTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"file_path": "relative/path.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for relative path")
	}
	if !strings.Contains(out.Content, "absolute") {
		t.Errorf("expected error about absolute path, got %q", out.Content)
	}
}

func TestFileRead_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	os.WriteFile(path, []byte{}, 0o644)

	tool := &FileReadTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"file_path": path,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "empty") {
		t.Errorf("expected empty file message, got %q", out.Content)
	}
}
