package tools

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestTaskOutput_CompletedTask(t *testing.T) {
	tm := NewTaskManager()
	task := tm.Launch(context.Background(), "t1", func(ctx context.Context) (string, error) {
		return "hello output", nil
	})
	<-task.Done

	tool := &TaskOutputTool{TaskManager: tm}
	out, err := tool.Execute(context.Background(), map[string]any{
		"task_id": "t1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "hello output") {
		t.Errorf("expected output, got %q", out.Content)
	}
	if !strings.Contains(out.Content, "completed") {
		t.Errorf("expected completed status, got %q", out.Content)
	}
}

func TestTaskOutput_BlocksUntilDone(t *testing.T) {
	tm := NewTaskManager()
	tm.Launch(context.Background(), "t1", func(ctx context.Context) (string, error) {
		time.Sleep(50 * time.Millisecond)
		return "waited result", nil
	})

	tool := &TaskOutputTool{TaskManager: tm}
	out, err := tool.Execute(context.Background(), map[string]any{
		"task_id": "t1",
		"block":   true,
		"timeout": float64(5000),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Content, "waited result") {
		t.Errorf("expected result, got %q", out.Content)
	}
}

func TestTaskOutput_TimeoutReturnsPartial(t *testing.T) {
	tm := NewTaskManager()
	tm.Launch(context.Background(), "t1", func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})

	tool := &TaskOutputTool{TaskManager: tm}
	out, err := tool.Execute(context.Background(), map[string]any{
		"task_id": "t1",
		"block":   true,
		"timeout": float64(50),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error on timeout")
	}
	if !strings.Contains(out.Content, "timeout") {
		t.Errorf("expected timeout error, got %q", out.Content)
	}
}

func TestTaskOutput_UnknownID(t *testing.T) {
	tm := NewTaskManager()
	tool := &TaskOutputTool{TaskManager: tm}
	out, err := tool.Execute(context.Background(), map[string]any{
		"task_id": "nope",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for unknown task")
	}
}

func TestTaskOutput_MissingTaskID(t *testing.T) {
	tm := NewTaskManager()
	tool := &TaskOutputTool{TaskManager: tm}
	out, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for missing task_id")
	}
}

func TestTaskOutput_NilManager(t *testing.T) {
	tool := &TaskOutputTool{}
	out, err := tool.Execute(context.Background(), map[string]any{"task_id": "t1"})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for nil manager")
	}
}
