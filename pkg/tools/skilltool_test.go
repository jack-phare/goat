package tools

import (
	"context"
	"fmt"
	"testing"
)

// mockSkillProvider implements SkillProvider for tests.
type mockSkillProvider struct {
	skills map[string]SkillInfo
}

func (m *mockSkillProvider) GetSkillInfo(name string) (SkillInfo, bool) {
	info, ok := m.skills[name]
	return info, ok
}

func TestSkillTool_Name(t *testing.T) {
	tool := &SkillTool{}
	if got := tool.Name(); got != "Skill" {
		t.Errorf("Name() = %q, want %q", got, "Skill")
	}
}

func TestSkillTool_InputSchema(t *testing.T) {
	tool := &SkillTool{}
	schema := tool.InputSchema()

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map")
	}
	if _, ok := props["skill"]; !ok {
		t.Error("missing 'skill' property")
	}
	if _, ok := props["args"]; !ok {
		t.Error("missing 'args' property")
	}

	required, ok := schema["required"].([]string)
	if !ok || len(required) != 1 || required[0] != "skill" {
		t.Errorf("required = %v, want [\"skill\"]", required)
	}
}

func TestSkillTool_SideEffect(t *testing.T) {
	tool := &SkillTool{}
	if got := tool.SideEffect(); got != SideEffectNone {
		t.Errorf("SideEffect() = %v, want SideEffectNone", got)
	}
}

func TestSkillTool_InvokeExisting(t *testing.T) {
	provider := &mockSkillProvider{
		skills: map[string]SkillInfo{
			"commit": {
				Name:        "commit",
				Description: "Create a git commit",
				Body:        "# Commit Skill\n\nCreate a well-formatted commit.",
			},
		},
	}
	tool := &SkillTool{Skills: provider}

	out, err := tool.Execute(context.Background(), map[string]any{
		"skill": "commit",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.IsError {
		t.Errorf("unexpected error output: %s", out.Content)
	}
	if out.Content != "# Commit Skill\n\nCreate a well-formatted commit." {
		t.Errorf("Content = %q", out.Content)
	}
}

func TestSkillTool_InvokeUnknown(t *testing.T) {
	provider := &mockSkillProvider{skills: map[string]SkillInfo{}}
	tool := &SkillTool{Skills: provider}

	out, err := tool.Execute(context.Background(), map[string]any{
		"skill": "nonexistent",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.IsError {
		t.Error("expected error output for unknown skill")
	}
}

func TestSkillTool_NilProvider(t *testing.T) {
	tool := &SkillTool{}

	out, err := tool.Execute(context.Background(), map[string]any{
		"skill": "anything",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.IsError {
		t.Error("expected error output for nil provider")
	}
}

func TestSkillTool_MissingSkillName(t *testing.T) {
	tool := &SkillTool{}

	out, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.IsError {
		t.Error("expected error for missing skill name")
	}
}

func TestSkillTool_EmptySkillName(t *testing.T) {
	tool := &SkillTool{}

	out, err := tool.Execute(context.Background(), map[string]any{
		"skill": "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.IsError {
		t.Error("expected error for empty skill name")
	}
}

func TestSkillTool_WithArgs(t *testing.T) {
	provider := &mockSkillProvider{
		skills: map[string]SkillInfo{
			"deploy": {
				Name:      "deploy",
				Body:      "Deploy to $environment",
				Arguments: []string{"environment"},
			},
		},
	}

	substituter := func(body string, argDefs []string, argsStr string) (string, error) {
		// Simple mock substitution
		return "Deploy to production", nil
	}

	tool := &SkillTool{Skills: provider, ArgSubstituter: substituter}

	out, err := tool.Execute(context.Background(), map[string]any{
		"skill": "deploy",
		"args":  "production",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Content != "Deploy to production" {
		t.Errorf("Content = %q", out.Content)
	}
}

func TestSkillTool_ArgSubstituterError(t *testing.T) {
	provider := &mockSkillProvider{
		skills: map[string]SkillInfo{
			"deploy": {
				Name:      "deploy",
				Body:      "Deploy to $environment",
				Arguments: []string{"environment"},
			},
		},
	}

	substituter := func(body string, argDefs []string, argsStr string) (string, error) {
		return "", fmt.Errorf("bad arg")
	}

	tool := &SkillTool{Skills: provider, ArgSubstituter: substituter}

	out, err := tool.Execute(context.Background(), map[string]any{
		"skill": "deploy",
		"args":  "production",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.IsError {
		t.Error("expected error for substitution failure")
	}
}

func TestSkillTool_ForkExecution(t *testing.T) {
	provider := &mockSkillProvider{
		skills: map[string]SkillInfo{
			"forked": {
				Name:    "forked",
				Body:    "Do the forked thing.",
				Context: "fork",
			},
		},
	}

	spawner := &mockSubagentSpawner{
		result: AgentResult{
			AgentID: "agent-123",
			Output:  "Forked skill completed successfully.",
		},
	}

	tool := &SkillTool{Skills: provider, Spawner: spawner}

	out, err := tool.Execute(context.Background(), map[string]any{
		"skill": "forked",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Content != "Forked skill completed successfully." {
		t.Errorf("Content = %q", out.Content)
	}
}

func TestSkillTool_ForkWithNoSpawner(t *testing.T) {
	// When context is "fork" but no spawner, fall back to inline
	provider := &mockSkillProvider{
		skills: map[string]SkillInfo{
			"forked": {
				Name:    "forked",
				Body:    "Inline fallback body.",
				Context: "fork",
			},
		},
	}

	tool := &SkillTool{Skills: provider}

	out, err := tool.Execute(context.Background(), map[string]any{
		"skill": "forked",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Content != "Inline fallback body." {
		t.Errorf("Content = %q, expected inline fallback", out.Content)
	}
}

func TestSkillTool_InlineExecution(t *testing.T) {
	provider := &mockSkillProvider{
		skills: map[string]SkillInfo{
			"inline": {
				Name:    "inline",
				Body:    "Do the inline thing.",
				Context: "inline",
			},
		},
	}

	tool := &SkillTool{Skills: provider}

	out, err := tool.Execute(context.Background(), map[string]any{
		"skill": "inline",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Content != "Do the inline thing." {
		t.Errorf("Content = %q", out.Content)
	}
}

// mockSubagentSpawner for fork tests.
type mockSubagentSpawner struct {
	result AgentResult
	err    error
}

func (m *mockSubagentSpawner) Spawn(_ context.Context, _ AgentInput) (AgentResult, error) {
	return m.result, m.err
}
