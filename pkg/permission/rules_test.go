package permission

import (
	"context"
	"testing"

	"github.com/jg-phare/goat/pkg/types"
)

func TestRule_ExactToolNameMatch(t *testing.T) {
	rule := PermissionRule{ToolName: "Bash", Behavior: BehaviorAllow}
	if !rule.Matches("Bash", nil) {
		t.Error("expected Bash to match")
	}
	if rule.Matches("Read", nil) {
		t.Error("expected Read not to match")
	}
}

func TestRule_EmptyRuleContentMatchesAll(t *testing.T) {
	rule := PermissionRule{ToolName: "Bash", RuleContent: "", Behavior: BehaviorAllow}
	if !rule.Matches("Bash", map[string]any{"command": "ls"}) {
		t.Error("empty rule content should match all invocations")
	}
	if !rule.Matches("Bash", nil) {
		t.Error("empty rule content should match nil input")
	}
}

func TestRule_SubstringMatch_BashCommand(t *testing.T) {
	rule := PermissionRule{ToolName: "Bash", RuleContent: "npm test", Behavior: BehaviorAllow}

	if !rule.Matches("Bash", map[string]any{"command": "npm test -- --watch"}) {
		t.Error("expected substring match on Bash command")
	}
	if rule.Matches("Bash", map[string]any{"command": "go test ./..."}) {
		t.Error("expected no match for different command")
	}
}

func TestRule_SubstringMatch_CaseInsensitive(t *testing.T) {
	rule := PermissionRule{ToolName: "Bash", RuleContent: "NPM TEST", Behavior: BehaviorAllow}

	if !rule.Matches("Bash", map[string]any{"command": "npm test"}) {
		t.Error("expected case-insensitive match")
	}
}

func TestRule_GlobMatch_FilePath(t *testing.T) {
	rule := PermissionRule{ToolName: "Write", RuleContent: "/src/**", Behavior: BehaviorAllow}

	if !rule.Matches("Write", map[string]any{"file_path": "/src/main.go"}) {
		t.Error("expected glob match on /src/**")
	}
	if !rule.Matches("Write", map[string]any{"file_path": "/src/pkg/foo.go"}) {
		t.Error("expected glob match on nested path")
	}
	if rule.Matches("Write", map[string]any{"file_path": "/tmp/out.go"}) {
		t.Error("expected no match for /tmp path")
	}
}

func TestRule_GlobMatch_SingleStar(t *testing.T) {
	rule := PermissionRule{ToolName: "Edit", RuleContent: "*.go", Behavior: BehaviorAllow}

	if !rule.Matches("Edit", map[string]any{"file_path": "main.go"}) {
		t.Error("expected *.go to match main.go")
	}
}

func TestRule_NoMatch_DifferentToolName(t *testing.T) {
	rule := PermissionRule{ToolName: "Bash", RuleContent: "ls", Behavior: BehaviorAllow}
	if rule.Matches("Read", map[string]any{"command": "ls"}) {
		t.Error("should not match different tool name")
	}
}

func TestRule_EmptyInput_WithContent(t *testing.T) {
	rule := PermissionRule{ToolName: "Bash", RuleContent: "ls", Behavior: BehaviorAllow}
	if rule.Matches("Bash", nil) {
		t.Error("non-empty ruleContent should not match nil input")
	}
	if rule.Matches("Bash", map[string]any{}) {
		t.Error("non-empty ruleContent should not match empty input")
	}
}

func TestRule_MissingField(t *testing.T) {
	rule := PermissionRule{ToolName: "Bash", RuleContent: "ls", Behavior: BehaviorAllow}
	// Input has a field but not "command"
	if rule.Matches("Bash", map[string]any{"other": "ls"}) {
		t.Error("should not match when 'command' field is missing")
	}
}

func TestRule_NonStringField(t *testing.T) {
	rule := PermissionRule{ToolName: "Bash", RuleContent: "ls", Behavior: BehaviorAllow}
	if rule.Matches("Bash", map[string]any{"command": 42}) {
		t.Error("should not match non-string field")
	}
}

func TestRule_GenericTool_AnyStringField(t *testing.T) {
	rule := PermissionRule{ToolName: "CustomTool", RuleContent: "pattern", Behavior: BehaviorAllow}
	if !rule.Matches("CustomTool", map[string]any{"query": "some pattern here"}) {
		t.Error("generic tool should match any string field")
	}
	if rule.Matches("CustomTool", map[string]any{"query": "something else"}) {
		t.Error("generic tool should not match non-matching field")
	}
}

func TestRule_GlobAndGrep_PathAndPattern(t *testing.T) {
	rule := PermissionRule{ToolName: "Glob", RuleContent: "/src/**", Behavior: BehaviorAllow}
	if !rule.Matches("Glob", map[string]any{"path": "/src/pkg"}) {
		t.Error("Glob should match on path field")
	}

	rule2 := PermissionRule{ToolName: "Grep", RuleContent: "TODO", Behavior: BehaviorAllow}
	if !rule2.Matches("Grep", map[string]any{"pattern": "TODO"}) {
		t.Error("Grep should match on pattern field")
	}
}

// --- ApplyUpdate tests ---

func TestApplyUpdate_AddRules_Session(t *testing.T) {
	c := NewChecker(CheckerConfig{Mode: string(types.PermissionModeDefault)})

	mode := types.PermissionModeDefault
	_ = mode // unused, just for type reference

	err := c.ApplyUpdate(types.PermissionUpdate{
		Type:        "addRules",
		Destination: "session",
		Rule: &types.PermissionRuleValue{
			ToolName:    "Bash",
			RuleContent: "npm test",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rules := c.SessionRules()
	if len(rules) != 1 {
		t.Fatalf("expected 1 session rule, got %d", len(rules))
	}
	if rules[0].ToolName != "Bash" || rules[0].RuleContent != "npm test" {
		t.Errorf("rule = %+v, expected Bash/npm test", rules[0])
	}
}

func TestApplyUpdate_RemoveRules(t *testing.T) {
	c := NewChecker(CheckerConfig{Mode: string(types.PermissionModeDefault)})

	// Add two rules
	c.ApplyUpdate(types.PermissionUpdate{
		Type: "addRules", Destination: "session",
		Rule: &types.PermissionRuleValue{ToolName: "Bash", RuleContent: "npm test"},
	})
	c.ApplyUpdate(types.PermissionUpdate{
		Type: "addRules", Destination: "session",
		Rule: &types.PermissionRuleValue{ToolName: "Bash", RuleContent: "go test"},
	})

	if len(c.SessionRules()) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(c.SessionRules()))
	}

	// Remove the first
	c.ApplyUpdate(types.PermissionUpdate{
		Type: "removeRules", Destination: "session",
		Rule: &types.PermissionRuleValue{ToolName: "Bash", RuleContent: "npm test"},
	})

	rules := c.SessionRules()
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule after removal, got %d", len(rules))
	}
	if rules[0].RuleContent != "go test" {
		t.Errorf("remaining rule = %q, expected 'go test'", rules[0].RuleContent)
	}
}

func TestApplyUpdate_ReplaceRules(t *testing.T) {
	c := NewChecker(CheckerConfig{Mode: string(types.PermissionModeDefault)})

	// Add two Bash rules
	c.ApplyUpdate(types.PermissionUpdate{
		Type: "addRules", Destination: "session",
		Rule: &types.PermissionRuleValue{ToolName: "Bash", RuleContent: "npm test"},
	})
	c.ApplyUpdate(types.PermissionUpdate{
		Type: "addRules", Destination: "session",
		Rule: &types.PermissionRuleValue{ToolName: "Bash", RuleContent: "go test"},
	})

	// Replace all Bash rules with a single new one
	c.ApplyUpdate(types.PermissionUpdate{
		Type: "replaceRules", Destination: "session",
		Rule: &types.PermissionRuleValue{ToolName: "Bash", RuleContent: "make build"},
	})

	rules := c.SessionRules()
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule after replace, got %d", len(rules))
	}
	if rules[0].RuleContent != "make build" {
		t.Errorf("rule = %q, expected 'make build'", rules[0].RuleContent)
	}
}

func TestApplyUpdate_SetMode(t *testing.T) {
	c := NewChecker(CheckerConfig{Mode: string(types.PermissionModeDefault)})

	planMode := types.PermissionModePlan
	err := c.ApplyUpdate(types.PermissionUpdate{
		Type: "setMode",
		Mode: &planMode,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Mode() != types.PermissionModePlan {
		t.Errorf("mode = %q, want plan", c.Mode())
	}
}

func TestApplyUpdate_UnknownType(t *testing.T) {
	c := NewChecker(CheckerConfig{Mode: string(types.PermissionModeDefault)})
	err := c.ApplyUpdate(types.PermissionUpdate{Type: "unknownType"})
	if err == nil {
		t.Fatal("expected error for unknown update type")
	}
}

func TestApplyUpdate_DirectoryTypes_NoOp(t *testing.T) {
	c := NewChecker(CheckerConfig{Mode: string(types.PermissionModeDefault)})

	for _, typ := range []string{"addDirectories", "removeDirectories"} {
		err := c.ApplyUpdate(types.PermissionUpdate{
			Type:        typ,
			Directories: []string{"/src"},
		})
		if err != nil {
			t.Errorf("expected no error for %s, got %v", typ, err)
		}
	}
}

func TestSessionRules_PersistAcrossChecks(t *testing.T) {
	c := NewChecker(CheckerConfig{Mode: string(types.PermissionModeDefault)})

	// Add a session rule allowing Bash with "npm test"
	c.ApplyUpdate(types.PermissionUpdate{
		Type: "addRules", Destination: "session",
		Rule: &types.PermissionRuleValue{ToolName: "Bash", RuleContent: "npm test"},
	})

	// First check: should allow
	result, _ := c.Check(context.Background(), "Bash", map[string]any{"command": "npm test"})
	if result.Behavior != "allow" {
		t.Errorf("first check: behavior = %q, want allow", result.Behavior)
	}

	// Second check with same input: should still allow (rule persists)
	result, _ = c.Check(context.Background(), "Bash", map[string]any{"command": "npm test --watch"})
	if result.Behavior != "allow" {
		t.Errorf("second check: behavior = %q, want allow", result.Behavior)
	}

	// Check with different command: should fall to mode default → denied (headless)
	result, _ = c.Check(context.Background(), "Bash", map[string]any{"command": "rm -rf /"})
	if result.Behavior != "deny" {
		t.Errorf("unmatched check: behavior = %q, want deny (headless)", result.Behavior)
	}
}

func TestConfigRules_TakePriorityOverSessionRules(t *testing.T) {
	c := NewChecker(CheckerConfig{
		Mode: string(types.PermissionModeDefault),
		Rules: []PermissionRule{
			{ToolName: "Bash", RuleContent: "", Behavior: BehaviorDeny}, // deny all Bash
		},
	})

	// Add a session rule allowing Bash
	c.ApplyUpdate(types.PermissionUpdate{
		Type: "addRules", Destination: "session",
		Rule: &types.PermissionRuleValue{ToolName: "Bash", RuleContent: ""},
	})

	// Config rule (deny) should take priority over session rule (allow)
	result, _ := c.Check(context.Background(), "Bash", map[string]any{"command": "ls"})
	if result.Behavior != "deny" {
		t.Errorf("behavior = %q, want deny (config rule priority)", result.Behavior)
	}
}

func TestApplyUpdate_NilRule(t *testing.T) {
	c := NewChecker(CheckerConfig{Mode: string(types.PermissionModeDefault)})

	// Should not panic
	err := c.ApplyUpdate(types.PermissionUpdate{Type: "addRules", Destination: "session", Rule: nil})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(c.SessionRules()) != 0 {
		t.Error("expected 0 session rules with nil rule")
	}
}
