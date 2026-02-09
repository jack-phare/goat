package agent

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jg-phare/goat/pkg/tools"
	"github.com/jg-phare/goat/pkg/types"
)

func TestRecordToolFileAccess_ReadTool(t *testing.T) {
	state := &LoopState{}
	recordToolFileAccess(state, "Read", map[string]any{
		"file_path": "/tmp/foo.go",
	})
	if !state.AccessedFiles["/tmp/foo.go"]["read"] {
		t.Error("expected read op for /tmp/foo.go")
	}
}

func TestRecordToolFileAccess_WriteTool(t *testing.T) {
	state := &LoopState{}
	recordToolFileAccess(state, "Write", map[string]any{
		"file_path": "/tmp/bar.go",
	})
	if !state.AccessedFiles["/tmp/bar.go"]["write"] {
		t.Error("expected write op for /tmp/bar.go")
	}
}

func TestRecordToolFileAccess_EditTool(t *testing.T) {
	state := &LoopState{}
	recordToolFileAccess(state, "Edit", map[string]any{
		"file_path": "/tmp/baz.go",
	})
	if !state.AccessedFiles["/tmp/baz.go"]["edit"] {
		t.Error("expected edit op for /tmp/baz.go")
	}
}

func TestRecordToolFileAccess_GlobTool(t *testing.T) {
	state := &LoopState{}
	recordToolFileAccess(state, "Glob", map[string]any{
		"path": "/tmp/search",
	})
	if !state.AccessedFiles["/tmp/search"]["glob"] {
		t.Error("expected glob op for /tmp/search")
	}
}

func TestRecordToolFileAccess_GrepTool(t *testing.T) {
	state := &LoopState{}
	recordToolFileAccess(state, "Grep", map[string]any{
		"path": "/tmp/grep-dir",
	})
	if !state.AccessedFiles["/tmp/grep-dir"]["grep"] {
		t.Error("expected grep op for /tmp/grep-dir")
	}
}

func TestRecordToolFileAccess_NotebookEdit(t *testing.T) {
	state := &LoopState{}
	recordToolFileAccess(state, "NotebookEdit", map[string]any{
		"notebook_path": "/tmp/notebook.ipynb",
	})
	if !state.AccessedFiles["/tmp/notebook.ipynb"]["edit"] {
		t.Error("expected edit op for /tmp/notebook.ipynb")
	}
}

func TestRecordToolFileAccess_UntrackedTool(t *testing.T) {
	state := &LoopState{}
	recordToolFileAccess(state, "AskUserQuestion", map[string]any{
		"question": "How are you?",
	})
	if state.AccessedFiles != nil {
		t.Error("untracked tool should not populate AccessedFiles")
	}
}

func TestRecordToolFileAccess_EmptyPath(t *testing.T) {
	state := &LoopState{}
	recordToolFileAccess(state, "Read", map[string]any{
		"file_path": "",
	})
	if state.AccessedFiles != nil {
		t.Error("empty path should not populate AccessedFiles")
	}
}

func TestExecuteSingleTool_RecordsFileAccess(t *testing.T) {
	// This is an integration-style test verifying that after tool execution,
	// file access is recorded in LoopState. Covered indirectly by the
	// recordToolFileAccess unit tests above, since executeSingleTool calls it.
	state := &LoopState{}

	// Simulate what executeSingleTool does for a Read tool
	recordToolFileAccess(state, "Read", map[string]any{"file_path": "/foo/bar.go"})
	recordToolFileAccess(state, "Write", map[string]any{"file_path": "/foo/bar.go"})
	recordToolFileAccess(state, "Edit", map[string]any{"file_path": "/foo/baz.go"})

	if len(state.AccessedFiles) != 2 {
		t.Errorf("expected 2 files, got %d", len(state.AccessedFiles))
	}
	if len(state.AccessedFiles["/foo/bar.go"]) != 2 {
		t.Errorf("expected 2 ops on /foo/bar.go, got %d", len(state.AccessedFiles["/foo/bar.go"]))
	}
}

// --- Parallel tool execution tests ---

// slowMockTool is a mock tool that sleeps for a configurable duration.
type slowMockTool struct {
	name      string
	delay     time.Duration
	output    tools.ToolOutput
	sideEff   tools.SideEffectType
	callCount atomic.Int32
}

func (s *slowMockTool) Name() string               { return s.name }
func (s *slowMockTool) Description() string         { return "slow mock tool" }
func (s *slowMockTool) InputSchema() map[string]any { return map[string]any{"type": "object"} }
func (s *slowMockTool) SideEffect() tools.SideEffectType { return s.sideEff }

func (s *slowMockTool) Execute(_ context.Context, _ map[string]any) (tools.ToolOutput, error) {
	s.callCount.Add(1)
	time.Sleep(s.delay)
	return s.output, nil
}

func TestCanRunParallel(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(&slowMockTool{name: "ReadOnly1", sideEff: tools.SideEffectNone})
	registry.Register(&slowMockTool{name: "ReadOnly2", sideEff: tools.SideEffectNone})
	registry.Register(&slowMockTool{name: "Writer", sideEff: tools.SideEffectMutating})

	// All read-only → parallel
	if !canRunParallel([]types.ContentBlock{
		{Name: "ReadOnly1", ID: "a"},
		{Name: "ReadOnly2", ID: "b"},
	}, registry) {
		t.Error("expected parallel for all read-only tools")
	}

	// Mixed → serial
	if canRunParallel([]types.ContentBlock{
		{Name: "ReadOnly1", ID: "a"},
		{Name: "Writer", ID: "b"},
	}, registry) {
		t.Error("expected serial when any tool has side effects")
	}

	// Unknown tool → serial
	if canRunParallel([]types.ContentBlock{
		{Name: "ReadOnly1", ID: "a"},
		{Name: "NonexistentTool", ID: "b"},
	}, registry) {
		t.Error("expected serial for unknown tool")
	}

	// Nil registry → serial
	if canRunParallel([]types.ContentBlock{{Name: "X", ID: "a"}}, nil) {
		t.Error("expected serial for nil registry")
	}
}

func TestExecuteTools_ParallelReadOnly(t *testing.T) {
	delay := 50 * time.Millisecond
	tool1 := &slowMockTool{name: "Read1", delay: delay, sideEff: tools.SideEffectNone, output: tools.ToolOutput{Content: "result1"}}
	tool2 := &slowMockTool{name: "Read2", delay: delay, sideEff: tools.SideEffectNone, output: tools.ToolOutput{Content: "result2"}}
	tool3 := &slowMockTool{name: "Read3", delay: delay, sideEff: tools.SideEffectNone, output: tools.ToolOutput{Content: "result3"}}

	registry := tools.NewRegistry()
	registry.Register(tool1)
	registry.Register(tool2)
	registry.Register(tool3)

	config := &AgentConfig{
		ToolRegistry: registry,
		Permissions:  &AllowAllChecker{},
		Hooks:        &NoOpHookRunner{},
	}
	state := &LoopState{}
	ch := make(chan types.SDKMessage, 100)

	blocks := []types.ContentBlock{
		{Name: "Read1", ID: "tc1", Input: map[string]any{}},
		{Name: "Read2", ID: "tc2", Input: map[string]any{}},
		{Name: "Read3", ID: "tc3", Input: map[string]any{}},
	}

	start := time.Now()
	results, interrupted := executeTools(context.Background(), blocks, config, state, ch)
	elapsed := time.Since(start)

	if interrupted {
		t.Error("unexpected interrupt")
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Verify results are in correct order (indexed, not appended)
	for i, r := range results {
		expected := blocks[i].ID
		if r.ToolUseID != expected {
			t.Errorf("result[%d].ToolUseID = %q, want %q", i, r.ToolUseID, expected)
		}
	}
	if results[0].Content != "result1" || results[1].Content != "result2" || results[2].Content != "result3" {
		t.Errorf("unexpected results: %v", results)
	}

	// Parallel execution should be ~1x delay, not ~3x
	// Allow generous margin for CI but check it's not fully serial
	if elapsed > delay*2+50*time.Millisecond {
		t.Errorf("expected parallel execution (~%v), but took %v", delay, elapsed)
	}

	// All tools should have been called
	if tool1.callCount.Load() != 1 || tool2.callCount.Load() != 1 || tool3.callCount.Load() != 1 {
		t.Error("each tool should be called exactly once")
	}
}

func TestExecuteTools_SerialWhenSideEffects(t *testing.T) {
	readTool := &slowMockTool{name: "ReadTool", sideEff: tools.SideEffectNone, output: tools.ToolOutput{Content: "read"}}
	writeTool := &slowMockTool{name: "WriteTool", sideEff: tools.SideEffectMutating, output: tools.ToolOutput{Content: "written"}}

	registry := tools.NewRegistry()
	registry.Register(readTool)
	registry.Register(writeTool)

	config := &AgentConfig{
		ToolRegistry: registry,
		Permissions:  &AllowAllChecker{},
		Hooks:        &NoOpHookRunner{},
	}
	state := &LoopState{}
	ch := make(chan types.SDKMessage, 100)

	blocks := []types.ContentBlock{
		{Name: "ReadTool", ID: "tc1", Input: map[string]any{}},
		{Name: "WriteTool", ID: "tc2", Input: map[string]any{}},
	}

	results, interrupted := executeTools(context.Background(), blocks, config, state, ch)

	if interrupted {
		t.Error("unexpected interrupt")
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Content != "read" || results[1].Content != "written" {
		t.Errorf("unexpected results: %v", results)
	}
}

func TestExecuteTools_ConcurrencyLimit(t *testing.T) {
	var maxConcurrent atomic.Int32
	var currentConcurrent atomic.Int32

	concurrencyTool := &concurrencyTrackingTool{
		name:           "ConcTool",
		delay:          30 * time.Millisecond,
		current:        &currentConcurrent,
		maxObserved:    &maxConcurrent,
	}

	registry := tools.NewRegistry()
	registry.Register(concurrencyTool)

	config := &AgentConfig{
		ToolRegistry:     registry,
		Permissions:      &AllowAllChecker{},
		Hooks:            &NoOpHookRunner{},
		MaxParallelTools: 2, // limit to 2
	}
	state := &LoopState{}
	ch := make(chan types.SDKMessage, 100)

	// Launch 5 tools (all same name, different IDs)
	blocks := make([]types.ContentBlock, 5)
	for i := range blocks {
		blocks[i] = types.ContentBlock{
			Name:  "ConcTool",
			ID:    "tc" + string(rune('a'+i)),
			Input: map[string]any{},
		}
	}

	results, interrupted := executeTools(context.Background(), blocks, config, state, ch)

	if interrupted {
		t.Error("unexpected interrupt")
	}
	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}

	// Max concurrent should not exceed our limit of 2
	observed := maxConcurrent.Load()
	if observed > 2 {
		t.Errorf("max concurrent = %d, want <= 2", observed)
	}
	// Should have observed at least 2 (proving some parallelism occurred)
	if observed < 2 {
		t.Errorf("max concurrent = %d, want >= 2 (parallel execution should occur)", observed)
	}
}

// concurrencyTrackingTool tracks max concurrent executions.
type concurrencyTrackingTool struct {
	name        string
	delay       time.Duration
	current     *atomic.Int32
	maxObserved *atomic.Int32
}

func (c *concurrencyTrackingTool) Name() string               { return c.name }
func (c *concurrencyTrackingTool) Description() string         { return "concurrency tracking tool" }
func (c *concurrencyTrackingTool) InputSchema() map[string]any { return map[string]any{"type": "object"} }
func (c *concurrencyTrackingTool) SideEffect() tools.SideEffectType { return tools.SideEffectNone }

func (c *concurrencyTrackingTool) Execute(_ context.Context, _ map[string]any) (tools.ToolOutput, error) {
	cur := c.current.Add(1)
	// Update max if current is higher
	for {
		max := c.maxObserved.Load()
		if cur <= max || c.maxObserved.CompareAndSwap(max, cur) {
			break
		}
	}
	time.Sleep(c.delay)
	c.current.Add(-1)
	return tools.ToolOutput{Content: "ok"}, nil
}

func TestExecuteTools_ParallelInterrupt(t *testing.T) {
	// One tool returns normally, another triggers a permission interrupt
	readTool := &slowMockTool{
		name:    "SlowRead",
		delay:   20 * time.Millisecond,
		sideEff: tools.SideEffectNone,
		output:  tools.ToolOutput{Content: "read-result"},
	}

	registry := tools.NewRegistry()
	registry.Register(readTool)

	// Permission checker that denies the second tool with interrupt
	interruptChecker := &interruptOnSecondChecker{}

	config := &AgentConfig{
		ToolRegistry: registry,
		Permissions:  interruptChecker,
		Hooks:        &NoOpHookRunner{},
	}
	state := &LoopState{}
	ch := make(chan types.SDKMessage, 100)

	blocks := []types.ContentBlock{
		{Name: "SlowRead", ID: "tc1", Input: map[string]any{}},
		{Name: "SlowRead", ID: "tc2", Input: map[string]any{}},
		{Name: "SlowRead", ID: "tc3", Input: map[string]any{}},
	}

	results, interrupted := executeTools(context.Background(), blocks, config, state, ch)

	if !interrupted {
		t.Error("expected interrupt")
	}

	// All results should be filled (no empty ToolUseIDs)
	for i, r := range results {
		if r.ToolUseID == "" {
			t.Errorf("result[%d] has empty ToolUseID", i)
		}
	}
}

// interruptOnSecondChecker allows the first call and interrupts the second.
type interruptOnSecondChecker struct {
	calls atomic.Int32
}

func (c *interruptOnSecondChecker) Check(_ context.Context, _ string, _ map[string]any) (PermissionResult, error) {
	n := c.calls.Add(1)
	if n >= 2 {
		return PermissionResult{
			Behavior:  "deny",
			Message:   "interrupted",
			Interrupt: true,
		}, nil
	}
	return PermissionResult{Behavior: "allow"}, nil
}

func TestExecuteTools_SingleToolNoParallel(t *testing.T) {
	// Single tool should always use serial execution, even if side-effect-free
	readTool := &slowMockTool{name: "SingleRead", sideEff: tools.SideEffectNone, output: tools.ToolOutput{Content: "solo"}}

	registry := tools.NewRegistry()
	registry.Register(readTool)

	config := &AgentConfig{
		ToolRegistry: registry,
		Permissions:  &AllowAllChecker{},
		Hooks:        &NoOpHookRunner{},
	}
	state := &LoopState{}
	ch := make(chan types.SDKMessage, 100)

	blocks := []types.ContentBlock{
		{Name: "SingleRead", ID: "tc1", Input: map[string]any{}},
	}

	results, interrupted := executeTools(context.Background(), blocks, config, state, ch)

	if interrupted {
		t.Error("unexpected interrupt")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Content != "solo" {
		t.Errorf("unexpected content: %q", results[0].Content)
	}
}

// --- Skill permission scoping tests ---

// skillDenyChecker always denies permission.
type skillDenyChecker struct{}

func (d *skillDenyChecker) Check(_ context.Context, _ string, _ map[string]any) (PermissionResult, error) {
	return PermissionResult{Behavior: "deny", Message: "denied by default"}, nil
}

func TestSkillPermissionWrapper_AllowedTool(t *testing.T) {
	wrapper := &skillPermissionWrapper{
		allowedTools: []string{"Bash", "Read"},
		inner:        &skillDenyChecker{},
	}
	result, err := wrapper.Check(context.Background(), "Bash", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Behavior != "allow" {
		t.Errorf("expected allow, got %s", result.Behavior)
	}
}

func TestSkillPermissionWrapper_DeniedTool(t *testing.T) {
	wrapper := &skillPermissionWrapper{
		allowedTools: []string{"Bash"},
		inner:        &skillDenyChecker{},
	}
	result, err := wrapper.Check(context.Background(), "Write", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Behavior != "deny" {
		t.Errorf("expected deny, got %s", result.Behavior)
	}
}

func TestSkillPermissionWrapper_GlobPattern(t *testing.T) {
	wrapper := &skillPermissionWrapper{
		allowedTools: []string{"mcp__*"},
		inner:        &skillDenyChecker{},
	}
	result, err := wrapper.Check(context.Background(), "mcp__server__tool", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Behavior != "allow" {
		t.Errorf("expected allow for glob match, got %s", result.Behavior)
	}
}

func TestSkillPermissionWrapper_ConstraintMatch(t *testing.T) {
	wrapper := &skillPermissionWrapper{
		allowedTools: []string{"Bash(gh:*)"},
		inner:        &skillDenyChecker{},
	}
	result, err := wrapper.Check(context.Background(), "Bash", map[string]any{
		"command": "gh pr list",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Behavior != "allow" {
		t.Errorf("expected allow for constraint match, got %s", result.Behavior)
	}

	// Non-matching command should deny
	result2, _ := wrapper.Check(context.Background(), "Bash", map[string]any{
		"command": "rm -rf /",
	})
	if result2.Behavior != "deny" {
		t.Errorf("expected deny for non-matching command, got %s", result2.Behavior)
	}
}

func TestEffectivePermissionChecker_NoActiveSkill(t *testing.T) {
	base := &AllowAllChecker{}
	state := &LoopState{}
	checker := effectivePermissionChecker(base, state)
	if checker != base {
		t.Error("expected base checker when no active skill")
	}
}

func TestEffectivePermissionChecker_WithActiveSkill(t *testing.T) {
	base := &skillDenyChecker{}
	state := &LoopState{
		ActiveSkill: &SkillScope{
			SkillName:    "test-skill",
			AllowedTools: []string{"Bash"},
		},
	}
	checker := effectivePermissionChecker(base, state)
	if checker == base {
		t.Error("expected wrapped checker when active skill is set")
	}
	// Allowed tool should pass
	result, _ := checker.Check(context.Background(), "Bash", nil)
	if result.Behavior != "allow" {
		t.Errorf("expected allow for skill-allowed tool, got %s", result.Behavior)
	}
	// Non-allowed tool should delegate to inner (deny)
	result2, _ := checker.Check(context.Background(), "Write", nil)
	if result2.Behavior != "deny" {
		t.Errorf("expected deny for non-allowed tool, got %s", result2.Behavior)
	}
}

func TestEffectivePermissionChecker_EmptyAllowedTools(t *testing.T) {
	base := &AllowAllChecker{}
	state := &LoopState{
		ActiveSkill: &SkillScope{
			SkillName:    "test-skill",
			AllowedTools: []string{},
		},
	}
	checker := effectivePermissionChecker(base, state)
	if checker != base {
		t.Error("expected base checker when allowed-tools is empty")
	}
}

// mockSkillProvider implements SkillProvider for testing setActiveSkillScope.
type mockSkillProvider struct {
	skills map[string]types.SkillEntry
}

func (m *mockSkillProvider) GetSkill(name string) (types.SkillEntry, bool) {
	e, ok := m.skills[name]
	return e, ok
}
func (m *mockSkillProvider) ListSkills() []types.SkillEntry { return nil }
func (m *mockSkillProvider) SkillNames() []string           { return nil }
func (m *mockSkillProvider) SlashCommands() []string        { return nil }
func (m *mockSkillProvider) FormatSkillsList() string       { return "" }

func TestSetActiveSkillScope_SetsScope(t *testing.T) {
	config := &AgentConfig{
		Skills: &mockSkillProvider{
			skills: map[string]types.SkillEntry{
				"deploy": {
					SkillDefinition: types.SkillDefinition{
						Name:         "deploy",
						AllowedTools: []string{"Bash(gh:*)", "Read"},
					},
				},
			},
		},
	}
	state := &LoopState{}
	blocks := []types.ContentBlock{
		{Name: "Skill", ID: "tc1", Input: map[string]any{"skill": "deploy"}},
	}

	setActiveSkillScope(blocks, config, state)

	if state.ActiveSkill == nil {
		t.Fatal("expected ActiveSkill to be set")
	}
	if state.ActiveSkill.SkillName != "deploy" {
		t.Errorf("expected skill name 'deploy', got %q", state.ActiveSkill.SkillName)
	}
	if len(state.ActiveSkill.AllowedTools) != 2 {
		t.Errorf("expected 2 allowed tools, got %d", len(state.ActiveSkill.AllowedTools))
	}
}

func TestSetActiveSkillScope_NoScope_NoAllowedTools(t *testing.T) {
	config := &AgentConfig{
		Skills: &mockSkillProvider{
			skills: map[string]types.SkillEntry{
				"simple": {
					SkillDefinition: types.SkillDefinition{
						Name: "simple",
					},
				},
			},
		},
	}
	state := &LoopState{}
	blocks := []types.ContentBlock{
		{Name: "Skill", ID: "tc1", Input: map[string]any{"skill": "simple"}},
	}

	setActiveSkillScope(blocks, config, state)

	if state.ActiveSkill != nil {
		t.Error("expected no ActiveSkill when skill has no allowed-tools")
	}
}

func TestSetActiveSkillScope_NoScope_NilSkillProvider(t *testing.T) {
	config := &AgentConfig{Skills: nil}
	state := &LoopState{}
	blocks := []types.ContentBlock{
		{Name: "Skill", ID: "tc1", Input: map[string]any{"skill": "anything"}},
	}

	setActiveSkillScope(blocks, config, state)

	if state.ActiveSkill != nil {
		t.Error("expected no ActiveSkill when skill provider is nil")
	}
}

func TestSetActiveSkillScope_IgnoresNonSkillTools(t *testing.T) {
	config := &AgentConfig{
		Skills: &mockSkillProvider{
			skills: map[string]types.SkillEntry{
				"deploy": {
					SkillDefinition: types.SkillDefinition{
						Name:         "deploy",
						AllowedTools: []string{"Bash"},
					},
				},
			},
		},
	}
	state := &LoopState{}
	blocks := []types.ContentBlock{
		{Name: "Bash", ID: "tc1", Input: map[string]any{"command": "ls"}},
		{Name: "Read", ID: "tc2", Input: map[string]any{"file_path": "/tmp/f"}},
	}

	setActiveSkillScope(blocks, config, state)

	if state.ActiveSkill != nil {
		t.Error("expected no ActiveSkill when no Skill tool in blocks")
	}
}
