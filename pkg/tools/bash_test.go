package tools

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestBash_SimpleCommand(t *testing.T) {
	tool := &BashTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"command": "echo hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if out.Content != "hello" {
		t.Errorf("got %q, want %q", out.Content, "hello")
	}
}

func TestBash_StderrCapture(t *testing.T) {
	tool := &BashTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"command": "echo stderr_msg >&2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Content, "stderr_msg") {
		t.Errorf("expected stderr_msg in output, got %q", out.Content)
	}
}

func TestBash_NonZeroExit(t *testing.T) {
	tool := &BashTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"command": "exit 1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected IsError for non-zero exit")
	}
}

func TestBash_Timeout(t *testing.T) {
	tool := &BashTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"command": "sleep 10",
		"timeout": float64(100), // 100ms
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error on timeout")
	}
	if !strings.Contains(out.Content, "timed out") {
		t.Errorf("expected timeout message, got %q", out.Content)
	}
}

func TestBash_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	tool := &BashTool{}
	out, err := tool.Execute(ctx, map[string]any{
		"command": "sleep 10",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error on context cancel")
	}
}

func TestBash_MissingCommand(t *testing.T) {
	tool := &BashTool{}
	out, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for missing command")
	}
}

func TestBash_CWD(t *testing.T) {
	tool := &BashTool{CWD: "/tmp"}
	out, err := tool.Execute(context.Background(), map[string]any{
		"command": "pwd",
	})
	if err != nil {
		t.Fatal(err)
	}
	// On macOS, /tmp is a symlink to /private/tmp
	if !strings.Contains(out.Content, "tmp") {
		t.Errorf("expected CWD /tmp, got %q", out.Content)
	}
}

func TestBash_BackgroundReturnsTaskID(t *testing.T) {
	tm := NewTaskManager()
	tool := &BashTool{TaskManager: tm}
	out, err := tool.Execute(context.Background(), map[string]any{
		"command":           "echo bg_output",
		"run_in_background": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "Task started in background") {
		t.Errorf("expected background message, got %q", out.Content)
	}
}

func TestBash_BackgroundOutputRetrievable(t *testing.T) {
	tm := NewTaskManager()
	tool := &BashTool{TaskManager: tm}
	out, err := tool.Execute(context.Background(), map[string]any{
		"command":           "echo retrievable_output",
		"run_in_background": true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Extract task ID from output
	// Output format: "Task started in background with ID: <id>\n..."
	lines := strings.Split(out.Content, "\n")
	idLine := lines[0]
	parts := strings.Split(idLine, ": ")
	taskID := parts[len(parts)-1]

	// Wait for output
	output, err := tm.GetOutput(taskID, true, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if output != "retrievable_output" {
		t.Errorf("got %q, want %q", output, "retrievable_output")
	}
}

func TestBash_BackgroundNoTaskManager(t *testing.T) {
	tool := &BashTool{} // no TaskManager
	out, err := tool.Execute(context.Background(), map[string]any{
		"command":           "echo test",
		"run_in_background": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error when no task manager")
	}
}

func TestBash_LargeOutput(t *testing.T) {
	tool := &BashTool{}
	// Generate output larger than bashMaxOutput
	out, err := tool.Execute(context.Background(), map[string]any{
		"command": "python3 -c 'print(\"x\" * 40000)'",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Content) > bashMaxOutput+200 { // +200 for truncation message
		t.Errorf("output not truncated: %d chars", len(out.Content))
	}
	if !strings.Contains(out.Content, "truncated") {
		t.Error("expected truncation message")
	}
}
