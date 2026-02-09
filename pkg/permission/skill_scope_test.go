package permission

import (
	"context"
	"testing"

	"github.com/jg-phare/goat/pkg/agent"
)

// denyAllChecker always denies.
type denyAllChecker struct{}

func (d *denyAllChecker) Check(_ context.Context, _ string, _ map[string]any) (agent.PermissionResult, error) {
	return agent.PermissionResult{Behavior: "deny", Message: "denied by inner"}, nil
}

func TestSkillPermissionScope_AllowedToolPasses(t *testing.T) {
	scope := &SkillPermissionScope{
		AllowedTools: []string{"Bash", "Read"},
		Inner:        &denyAllChecker{},
	}

	result, err := scope.Check(context.Background(), "Bash", map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Behavior != "allow" {
		t.Errorf("Behavior = %q, want %q", result.Behavior, "allow")
	}
}

func TestSkillPermissionScope_DisallowedDelegates(t *testing.T) {
	scope := &SkillPermissionScope{
		AllowedTools: []string{"Bash"},
		Inner:        &denyAllChecker{},
	}

	result, err := scope.Check(context.Background(), "Write", map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Behavior != "deny" {
		t.Errorf("Behavior = %q, want %q", result.Behavior, "deny")
	}
}

func TestSkillPermissionScope_GlobPattern(t *testing.T) {
	scope := &SkillPermissionScope{
		AllowedTools: []string{"mcp__*"},
		Inner:        &denyAllChecker{},
	}

	result, err := scope.Check(context.Background(), "mcp__server_tool", map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Behavior != "allow" {
		t.Errorf("Behavior = %q, want %q for glob match", result.Behavior, "allow")
	}
}

func TestSkillPermissionScope_GlobNoMatch(t *testing.T) {
	scope := &SkillPermissionScope{
		AllowedTools: []string{"mcp__*"},
		Inner:        &denyAllChecker{},
	}

	result, err := scope.Check(context.Background(), "Bash", map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Behavior != "deny" {
		t.Errorf("Behavior = %q, want %q", result.Behavior, "deny")
	}
}

func TestSkillPermissionScope_BashConstraint(t *testing.T) {
	scope := &SkillPermissionScope{
		AllowedTools: []string{"Bash(gh:*)"},
		Inner:        &denyAllChecker{},
	}

	// gh command should be allowed
	result, err := scope.Check(context.Background(), "Bash", map[string]any{
		"command": "gh pr list",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Behavior != "allow" {
		t.Errorf("Behavior = %q, want %q for gh command", result.Behavior, "allow")
	}

	// Non-gh command should be denied
	result, err = scope.Check(context.Background(), "Bash", map[string]any{
		"command": "rm -rf /",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Behavior != "deny" {
		t.Errorf("Behavior = %q, want %q for non-gh command", result.Behavior, "deny")
	}
}

func TestSkillPermissionScope_BashConstraintExact(t *testing.T) {
	scope := &SkillPermissionScope{
		AllowedTools: []string{"Bash(gh:*)"},
		Inner:        &denyAllChecker{},
	}

	// Just "gh" with no subcommand should match
	result, err := scope.Check(context.Background(), "Bash", map[string]any{
		"command": "gh",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Behavior != "allow" {
		t.Errorf("Behavior = %q, want %q for exact gh command", result.Behavior, "allow")
	}
}

func TestSkillPermissionScope_EmptyAllowedTools(t *testing.T) {
	scope := &SkillPermissionScope{
		AllowedTools: nil,
		Inner:        &denyAllChecker{},
	}

	result, err := scope.Check(context.Background(), "Bash", map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Behavior != "deny" {
		t.Errorf("Behavior = %q, want %q with no allowed tools", result.Behavior, "deny")
	}
}

func TestMatchToolPattern_ExactMatch(t *testing.T) {
	if !matchToolPattern("Bash", "Bash", nil) {
		t.Error("expected exact match")
	}
}

func TestMatchToolPattern_GlobMatch(t *testing.T) {
	if !matchToolPattern("mcp__*", "mcp__server_tool", nil) {
		t.Error("expected glob match")
	}
}

func TestMatchToolPattern_ConstraintMatch(t *testing.T) {
	if !matchToolPattern("Bash(npm:*)", "Bash", map[string]any{"command": "npm install"}) {
		t.Error("expected constraint match")
	}
}

func TestMatchToolPattern_ConstraintNoMatch(t *testing.T) {
	if matchToolPattern("Bash(npm:*)", "Bash", map[string]any{"command": "pip install"}) {
		t.Error("expected constraint no match")
	}
}
