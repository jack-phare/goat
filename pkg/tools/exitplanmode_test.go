package tools

import (
	"context"
	"strings"
	"testing"
)

func TestExitPlanMode_NoPrompts(t *testing.T) {
	tool := &ExitPlanModeTool{}
	out, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "Exiting plan mode") {
		t.Errorf("expected exit message, got %q", out.Content)
	}
}

func TestExitPlanMode_WithPrompts(t *testing.T) {
	tool := &ExitPlanModeTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"allowedPrompts": []any{
			map[string]any{"tool": "Bash", "prompt": "run tests"},
			map[string]any{"tool": "Bash", "prompt": "install dependencies"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "run tests") {
		t.Errorf("expected 'run tests' in output, got %q", out.Content)
	}
	if !strings.Contains(out.Content, "install dependencies") {
		t.Errorf("expected 'install dependencies' in output, got %q", out.Content)
	}
}

func TestExitPlanMode_InvalidTool(t *testing.T) {
	tool := &ExitPlanModeTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"allowedPrompts": []any{
			map[string]any{"tool": "NotBash", "prompt": "something"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for invalid tool")
	}
	if !strings.Contains(out.Content, "must be \"Bash\"") {
		t.Errorf("expected tool validation error, got %q", out.Content)
	}
}

func TestExitPlanMode_EmptyPrompt(t *testing.T) {
	tool := &ExitPlanModeTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"allowedPrompts": []any{
			map[string]any{"tool": "Bash", "prompt": ""},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for empty prompt")
	}
}
