package permission

import (
	"context"
	"testing"

	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/types"
)

func TestChecker_BypassPermissions_WithSafetyFlag(t *testing.T) {
	c := NewChecker(CheckerConfig{
		Mode:                            string(types.PermissionModeBypassPermissions),
		AllowDangerouslySkipPermissions: true,
	})

	result, err := c.Check(context.Background(), "Bash", map[string]any{"command": "rm -rf /"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Behavior != "allow" {
		t.Errorf("behavior = %q, want allow", result.Behavior)
	}
}

func TestChecker_BypassPermissions_WithoutSafetyFlag(t *testing.T) {
	c := NewChecker(CheckerConfig{
		Mode:                            string(types.PermissionModeBypassPermissions),
		AllowDangerouslySkipPermissions: false,
	})

	_, err := c.Check(context.Background(), "Bash", nil)
	if err == nil {
		t.Fatal("expected error when bypassPermissions used without safety flag")
	}
}

func TestChecker_PlanMode_DeniesAll(t *testing.T) {
	c := NewChecker(CheckerConfig{Mode: string(types.PermissionModePlan)})

	tools := []string{"Bash", "Read", "Write", "Glob", "Agent"}
	for _, tool := range tools {
		result, err := c.Check(context.Background(), tool, nil)
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", tool, err)
		}
		if result.Behavior != "deny" {
			t.Errorf("plan mode: %s behavior = %q, want deny", tool, result.Behavior)
		}
	}
}

func TestChecker_DelegateMode_AllowsOnlyAgent(t *testing.T) {
	c := NewChecker(CheckerConfig{Mode: string(types.PermissionModeDelegate)})

	// Agent should be allowed
	result, err := c.Check(context.Background(), "Agent", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Behavior != "allow" {
		t.Errorf("Agent behavior = %q, want allow", result.Behavior)
	}

	// Everything else should be denied
	denied := []string{"Bash", "Read", "Write", "Glob", "Grep"}
	for _, tool := range denied {
		result, err := c.Check(context.Background(), tool, nil)
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", tool, err)
		}
		if result.Behavior != "deny" {
			t.Errorf("delegate mode: %s behavior = %q, want deny", tool, result.Behavior)
		}
	}
}

func TestChecker_DefaultMode_ReadToolsAutoAllowed(t *testing.T) {
	c := NewChecker(CheckerConfig{Mode: string(types.PermissionModeDefault)})

	readTools := []string{"Read", "Glob", "Grep", "TodoWrite"}
	for _, tool := range readTools {
		result, err := c.Check(context.Background(), tool, nil)
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", tool, err)
		}
		if result.Behavior != "allow" {
			t.Errorf("default mode: %s behavior = %q, want allow", tool, result.Behavior)
		}
	}
}

func TestChecker_DefaultMode_WriteToolsDenyHeadless(t *testing.T) {
	// Without a prompter, "ask" tools are denied (headless mode)
	c := NewChecker(CheckerConfig{Mode: string(types.PermissionModeDefault)})

	writeTools := []string{"Write", "Edit", "NotebookEdit"}
	for _, tool := range writeTools {
		result, err := c.Check(context.Background(), tool, nil)
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", tool, err)
		}
		if result.Behavior != "deny" {
			t.Errorf("default mode headless: %s behavior = %q, want deny", tool, result.Behavior)
		}
	}
}

func TestChecker_DefaultMode_BashDeniedHeadless(t *testing.T) {
	// Without a prompter, Bash is denied (headless mode)
	c := NewChecker(CheckerConfig{Mode: string(types.PermissionModeDefault)})

	result, err := c.Check(context.Background(), "Bash", map[string]any{"command": "ls"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Behavior != "deny" {
		t.Errorf("Bash behavior = %q, want deny (headless)", result.Behavior)
	}
}

func TestChecker_AcceptEditsMode_WritesAutoAllowed(t *testing.T) {
	c := NewChecker(CheckerConfig{Mode: string(types.PermissionModeAcceptEdits)})

	// Read tools allowed
	for _, tool := range []string{"Read", "Glob", "Grep"} {
		result, _ := c.Check(context.Background(), tool, nil)
		if result.Behavior != "allow" {
			t.Errorf("acceptEdits mode: %s behavior = %q, want allow", tool, result.Behavior)
		}
	}

	// Write tools auto-allowed
	for _, tool := range []string{"Write", "Edit", "NotebookEdit"} {
		result, _ := c.Check(context.Background(), tool, nil)
		if result.Behavior != "allow" {
			t.Errorf("acceptEdits mode: %s behavior = %q, want allow", tool, result.Behavior)
		}
	}

	// Bash denied in headless (no prompter)
	result, _ := c.Check(context.Background(), "Bash", nil)
	if result.Behavior != "deny" {
		t.Errorf("acceptEdits mode headless: Bash behavior = %q, want deny", result.Behavior)
	}
}

func TestChecker_DontAskMode_DeniesUnlessPreApproved(t *testing.T) {
	c := NewChecker(CheckerConfig{Mode: string(types.PermissionModeDontAsk)})

	// Read tools (RiskNone/Low) are allowed
	for _, tool := range []string{"Read", "Glob", "Grep"} {
		result, _ := c.Check(context.Background(), tool, nil)
		if result.Behavior != "allow" {
			t.Errorf("dontAsk mode: %s behavior = %q, want allow", tool, result.Behavior)
		}
	}

	// Write/Bash/Network denied
	for _, tool := range []string{"Write", "Bash", "WebFetch"} {
		result, _ := c.Check(context.Background(), tool, nil)
		if result.Behavior != "deny" {
			t.Errorf("dontAsk mode: %s behavior = %q, want deny", tool, result.Behavior)
		}
	}
}

func TestChecker_DisabledTools_AlwaysDenied(t *testing.T) {
	c := NewChecker(CheckerConfig{
		Mode:          string(types.PermissionModeDefault),
		DisabledTools: []string{"Bash", "Read"},
	})

	for _, tool := range []string{"Bash", "Read"} {
		result, err := c.Check(context.Background(), tool, nil)
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", tool, err)
		}
		if result.Behavior != "deny" {
			t.Errorf("disabled %s behavior = %q, want deny", tool, result.Behavior)
		}
		if result.Message != "tool is disabled" {
			t.Errorf("disabled %s message = %q, want 'tool is disabled'", tool, result.Message)
		}
	}
}

func TestChecker_AllowedTools_AlwaysAllowed(t *testing.T) {
	c := NewChecker(CheckerConfig{
		Mode:         string(types.PermissionModeDefault),
		AllowedTools: []string{"Bash", "WebFetch"},
	})

	for _, tool := range []string{"Bash", "WebFetch"} {
		result, err := c.Check(context.Background(), tool, nil)
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", tool, err)
		}
		if result.Behavior != "allow" {
			t.Errorf("allowed %s behavior = %q, want allow", tool, result.Behavior)
		}
	}
}

func TestChecker_AllowedTools_NotInPlanMode(t *testing.T) {
	// Plan mode overrides allowed tools
	c := NewChecker(CheckerConfig{
		Mode:         string(types.PermissionModePlan),
		AllowedTools: []string{"Bash"},
	})

	result, _ := c.Check(context.Background(), "Bash", nil)
	if result.Behavior != "deny" {
		t.Errorf("plan mode with allowed Bash: behavior = %q, want deny", result.Behavior)
	}
}

func TestChecker_AllowedTools_NotInDelegateMode(t *testing.T) {
	// Delegate mode overrides allowed tools (except Agent)
	c := NewChecker(CheckerConfig{
		Mode:         string(types.PermissionModeDelegate),
		AllowedTools: []string{"Bash"},
	})

	result, _ := c.Check(context.Background(), "Bash", nil)
	if result.Behavior != "deny" {
		t.Errorf("delegate mode with allowed Bash: behavior = %q, want deny", result.Behavior)
	}
}

func TestChecker_DisabledOverridesAllowed(t *testing.T) {
	// If a tool is both allowed and disabled, disabled wins (disabled checked first)
	c := NewChecker(CheckerConfig{
		Mode:          string(types.PermissionModeDefault),
		AllowedTools:  []string{"Bash"},
		DisabledTools: []string{"Bash"},
	})

	result, _ := c.Check(context.Background(), "Bash", nil)
	if result.Behavior != "deny" {
		t.Errorf("disabled+allowed Bash: behavior = %q, want deny", result.Behavior)
	}
}

func TestChecker_SetMode(t *testing.T) {
	c := NewChecker(CheckerConfig{Mode: string(types.PermissionModeDefault)})

	if c.Mode() != types.PermissionModeDefault {
		t.Errorf("initial mode = %q, want default", c.Mode())
	}

	c.SetMode(types.PermissionModePlan)
	if c.Mode() != types.PermissionModePlan {
		t.Errorf("after SetMode: mode = %q, want plan", c.Mode())
	}
}

func TestChecker_DefaultMode_EmptyString(t *testing.T) {
	// Empty mode string defaults to "default"
	c := NewChecker(CheckerConfig{Mode: ""})
	if c.Mode() != types.PermissionModeDefault {
		t.Errorf("mode = %q, want default", c.Mode())
	}
}

func TestChecker_MCP_ToolsDeniedHeadless(t *testing.T) {
	// MCP tools are high risk → "ask" → denied without prompter
	c := NewChecker(CheckerConfig{Mode: string(types.PermissionModeDefault)})

	result, _ := c.Check(context.Background(), "mcp__server__tool", nil)
	if result.Behavior != "deny" {
		t.Errorf("MCP tool behavior = %q, want deny (headless)", result.Behavior)
	}
}

func TestChecker_ConfigRules_Allow(t *testing.T) {
	c := NewChecker(CheckerConfig{
		Mode: string(types.PermissionModeDefault),
		Rules: []PermissionRule{
			{ToolName: "Bash", Behavior: BehaviorAllow},
		},
	})

	result, _ := c.Check(context.Background(), "Bash", nil)
	if result.Behavior != "allow" {
		t.Errorf("Bash with allow rule: behavior = %q, want allow", result.Behavior)
	}
}

func TestChecker_ConfigRules_Deny(t *testing.T) {
	c := NewChecker(CheckerConfig{
		Mode: string(types.PermissionModeDefault),
		Rules: []PermissionRule{
			{ToolName: "Read", Behavior: BehaviorDeny},
		},
	})

	result, _ := c.Check(context.Background(), "Read", nil)
	if result.Behavior != "deny" {
		t.Errorf("Read with deny rule: behavior = %q, want deny", result.Behavior)
	}
}

// --- Hook & Callback Tests (Phase 4) ---

// mockHookRunner returns configurable hook results.
type mockHookRunner struct {
	decision string // "allow", "deny", or "" (continue)
	message  string
}

func (m *mockHookRunner) Fire(_ context.Context, _ types.HookEvent, _ any) ([]agent.HookResult, error) {
	if m.decision == "" {
		return nil, nil // no decision
	}
	return []agent.HookResult{{Decision: m.decision, Message: m.message}}, nil
}

func TestChecker_HookAllow_ShortCircuitsCallback(t *testing.T) {
	callbackCalled := false
	c := NewChecker(CheckerConfig{
		Mode:       string(types.PermissionModeDefault),
		HookRunner: &mockHookRunner{decision: "allow"},
		CanUseTool: func(toolName string, input map[string]any) (*types.PermissionResult, error) {
			callbackCalled = true
			return &types.PermissionResult{Behavior: "deny"}, nil
		},
	})

	result, _ := c.Check(context.Background(), "Bash", nil)
	if result.Behavior != "allow" {
		t.Errorf("behavior = %q, want allow (from hook)", result.Behavior)
	}
	if callbackCalled {
		t.Error("callback should not have been called when hook allows")
	}
}

func TestChecker_HookDeny_ShortCircuitsCallback(t *testing.T) {
	callbackCalled := false
	c := NewChecker(CheckerConfig{
		Mode:       string(types.PermissionModeDefault),
		HookRunner: &mockHookRunner{decision: "deny", message: "hook says no"},
		CanUseTool: func(toolName string, input map[string]any) (*types.PermissionResult, error) {
			callbackCalled = true
			return &types.PermissionResult{Behavior: "allow"}, nil
		},
	})

	result, _ := c.Check(context.Background(), "Bash", nil)
	if result.Behavior != "deny" {
		t.Errorf("behavior = %q, want deny (from hook)", result.Behavior)
	}
	if result.Message != "hook says no" {
		t.Errorf("message = %q, want 'hook says no'", result.Message)
	}
	if callbackCalled {
		t.Error("callback should not have been called when hook denies")
	}
}

func TestChecker_HookContinue_FallsToCallback(t *testing.T) {
	c := NewChecker(CheckerConfig{
		Mode:       string(types.PermissionModeDefault),
		HookRunner: &mockHookRunner{decision: ""}, // continue
		CanUseTool: func(toolName string, input map[string]any) (*types.PermissionResult, error) {
			return &types.PermissionResult{Behavior: "allow"}, nil
		},
	})

	result, _ := c.Check(context.Background(), "Bash", nil)
	if result.Behavior != "allow" {
		t.Errorf("behavior = %q, want allow (from callback after hook continue)", result.Behavior)
	}
}

func TestChecker_CallbackAllow(t *testing.T) {
	c := NewChecker(CheckerConfig{
		Mode: string(types.PermissionModeDefault),
		CanUseTool: func(toolName string, input map[string]any) (*types.PermissionResult, error) {
			return &types.PermissionResult{
				Behavior:     "allow",
				UpdatedInput: map[string]any{"command": "safe-command"},
			}, nil
		},
	})

	result, _ := c.Check(context.Background(), "Bash", nil)
	if result.Behavior != "allow" {
		t.Errorf("behavior = %q, want allow", result.Behavior)
	}
	if result.UpdatedInput["command"] != "safe-command" {
		t.Errorf("updated input = %v, want command=safe-command", result.UpdatedInput)
	}
}

func TestChecker_CallbackDeny(t *testing.T) {
	c := NewChecker(CheckerConfig{
		Mode: string(types.PermissionModeDefault),
		CanUseTool: func(toolName string, input map[string]any) (*types.PermissionResult, error) {
			return &types.PermissionResult{Behavior: "deny", Message: "callback denied"}, nil
		},
	})

	result, _ := c.Check(context.Background(), "Bash", nil)
	if result.Behavior != "deny" {
		t.Errorf("behavior = %q, want deny", result.Behavior)
	}
}

func TestChecker_CallbackNil_FallsToDefault(t *testing.T) {
	c := NewChecker(CheckerConfig{
		Mode: string(types.PermissionModeDefault),
		CanUseTool: func(toolName string, input map[string]any) (*types.PermissionResult, error) {
			return nil, nil // no decision
		},
	})

	// Bash is high risk, mode default = ask → denied (headless)
	result, _ := c.Check(context.Background(), "Bash", nil)
	if result.Behavior != "deny" {
		t.Errorf("behavior = %q, want deny (headless fallback)", result.Behavior)
	}
}

func TestChecker_NoHookNoCallback_FallsToModeDefault(t *testing.T) {
	c := NewChecker(CheckerConfig{Mode: string(types.PermissionModeDefault)})

	// Read tools auto-allowed
	result, _ := c.Check(context.Background(), "Read", nil)
	if result.Behavior != "allow" {
		t.Errorf("Read behavior = %q, want allow", result.Behavior)
	}

	// Bash denied (headless)
	result, _ = c.Check(context.Background(), "Bash", nil)
	if result.Behavior != "deny" {
		t.Errorf("Bash behavior = %q, want deny (headless)", result.Behavior)
	}
}

func TestChecker_StubPrompter_DeniesAsk(t *testing.T) {
	c := NewChecker(CheckerConfig{
		Mode:         string(types.PermissionModeDefault),
		UserPrompter: &StubPrompter{},
	})

	result, _ := c.Check(context.Background(), "Bash", nil)
	if result.Behavior != "deny" {
		t.Errorf("behavior = %q, want deny (StubPrompter)", result.Behavior)
	}
}

// mockPrompter simulates an interactive user allowing the tool.
type mockPrompter struct {
	behavior string
}

func (m *mockPrompter) PromptForPermission(toolName string, input map[string]any, _ []types.PermissionUpdate) (agent.PermissionResult, error) {
	return agent.PermissionResult{Behavior: m.behavior}, nil
}

func TestChecker_UserPrompter_AllowsAskTools(t *testing.T) {
	c := NewChecker(CheckerConfig{
		Mode:         string(types.PermissionModeDefault),
		UserPrompter: &mockPrompter{behavior: "allow"},
	})

	result, _ := c.Check(context.Background(), "Bash", nil)
	if result.Behavior != "allow" {
		t.Errorf("behavior = %q, want allow (from prompter)", result.Behavior)
	}
}

func TestChecker_UserPrompter_DeniesAskTools(t *testing.T) {
	c := NewChecker(CheckerConfig{
		Mode:         string(types.PermissionModeDefault),
		UserPrompter: &mockPrompter{behavior: "deny"},
	})

	result, _ := c.Check(context.Background(), "Bash", nil)
	if result.Behavior != "deny" {
		t.Errorf("behavior = %q, want deny (from prompter)", result.Behavior)
	}
}

func TestChecker_FullCheckFlow_Integration(t *testing.T) {
	// Test the full check flow: mode → disabled → allowed → rules → hook → callback → default
	hookCalled := false
	callbackCalled := false

	c := NewChecker(CheckerConfig{
		Mode:          string(types.PermissionModeDefault),
		DisabledTools: []string{"Forbidden"},
		AllowedTools:  []string{"AlwaysOK"},
		Rules: []PermissionRule{
			{ToolName: "Bash", RuleContent: "safe-cmd", Behavior: BehaviorAllow},
		},
		HookRunner: &mockHookRunner{decision: ""}, // continue
		CanUseTool: func(toolName string, input map[string]any) (*types.PermissionResult, error) {
			callbackCalled = true
			return nil, nil // no decision
		},
	})
	_ = hookCalled

	// Disabled tool → deny at layer 2
	result, _ := c.Check(context.Background(), "Forbidden", nil)
	if result.Behavior != "deny" {
		t.Errorf("Forbidden: behavior = %q, want deny", result.Behavior)
	}

	// Allowed tool → allow at layer 3
	result, _ = c.Check(context.Background(), "AlwaysOK", nil)
	if result.Behavior != "allow" {
		t.Errorf("AlwaysOK: behavior = %q, want allow", result.Behavior)
	}

	// Rule match → allow at layer 4
	result, _ = c.Check(context.Background(), "Bash", map[string]any{"command": "safe-cmd"})
	if result.Behavior != "allow" {
		t.Errorf("Bash with safe-cmd: behavior = %q, want allow", result.Behavior)
	}

	// No rule match → falls through hook → callback → mode default (deny headless)
	callbackCalled = false
	result, _ = c.Check(context.Background(), "Bash", map[string]any{"command": "rm -rf /"})
	if result.Behavior != "deny" {
		t.Errorf("Bash unmatched: behavior = %q, want deny (headless)", result.Behavior)
	}
	if !callbackCalled {
		t.Error("expected callback to be called for unmatched Bash")
	}

	// Read tool → auto-allowed by mode default
	result, _ = c.Check(context.Background(), "Read", nil)
	if result.Behavior != "allow" {
		t.Errorf("Read: behavior = %q, want allow", result.Behavior)
	}
}
