package tools

import (
	"context"
	"strings"
	"testing"
)

func TestTaskStop_RunningTask(t *testing.T) {
	tm := NewTaskManager()
	started := make(chan struct{})
	tm.Launch(context.Background(), "t1", func(ctx context.Context) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	})
	<-started

	tool := &TaskStopTool{TaskManager: tm}
	out, err := tool.Execute(context.Background(), map[string]any{
		"task_id": "t1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "stopped successfully") {
		t.Errorf("expected success message, got %q", out.Content)
	}
}

func TestTaskStop_CompletedTask(t *testing.T) {
	tm := NewTaskManager()
	task := tm.Launch(context.Background(), "t1", func(ctx context.Context) (string, error) {
		return "done", nil
	})
	<-task.Done

	tool := &TaskStopTool{TaskManager: tm}
	out, err := tool.Execute(context.Background(), map[string]any{
		"task_id": "t1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error stopping completed task")
	}
	if !strings.Contains(out.Content, "not running") {
		t.Errorf("expected 'not running' error, got %q", out.Content)
	}
}

func TestTaskStop_UnknownID(t *testing.T) {
	tm := NewTaskManager()
	tool := &TaskStopTool{TaskManager: tm}
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

func TestTaskStop_MissingTaskID(t *testing.T) {
	tm := NewTaskManager()
	tool := &TaskStopTool{TaskManager: tm}
	out, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for missing task_id")
	}
}

func TestTaskStop_NilManager(t *testing.T) {
	tool := &TaskStopTool{}
	out, err := tool.Execute(context.Background(), map[string]any{"task_id": "t1"})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for nil manager")
	}
}
