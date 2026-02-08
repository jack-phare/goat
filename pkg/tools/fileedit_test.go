package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileEdit_SingleMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world\nfoo bar\n"), 0o644)

	tool := &FileEditTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"file_path":  path,
		"old_string": "hello world",
		"new_string": "hi earth",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "hi earth") {
		t.Errorf("expected 'hi earth', got %q", string(data))
	}
	if strings.Contains(string(data), "hello world") {
		t.Error("old string should be replaced")
	}
}

func TestFileEdit_MultipleMatchWithReplaceAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("foo bar foo baz foo\n"), 0o644)

	tool := &FileEditTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"file_path":   path,
		"old_string":  "foo",
		"new_string":  "qux",
		"replace_all": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "foo") {
		t.Error("expected all 'foo' replaced")
	}
	if !strings.Contains(out.Content, "3") {
		t.Errorf("expected 3 occurrences reported, got %q", out.Content)
	}
}

func TestFileEdit_MultipleMatchWithoutReplaceAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("foo bar foo\n"), 0o644)

	tool := &FileEditTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"file_path":  path,
		"old_string": "foo",
		"new_string": "qux",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error when multiple matches without replace_all")
	}
	if !strings.Contains(out.Content, "2 times") {
		t.Errorf("expected '2 times' in error, got %q", out.Content)
	}
}

func TestFileEdit_ZeroMatches(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world\n"), 0o644)

	tool := &FileEditTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"file_path":  path,
		"old_string": "not found",
		"new_string": "replacement",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for zero matches")
	}
	if !strings.Contains(out.Content, "not found") {
		t.Errorf("expected 'not found' in error, got %q", out.Content)
	}
}

func TestFileEdit_SameString(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello\n"), 0o644)

	tool := &FileEditTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"file_path":  path,
		"old_string": "hello",
		"new_string": "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error when old_string == new_string")
	}
}
