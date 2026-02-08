package tools

import (
	"context"
	"strings"
	"testing"
)

func TestTodoWrite_CreateList(t *testing.T) {
	tool := &TodoWriteTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"todos": []any{
			map[string]any{"content": "Write tests", "status": "pending"},
			map[string]any{"content": "Implement feature", "status": "in_progress", "activeForm": "Implementing feature"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "Write tests") {
		t.Errorf("expected content, got %q", out.Content)
	}
	if !strings.Contains(out.Content, "[~]") {
		t.Errorf("expected in_progress marker, got %q", out.Content)
	}
	if len(tool.Todos) != 2 {
		t.Errorf("expected 2 todos, got %d", len(tool.Todos))
	}
}

func TestTodoWrite_OverwriteList(t *testing.T) {
	tool := &TodoWriteTool{}

	// Create initial list
	tool.Execute(context.Background(), map[string]any{
		"todos": []any{
			map[string]any{"content": "Old task", "status": "pending"},
		},
	})

	// Overwrite with new list
	out, err := tool.Execute(context.Background(), map[string]any{
		"todos": []any{
			map[string]any{"content": "New task", "status": "completed"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Content, "New task") {
		t.Errorf("expected new content, got %q", out.Content)
	}
	if !strings.Contains(out.Content, "[x]") {
		t.Errorf("expected completed marker, got %q", out.Content)
	}
	if len(tool.Todos) != 1 {
		t.Errorf("expected 1 todo, got %d", len(tool.Todos))
	}
	if tool.Todos[0].Content != "New task" {
		t.Errorf("expected 'New task', got %q", tool.Todos[0].Content)
	}
}

func TestTodoWrite_InvalidStatus(t *testing.T) {
	tool := &TodoWriteTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"todos": []any{
			map[string]any{"content": "Bad task", "status": "invalid"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for invalid status")
	}
}

func TestTodoWrite_EmptyArray(t *testing.T) {
	tool := &TodoWriteTool{
		Todos: []TodoItem{{Content: "Old", Status: "pending"}},
	}
	out, err := tool.Execute(context.Background(), map[string]any{
		"todos": []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "cleared") {
		t.Errorf("expected cleared message, got %q", out.Content)
	}
	if len(tool.Todos) != 0 {
		t.Errorf("expected empty list, got %d items", len(tool.Todos))
	}
}

func TestTodoWrite_MissingContent(t *testing.T) {
	tool := &TodoWriteTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"todos": []any{
			map[string]any{"status": "pending"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for missing content")
	}
}

func TestTodoWrite_MissingTodos(t *testing.T) {
	tool := &TodoWriteTool{}
	out, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for missing todos")
	}
}
