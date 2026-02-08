package tools

import (
	"context"
	"strings"
	"testing"
)

type mockSpawner struct {
	result AgentResult
	err    error
	input  AgentInput // captured
}

func (m *mockSpawner) Spawn(_ context.Context, input AgentInput) (AgentResult, error) {
	m.input = input
	return m.result, m.err
}

func TestAgent_WithMockSpawner(t *testing.T) {
	spawner := &mockSpawner{
		result: AgentResult{AgentID: "agent-123", Output: "Task completed successfully"},
	}
	tool := &AgentTool{Spawner: spawner}

	out, err := tool.Execute(context.Background(), map[string]any{
		"description":   "test task",
		"prompt":        "do something",
		"subagent_type": "general",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "Task completed successfully") {
		t.Errorf("expected output, got %q", out.Content)
	}
	if !strings.Contains(out.Content, "agent-123") {
		t.Errorf("expected agent ID, got %q", out.Content)
	}
}

func TestAgent_StubSpawner(t *testing.T) {
	tool := &AgentTool{} // no spawner
	out, err := tool.Execute(context.Background(), map[string]any{
		"description":   "test",
		"prompt":        "test",
		"subagent_type": "general",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error from stub spawner")
	}
	if !strings.Contains(out.Content, "not yet configured") {
		t.Errorf("expected 'not yet configured' message, got %q", out.Content)
	}
}

func TestAgent_BackgroundMode(t *testing.T) {
	spawner := &mockSpawner{
		result: AgentResult{AgentID: "bg-agent-1", Output: ""},
	}
	tool := &AgentTool{Spawner: spawner}

	out, err := tool.Execute(context.Background(), map[string]any{
		"description":       "bg task",
		"prompt":            "do stuff",
		"subagent_type":     "general",
		"run_in_background": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "background") {
		t.Errorf("expected background message, got %q", out.Content)
	}
	if spawner.input.RunInBackground == nil || !*spawner.input.RunInBackground {
		t.Error("expected RunInBackground to be true")
	}
}

func TestAgent_MissingRequired(t *testing.T) {
	tool := &AgentTool{Spawner: &mockSpawner{}}

	tests := []struct {
		name  string
		input map[string]any
	}{
		{"missing description", map[string]any{"prompt": "p", "subagent_type": "g"}},
		{"missing prompt", map[string]any{"description": "d", "subagent_type": "g"}},
		{"missing subagent_type", map[string]any{"description": "d", "prompt": "p"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := tool.Execute(context.Background(), tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if !out.IsError {
				t.Error("expected error for missing required field")
			}
		})
	}
}

func TestAgent_OptionalFields(t *testing.T) {
	spawner := &mockSpawner{
		result: AgentResult{AgentID: "a1", Output: "ok"},
	}
	tool := &AgentTool{Spawner: spawner}

	tool.Execute(context.Background(), map[string]any{
		"description":   "test",
		"prompt":        "test",
		"subagent_type": "general",
		"model":         "sonnet",
		"resume":        "prev-id",
		"max_turns":     float64(10),
	})

	if spawner.input.Model == nil || *spawner.input.Model != "sonnet" {
		t.Error("expected model to be 'sonnet'")
	}
	if spawner.input.Resume == nil || *spawner.input.Resume != "prev-id" {
		t.Error("expected resume to be 'prev-id'")
	}
	if spawner.input.MaxTurns == nil || *spawner.input.MaxTurns != 10 {
		t.Error("expected max_turns to be 10")
	}
}

func TestAgent_NameAndModeFields(t *testing.T) {
	spawner := &mockSpawner{
		result: AgentResult{AgentID: "a1", Output: "ok"},
	}
	tool := &AgentTool{Spawner: spawner}

	tool.Execute(context.Background(), map[string]any{
		"description":   "test",
		"prompt":        "test",
		"subagent_type": "general",
		"name":          "my-agent",
		"mode":          "bypassPermissions",
	})

	if spawner.input.Name == nil || *spawner.input.Name != "my-agent" {
		t.Error("expected name to be 'my-agent'")
	}
	if spawner.input.Mode == nil || *spawner.input.Mode != "bypassPermissions" {
		t.Error("expected mode to be 'bypassPermissions'")
	}
}

func TestAgent_InputSchema_HasNameAndMode(t *testing.T) {
	tool := &AgentTool{}
	schema := tool.InputSchema()
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties in schema")
	}
	if _, ok := props["name"]; !ok {
		t.Error("expected 'name' in schema properties")
	}
	if _, ok := props["mode"]; !ok {
		t.Error("expected 'mode' in schema properties")
	}
}
