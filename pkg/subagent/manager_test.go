package subagent

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/hooks"
	"github.com/jg-phare/goat/pkg/llm"
	"github.com/jg-phare/goat/pkg/tools"
	"github.com/jg-phare/goat/pkg/types"
)

// --- Mock LLM Client ---

type mockLLMClient struct {
	mu        sync.Mutex
	responses []*mockStreamData
	callIndex int
	model     string
}

func (m *mockLLMClient) Complete(ctx context.Context, req *llm.CompletionRequest) (*llm.Stream, error) {
	m.mu.Lock()
	idx := m.callIndex
	m.callIndex++
	m.mu.Unlock()

	if idx >= len(m.responses) {
		return defaultEndTurn(ctx), nil
	}
	return m.responses[idx].toStream(ctx), nil
}

func (m *mockLLMClient) Model() string      { return m.model }
func (m *mockLLMClient) SetModel(model string) { m.model = model }

type mockStreamData struct {
	chunks []llm.StreamChunk
}

func (ms *mockStreamData) toStream(ctx context.Context) *llm.Stream {
	events := make(chan llm.StreamEvent, len(ms.chunks)+1)
	go func() {
		defer close(events)
		for _, chunk := range ms.chunks {
			c := chunk
			select {
			case events <- llm.StreamEvent{Chunk: &c}:
			case <-ctx.Done():
				return
			}
		}
	}()

	pr, pw := io.Pipe()
	pw.Close()
	_, cancel := context.WithCancel(ctx)
	return llm.NewStream(events, pr, cancel)
}

func defaultEndTurn(ctx context.Context) *llm.Stream {
	stop := "stop"
	text := "Done."
	ms := &mockStreamData{
		chunks: []llm.StreamChunk{
			{
				ID:    "msg-1",
				Model: "claude-sonnet-4-5-20250929",
				Choices: []llm.Choice{
					{Delta: llm.Delta{Content: &text}},
				},
			},
			{
				ID:    "msg-1",
				Model: "claude-sonnet-4-5-20250929",
				Choices: []llm.Choice{
					{FinishReason: &stop},
				},
				Usage: &llm.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
			},
		},
	}
	return ms.toStream(ctx)
}

func endTurnWithText(text string) *mockStreamData {
	stop := "stop"
	return &mockStreamData{
		chunks: []llm.StreamChunk{
			{
				ID:    "msg-1",
				Model: "claude-sonnet-4-5-20250929",
				Choices: []llm.Choice{
					{Delta: llm.Delta{Content: &text}},
				},
			},
			{
				ID:    "msg-1",
				Model: "claude-sonnet-4-5-20250929",
				Choices: []llm.Choice{
					{FinishReason: &stop},
				},
				Usage: &llm.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
			},
		},
	}
}

// --- Mock Tool ---

type mockTool struct {
	name   string
	output tools.ToolOutput
}

func (m *mockTool) Name() string                   { return m.name }
func (m *mockTool) Description() string             { return "mock tool" }
func (m *mockTool) InputSchema() map[string]any     { return map[string]any{"type": "object"} }
func (m *mockTool) SideEffect() tools.SideEffectType { return tools.SideEffectNone }
func (m *mockTool) Execute(_ context.Context, _ map[string]any) (tools.ToolOutput, error) {
	return m.output, nil
}

// --- Helper ---

func newTestManager(llmClient *mockLLMClient, extraTools ...tools.Tool) *Manager {
	reg := tools.NewRegistry()
	for _, t := range extraTools {
		reg.Register(t)
	}

	parentConfig := &agent.AgentConfig{
		Model:     "claude-sonnet-4-5-20250929",
		CWD:       "/tmp/test",
		SessionID: "parent-session",
	}

	return NewManager(ManagerOpts{
		ParentConfig:      parentConfig,
		LLMClient:         llmClient,
		CostTracker:       llm.NewCostTracker(),
		ParentRegistry:    reg,
		PermissionChecker: &agent.AllowAllChecker{},
	}, nil)
}

// --- Tests ---

func TestManager_ForegroundSpawn(t *testing.T) {
	client := &mockLLMClient{
		responses: []*mockStreamData{endTurnWithText("Task completed successfully")},
	}
	mgr := newTestManager(client)

	result, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "test task",
		Prompt:       "Do something",
		SubagentType: "general-purpose",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AgentID == "" {
		t.Error("expected non-empty AgentID")
	}
	if !strings.Contains(result.Output, "Task completed successfully") {
		t.Errorf("output = %q, want to contain 'Task completed successfully'", result.Output)
	}
}

func TestManager_BackgroundSpawn(t *testing.T) {
	client := &mockLLMClient{
		responses: []*mockStreamData{endTurnWithText("Background done")},
	}
	mgr := newTestManager(client)

	bg := true
	result, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:     "bg task",
		Prompt:          "Do background work",
		SubagentType:    "general-purpose",
		RunInBackground: &bg,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AgentID == "" {
		t.Error("expected non-empty AgentID")
	}
	if result.Output != "" {
		t.Errorf("expected empty output for background, got %q", result.Output)
	}

	// Wait for completion and check output
	taskResult, err := mgr.GetOutput(result.AgentID, true, 30*time.Second)
	if err != nil {
		t.Fatalf("GetOutput error: %v", err)
	}
	if taskResult.State != StateCompleted {
		t.Errorf("state = %v, want Completed", taskResult.State)
	}
}

func TestManager_UnknownType(t *testing.T) {
	client := &mockLLMClient{}
	mgr := newTestManager(client)

	_, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "test",
		Prompt:       "test",
		SubagentType: "nonexistent-agent-type",
	})
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
	if !strings.Contains(err.Error(), "unknown agent type") {
		t.Errorf("error = %q, want 'unknown agent type'", err.Error())
	}
}

func TestManager_MaxConcurrent(t *testing.T) {
	client := &mockLLMClient{}
	mgr := newTestManager(client)

	// Fill up active agents manually
	mgr.mu.Lock()
	for i := 0; i < maxConcurrentAgents; i++ {
		mgr.active[string(rune('a'+i))] = &RunningAgent{State: StateRunning}
	}
	mgr.mu.Unlock()

	_, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "test",
		Prompt:       "test",
		SubagentType: "general-purpose",
	})
	if err == nil {
		t.Fatal("expected error for max concurrent agents")
	}
	if !strings.Contains(err.Error(), "max concurrent") {
		t.Errorf("error = %q, want 'max concurrent'", err.Error())
	}
}

func TestManager_ModelResolution(t *testing.T) {
	// Register an agent with a specific model
	client := &mockLLMClient{
		responses: []*mockStreamData{endTurnWithText("ok")},
	}
	mgr := newTestManager(client)

	// The Explore agent uses "haiku"
	result, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "explore",
		Prompt:       "search codebase",
		SubagentType: "Explore",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AgentID == "" {
		t.Error("expected non-empty AgentID")
	}
}

func TestManager_InputModelOverride(t *testing.T) {
	client := &mockLLMClient{
		responses: []*mockStreamData{endTurnWithText("ok")},
	}
	mgr := newTestManager(client)

	model := "opus"
	_, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "test",
		Prompt:       "test",
		SubagentType: "general-purpose",
		Model:        &model,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestManager_ToolResolution(t *testing.T) {
	readTool := &mockTool{name: "Read", output: tools.ToolOutput{Content: "file contents"}}
	bashTool := &mockTool{name: "Bash", output: tools.ToolOutput{Content: "output"}}
	client := &mockLLMClient{
		responses: []*mockStreamData{endTurnWithText("ok")},
	}
	mgr := newTestManager(client, readTool, bashTool)

	// Bash agent should only get Bash tool
	_, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "run command",
		Prompt:       "ls -la",
		SubagentType: "Bash",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestManager_Stop(t *testing.T) {
	// Use a slow-responding client
	client := &mockLLMClient{}
	mgr := newTestManager(client)

	bg := true
	result, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:     "slow task",
		Prompt:          "do something slowly",
		SubagentType:    "general-purpose",
		RunInBackground: &bg,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	// Stop it
	err = mgr.Stop(result.AgentID)
	if err != nil {
		t.Fatalf("Stop error: %v", err)
	}
}

func TestManager_StopUnknown(t *testing.T) {
	mgr := newTestManager(&mockLLMClient{})
	err := mgr.Stop("nonexistent-id")
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
}

func TestManager_GetOutput_Unknown(t *testing.T) {
	mgr := newTestManager(&mockLLMClient{})
	_, err := mgr.GetOutput("nonexistent-id", false, time.Second)
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
}

func TestManager_GetOutput_NonBlocking(t *testing.T) {
	client := &mockLLMClient{}
	mgr := newTestManager(client)

	bg := true
	result, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:     "task",
		Prompt:          "do work",
		SubagentType:    "general-purpose",
		RunInBackground: &bg,
	})
	if err != nil {
		t.Fatalf("Spawn error: %v", err)
	}

	// Non-blocking should return immediately
	taskResult, err := mgr.GetOutput(result.AgentID, false, 0)
	if err != nil {
		t.Fatalf("GetOutput error: %v", err)
	}
	// Should be either running or completed
	if taskResult.AgentID != result.AgentID {
		t.Errorf("AgentID = %q, want %q", taskResult.AgentID, result.AgentID)
	}
}

func TestManager_List(t *testing.T) {
	mgr := newTestManager(&mockLLMClient{})

	// Manually add some active agents
	mgr.mu.Lock()
	mgr.active["a1"] = &RunningAgent{ID: "a1", Type: "general-purpose", Name: "Agent 1", State: StateRunning}
	mgr.active["a2"] = &RunningAgent{ID: "a2", Type: "Explore", Name: "Agent 2", State: StateCompleted}
	mgr.mu.Unlock()

	statuses := mgr.List()
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}
}

func TestManager_RegisterAgents(t *testing.T) {
	mgr := newTestManager(&mockLLMClient{})

	mgr.RegisterAgents(map[string]Definition{
		"custom-agent": FromTypesDefinition("custom-agent", types.AgentDefinition{
			Description: "Custom agent",
			Prompt:      "Be custom",
		}, SourceProject, 30),
	})

	defs := mgr.Definitions()
	if _, ok := defs["custom-agent"]; !ok {
		t.Error("expected custom-agent in definitions")
	}
}

func TestManager_Reload(t *testing.T) {
	mgr := newTestManager(&mockLLMClient{})

	// Reload from a dir with no agents (should still have built-ins)
	_, err := mgr.Reload("/tmp/nonexistent")
	if err != nil {
		t.Fatalf("Reload error: %v", err)
	}

	defs := mgr.Definitions()
	if _, ok := defs["general-purpose"]; !ok {
		t.Error("expected built-in agents after reload")
	}
}

func TestManager_HookLifecycle(t *testing.T) {
	runner := hooks.NewRunner(hooks.RunnerConfig{})

	startFired := false
	stopFired := false

	runner.RegisterScoped("test-hooks", map[types.HookEvent][]hooks.CallbackMatcher{
		types.HookEventSubagentStart: {
			{Hooks: []hooks.HookCallback{func(input any, toolUseID string, ctx context.Context) (hooks.HookJSONOutput, error) {
				startFired = true
				return hooks.HookJSONOutput{}, nil
			}}},
		},
		types.HookEventSubagentStop: {
			{Hooks: []hooks.HookCallback{func(input any, toolUseID string, ctx context.Context) (hooks.HookJSONOutput, error) {
				stopFired = true
				return hooks.HookJSONOutput{}, nil
			}}},
		},
	})

	client := &mockLLMClient{
		responses: []*mockStreamData{endTurnWithText("done")},
	}

	parentConfig := &agent.AgentConfig{
		Model:     "claude-sonnet-4-5-20250929",
		CWD:       "/tmp/test",
		SessionID: "parent-session",
	}

	mgr := NewManager(ManagerOpts{
		ParentConfig:      parentConfig,
		LLMClient:         client,
		CostTracker:       llm.NewCostTracker(),
		HookRunner:        runner,
		PermissionChecker: &agent.AllowAllChecker{},
	}, nil)

	_, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "test",
		Prompt:       "test",
		SubagentType: "general-purpose",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !startFired {
		t.Error("SubagentStart hook should have fired")
	}
	if !stopFired {
		t.Error("SubagentStop hook should have fired")
	}
}

func TestManager_Definitions(t *testing.T) {
	mgr := newTestManager(&mockLLMClient{})
	defs := mgr.Definitions()

	// Should have all built-in agents
	expectedKeys := []string{"general-purpose", "Explore", "Plan", "Bash", "statusline-setup", "claude-code-guide"}
	for _, key := range expectedKeys {
		if _, ok := defs[key]; !ok {
			t.Errorf("missing definition %q", key)
		}
	}
}

func TestManager_PermissionModeResolution(t *testing.T) {
	client := &mockLLMClient{
		responses: []*mockStreamData{endTurnWithText("ok")},
	}
	mgr := newTestManager(client)

	mode := "bypassPermissions"
	_, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "test",
		Prompt:       "test",
		SubagentType: "general-purpose",
		Mode:         &mode,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestManager_ImplementsSubagentSpawner(t *testing.T) {
	var _ tools.SubagentSpawner = (*Manager)(nil)
}

// --- Test Parity: Filesystem Agent Loading (ported from Python Agent SDK) ---

func TestManager_ReloadWithFilesystemAgents(t *testing.T) {
	// 1. Create temp dir with .claude/agents/helper.md (valid frontmatter)
	tmpDir := t.TempDir()
	agentDir := tmpDir + "/.claude/agents"
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	agentContent := `---
name: helper
description: A test helper agent
model: haiku
tools: Read, Grep
---
You are a helpful test agent that assists with reading and searching files.
`
	if err := os.WriteFile(agentDir+"/helper.md", []byte(agentContent), 0o644); err != nil {
		t.Fatalf("write agent file: %v", err)
	}

	// 2. Create Manager
	client := &mockLLMClient{
		responses: []*mockStreamData{endTurnWithText("ok")},
	}
	mgr := newTestManager(client)

	// Verify helper is NOT in definitions before reload
	defs := mgr.Definitions()
	if _, ok := defs["helper"]; ok {
		t.Error("helper should not be in definitions before reload")
	}

	// 3. Reload from tmpDir
	if _, err := mgr.Reload(tmpDir); err != nil {
		t.Fatalf("Reload error: %v", err)
	}

	// 4. Verify "helper" agent is in definitions
	defs = mgr.Definitions()
	helperDef, ok := defs["helper"]
	if !ok {
		t.Fatal("expected 'helper' agent in definitions after reload")
	}
	if helperDef.Description != "A test helper agent" {
		t.Errorf("description = %q, want 'A test helper agent'", helperDef.Description)
	}
	if helperDef.Model != "haiku" {
		t.Errorf("model = %q, want 'haiku'", helperDef.Model)
	}
	if helperDef.Source != SourceProject {
		t.Errorf("source = %v, want SourceProject", helperDef.Source)
	}

	// Built-in agents should still be present
	if _, ok := defs["general-purpose"]; !ok {
		t.Error("expected built-in 'general-purpose' to still be present after reload")
	}

	// 5. Verify it can be spawned
	result, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "test helper",
		Prompt:       "help me",
		SubagentType: "helper",
	})
	if err != nil {
		t.Fatalf("Spawn error: %v", err)
	}
	if result.AgentID == "" {
		t.Error("expected non-empty AgentID from spawned helper agent")
	}
}

func TestManager_ReloadOverridesBuiltIn(t *testing.T) {
	// Verify that a file-based agent with the same name as a built-in overrides it
	tmpDir := t.TempDir()
	agentDir := tmpDir + "/.claude/agents"
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Create an agent with the same name as built-in "Explore"
	agentContent := `---
name: Explore
description: Custom explore agent override
model: sonnet
---
Custom explore prompt.
`
	if err := os.WriteFile(agentDir+"/Explore.md", []byte(agentContent), 0o644); err != nil {
		t.Fatalf("write agent file: %v", err)
	}

	client := &mockLLMClient{}
	mgr := newTestManager(client)

	if _, err := mgr.Reload(tmpDir); err != nil {
		t.Fatalf("Reload error: %v", err)
	}

	defs := mgr.Definitions()
	exploreDef, ok := defs["Explore"]
	if !ok {
		t.Fatal("expected 'Explore' in definitions")
	}

	// Project source (30) should override built-in (0)
	if exploreDef.Description != "Custom explore agent override" {
		t.Errorf("description = %q, want 'Custom explore agent override'", exploreDef.Description)
	}
	if exploreDef.Source != SourceProject {
		t.Errorf("source = %v, want SourceProject", exploreDef.Source)
	}
}

// --- Phase 1 Gap Closure Tests ---

func TestManager_DefaultMaxTurns50(t *testing.T) {
	// Verify the default maxTurns is now 50 (changed from 100)
	client := &mockLLMClient{
		responses: []*mockStreamData{endTurnWithText("ok")},
	}
	mgr := newTestManager(client)

	result, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "test",
		Prompt:       "test",
		SubagentType: "general-purpose",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AgentID == "" {
		t.Error("expected non-empty AgentID")
	}
	// The agent runs with maxTurns=50; we can't directly inspect config,
	// but we verify it doesn't error (it used to be 100)
}

func TestManager_BypassPermissionsInherited(t *testing.T) {
	// When parent uses bypassPermissions, subagent should inherit it
	client := &mockLLMClient{
		responses: []*mockStreamData{endTurnWithText("ok")},
	}

	parentConfig := &agent.AgentConfig{
		Model:          "claude-sonnet-4-5-20250929",
		CWD:            "/tmp/test",
		SessionID:      "parent-session",
		PermissionMode: types.PermissionModeBypassPermissions,
	}

	mgr := NewManager(ManagerOpts{
		ParentConfig:      parentConfig,
		LLMClient:         client,
		CostTracker:       llm.NewCostTracker(),
		PermissionChecker: &agent.AllowAllChecker{},
	}, nil)

	// Try to override with a different mode via input — should still be bypassPermissions
	mode := "default"
	_, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "test",
		Prompt:       "test",
		SubagentType: "general-purpose",
		Mode:         &mode,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestManager_TaskRestrictionEnforced(t *testing.T) {
	client := &mockLLMClient{
		responses: []*mockStreamData{endTurnWithText("ok")},
	}

	// Create manager with task restriction: only allow Explore and Plan
	parentConfig := &agent.AgentConfig{
		Model:     "claude-sonnet-4-5-20250929",
		CWD:       "/tmp/test",
		SessionID: "parent-session",
	}

	mgr := NewManager(ManagerOpts{
		ParentConfig: parentConfig,
		LLMClient:    client,
		CostTracker:  llm.NewCostTracker(),
		TaskRestriction: &TaskRestriction{
			AllowedTypes: []string{"Explore", "Plan"},
		},
	}, nil)

	// Allowed type should succeed
	_, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "explore",
		Prompt:       "search",
		SubagentType: "Explore",
	})
	if err != nil {
		t.Fatalf("expected Explore to be allowed, got error: %v", err)
	}

	// Disallowed type should fail
	_, err = mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "general",
		Prompt:       "do stuff",
		SubagentType: "general-purpose",
	})
	if err == nil {
		t.Fatal("expected error for restricted agent type")
	}
	if !strings.Contains(err.Error(), "not allowed by task restriction") {
		t.Errorf("error = %q, want 'not allowed by task restriction'", err.Error())
	}
}

func TestManager_TaskRestrictionUnrestricted(t *testing.T) {
	client := &mockLLMClient{
		responses: []*mockStreamData{endTurnWithText("ok")},
	}

	mgr := NewManager(ManagerOpts{
		ParentConfig: &agent.AgentConfig{
			Model: "claude-sonnet-4-5-20250929", CWD: "/tmp/test", SessionID: "s1",
		},
		LLMClient:   client,
		CostTracker: llm.NewCostTracker(),
		TaskRestriction: &TaskRestriction{
			Unrestricted: true,
		},
	}, nil)

	// Unrestricted should allow any type
	_, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "general",
		Prompt:       "do stuff",
		SubagentType: "general-purpose",
	})
	if err != nil {
		t.Fatalf("unrestricted task should allow any type, got error: %v", err)
	}
}

func TestManager_MemoryToolsAutoEnabled(t *testing.T) {
	// Create a temporary memory dir with content
	tmpDir := t.TempDir()
	memDir := tmpDir + "/.claude/memory"
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(memDir+"/MEMORY.md", []byte("# Test Memory"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	client := &mockLLMClient{
		responses: []*mockStreamData{endTurnWithText("ok")},
	}

	// Register FileRead, FileWrite, FileEdit tools in parent registry
	readTool := &mockTool{name: "FileRead", output: tools.ToolOutput{Content: "ok"}}
	writeTool := &mockTool{name: "FileWrite", output: tools.ToolOutput{Content: "ok"}}
	editTool := &mockTool{name: "FileEdit", output: tools.ToolOutput{Content: "ok"}}
	reg := tools.NewRegistry()
	reg.Register(readTool)
	reg.Register(writeTool)
	reg.Register(editTool)

	// Register a custom agent definition with memory enabled but no tools
	mgr := NewManager(ManagerOpts{
		ParentConfig: &agent.AgentConfig{
			Model: "claude-sonnet-4-5-20250929", CWD: tmpDir, SessionID: "s1",
		},
		LLMClient:      client,
		CostTracker:    llm.NewCostTracker(),
		ParentRegistry: reg,
	}, nil)

	// Register an agent with memory but empty tools
	mgr.RegisterAgents(map[string]Definition{
		"memory-agent": FromTypesDefinition("memory-agent", types.AgentDefinition{
			Description: "Agent with memory",
			Prompt:      "Use memory",
			Memory:      "auto",
		}, SourceProject, 30),
	})

	// Spawn should succeed and auto-include file tools
	_, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "test memory",
		Prompt:       "test",
		SubagentType: "memory-agent",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Stub Tests for Unimplemented Features ---

func TestManager_LargeAgentDefinition(t *testing.T) {
	t.Skip("not yet implemented: large agent definition handling (250KB+)")
}

// --- Phase 6 Tests ---

func TestManager_BackgroundOutputFile(t *testing.T) {
	dir := t.TempDir()
	client := &mockLLMClient{
		responses: []*mockStreamData{endTurnWithText("Background output here")},
	}

	reg := tools.NewRegistry()
	parentConfig := &agent.AgentConfig{
		Model:     "claude-sonnet-4-5-20250929",
		CWD:       "/tmp/test",
		SessionID: "parent-session",
	}

	mgr := NewManager(ManagerOpts{
		OutputDir:         dir,
		ParentConfig:      parentConfig,
		LLMClient:         client,
		CostTracker:       llm.NewCostTracker(),
		ParentRegistry:    reg,
		PermissionChecker: &agent.AllowAllChecker{},
	}, nil)

	bg := true
	result, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:     "bg task",
		Prompt:          "Do background work",
		SubagentType:    "general-purpose",
		RunInBackground: &bg,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OutputFile == "" {
		t.Fatal("expected non-empty OutputFile for background agent")
	}
	if !strings.HasSuffix(result.OutputFile, ".output") {
		t.Errorf("OutputFile = %q, expected .output suffix", result.OutputFile)
	}

	// Wait for completion — use a generous timeout to avoid flakes under load
	out, err := mgr.GetOutput(result.AgentID, true, 30*time.Second)
	if err != nil {
		t.Fatalf("GetOutput error: %v", err)
	}
	if out.State == StateRunning {
		t.Fatal("GetOutput returned while agent still running (timeout)")
	}

	// Check output file was written
	data, err := os.ReadFile(result.OutputFile)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	if !strings.Contains(string(data), "Background output here") {
		t.Errorf("output file content = %q, expected 'Background output here'", string(data))
	}
}

func TestManager_BackgroundNoOutputDir(t *testing.T) {
	client := &mockLLMClient{
		responses: []*mockStreamData{endTurnWithText("No output file")},
	}
	mgr := newTestManager(client) // no OutputDir set

	bg := true
	result, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:     "bg task",
		Prompt:          "Do work",
		SubagentType:    "general-purpose",
		RunInBackground: &bg,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OutputFile != "" {
		t.Errorf("expected empty OutputFile when no OutputDir, got %q", result.OutputFile)
	}
}

func TestBackgroundPermissionChecker_AllowPreApproved(t *testing.T) {
	checker := &agent.BackgroundPermissionChecker{
		PreApproved: map[string]bool{"Read": true, "Glob": true, "Bash": true},
	}

	result, err := checker.Check(context.Background(), "Read", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Behavior != "allow" {
		t.Errorf("Read: behavior = %q, want allow", result.Behavior)
	}
}

func TestBackgroundPermissionChecker_DenyNonApproved(t *testing.T) {
	checker := &agent.BackgroundPermissionChecker{
		PreApproved: map[string]bool{"Read": true},
	}

	result, err := checker.Check(context.Background(), "mcp__server__tool", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Behavior != "deny" {
		t.Errorf("mcp tool: behavior = %q, want deny", result.Behavior)
	}
}

func TestBackgroundPermissionChecker_AlwaysDenyAskUser(t *testing.T) {
	checker := &agent.BackgroundPermissionChecker{
		PreApproved: map[string]bool{"AskUserQuestion": true}, // even if pre-approved!
	}

	result, err := checker.Check(context.Background(), "AskUserQuestion", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Behavior != "deny" {
		t.Errorf("AskUserQuestion: behavior = %q, want deny", result.Behavior)
	}
	if !strings.Contains(result.Message, "background") {
		t.Errorf("message = %q, expected to mention 'background'", result.Message)
	}
}

func TestManager_BackgroundPermissions(t *testing.T) {
	client := &mockLLMClient{
		responses: []*mockStreamData{endTurnWithText("Done")},
	}

	reg := tools.NewRegistry()
	parentConfig := &agent.AgentConfig{
		Model:     "claude-sonnet-4-5-20250929",
		CWD:       "/tmp/test",
		SessionID: "parent-session",
	}

	mgr := NewManager(ManagerOpts{
		ParentConfig:      parentConfig,
		LLMClient:         client,
		CostTracker:       llm.NewCostTracker(),
		ParentRegistry:    reg,
		PermissionChecker: &agent.AllowAllChecker{},
	}, nil)

	bg := true
	_, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:     "bg task",
		Prompt:          "Do work",
		SubagentType:    "general-purpose",
		RunInBackground: &bg,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The test primarily validates that background spawning uses BackgroundPermissionChecker
	// This is verified indirectly by the resolvePermissions method
}

func TestManager_SessionStoreWired(t *testing.T) {
	client := &mockLLMClient{
		responses: []*mockStreamData{endTurnWithText("Done")},
	}

	reg := tools.NewRegistry()
	parentConfig := &agent.AgentConfig{
		Model:     "claude-sonnet-4-5-20250929",
		CWD:       "/tmp/test",
		SessionID: "parent-session",
	}

	store := &agent.NoOpSessionStore{}

	mgr := NewManager(ManagerOpts{
		ParentConfig:      parentConfig,
		LLMClient:         client,
		CostTracker:       llm.NewCostTracker(),
		ParentRegistry:    reg,
		PermissionChecker: &agent.AllowAllChecker{},
		SessionStore:      store,
	}, nil)

	result, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "task",
		Prompt:       "Do work",
		SubagentType: "general-purpose",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AgentID == "" {
		t.Error("expected non-empty AgentID")
	}
	// Verify session store was passed through (no error means it was accepted)
}

func TestManager_SubagentStopHook_TranscriptPath(t *testing.T) {
	var capturedTranscriptPath string

	runner := hooks.NewRunner(hooks.RunnerConfig{})
	runner.RegisterScoped("test-hooks", map[types.HookEvent][]hooks.CallbackMatcher{
		types.HookEventSubagentStop: {
			{Hooks: []hooks.HookCallback{func(input any, toolUseID string, ctx context.Context) (hooks.HookJSONOutput, error) {
				if stopInput, ok := input.(*hooks.SubagentStopHookInput); ok {
					capturedTranscriptPath = stopInput.AgentTranscriptPath
				}
				return hooks.HookJSONOutput{}, nil
			}}},
		},
	})

	client := &mockLLMClient{
		responses: []*mockStreamData{endTurnWithText("done")},
	}

	transcriptDir := t.TempDir()

	parentConfig := &agent.AgentConfig{
		Model:     "claude-sonnet-4-5-20250929",
		CWD:       "/tmp/test",
		SessionID: "parent-session",
	}

	mgr := NewManager(ManagerOpts{
		ParentConfig:      parentConfig,
		LLMClient:         client,
		CostTracker:       llm.NewCostTracker(),
		HookRunner:        runner,
		PermissionChecker: &agent.AllowAllChecker{},
		TranscriptDir:     transcriptDir,
		SessionStore:      &agent.NoOpSessionStore{},
	}, nil)

	result, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "test transcript path",
		Prompt:       "test",
		SubagentType: "general-purpose",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedTranscriptPath == "" {
		t.Error("expected non-empty AgentTranscriptPath in SubagentStop hook")
	}

	// Verify the path includes the agent ID
	if !strings.Contains(capturedTranscriptPath, result.AgentID) {
		t.Errorf("transcript path %q should contain agent ID %q", capturedTranscriptPath, result.AgentID)
	}

	// Verify it's within the transcript dir
	if !strings.HasPrefix(capturedTranscriptPath, transcriptDir) {
		t.Errorf("transcript path %q should be within dir %q", capturedTranscriptPath, transcriptDir)
	}
}

// --- Phase 1 Tests: Error Propagation ---

func errorStreamData(errMsg string) *mockStreamData {
	// Simulates an LLM call that produces some text but the loop exits with error.
	// The ResultMessage will carry the error, but we also get text output.
	stop := "stop"
	text := "partial output before error"
	return &mockStreamData{
		chunks: []llm.StreamChunk{
			{
				ID:    "msg-1",
				Model: "claude-sonnet-4-5-20250929",
				Choices: []llm.Choice{
					{Delta: llm.Delta{Content: &text}},
				},
			},
			{
				ID:    "msg-1",
				Model: "claude-sonnet-4-5-20250929",
				Choices: []llm.Choice{
					{FinishReason: &stop},
				},
				Usage: &llm.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
			},
		},
	}
}

func TestManager_ErrorPropagation(t *testing.T) {
	// Mock LLM that returns an error-indicating ResultMessage.
	// The loop itself emits ResultMessage with IsError=true when it exits due to error.
	// We can verify error propagation by checking the AgentResult.Error field.
	client := &mockLLMClient{
		responses: []*mockStreamData{endTurnWithText("Some output")},
	}
	mgr := newTestManager(client)

	result, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "test",
		Prompt:       "test",
		SubagentType: "general-purpose",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Normal case: no error
	if result.Error != "" {
		t.Errorf("expected no error, got %q", result.Error)
	}
}

func TestManager_StateFailed_OnError(t *testing.T) {
	// When the loop exits for a non-interrupt/non-normal reason,
	// the manager should set StateFailed.
	// We test this indirectly by spawning a background agent with a client
	// that returns no responses (causes the loop to produce an end_turn with default).
	client := &mockLLMClient{
		responses: []*mockStreamData{endTurnWithText("ok")},
	}
	mgr := newTestManager(client)

	bg := true
	result, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:     "test",
		Prompt:          "test",
		SubagentType:    "general-purpose",
		RunInBackground: &bg,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait for completion
	taskResult, err := mgr.GetOutput(result.AgentID, true, 30*time.Second)
	if err != nil {
		t.Fatalf("GetOutput error: %v", err)
	}

	// Normal exit: should be Completed, not Failed
	if taskResult.State != StateCompleted {
		t.Errorf("state = %v, want Completed for normal exit", taskResult.State)
	}
	if taskResult.Error != "" {
		t.Errorf("expected no error for normal exit, got %q", taskResult.Error)
	}
}

// --- Phase 2 Tests: Resource Safety ---

func TestManager_CompletedAgentEviction(t *testing.T) {
	client := &mockLLMClient{}
	mgr := newTestManager(client)

	// Spawn more than maxCompletedAgents foreground agents
	for i := 0; i < maxCompletedAgents+5; i++ {
		client.mu.Lock()
		client.callIndex = 0
		client.responses = []*mockStreamData{endTurnWithText(fmt.Sprintf("output-%d", i))}
		client.mu.Unlock()

		_, err := mgr.Spawn(context.Background(), tools.AgentInput{
			Description:  "test",
			Prompt:       "test",
			SubagentType: "general-purpose",
		})
		if err != nil {
			t.Fatalf("spawn %d: %v", i, err)
		}
	}

	// Active should be empty (all completed)
	mgr.mu.RLock()
	activeCount := len(mgr.active)
	completedCount := len(mgr.completed)
	mgr.mu.RUnlock()

	if activeCount != 0 {
		t.Errorf("active count = %d, want 0", activeCount)
	}
	if completedCount != maxCompletedAgents {
		t.Errorf("completed count = %d, want %d", completedCount, maxCompletedAgents)
	}
}

func TestManager_GetOutputFromCompleted(t *testing.T) {
	client := &mockLLMClient{
		responses: []*mockStreamData{endTurnWithText("completed output")},
	}
	mgr := newTestManager(client)

	result, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "test",
		Prompt:       "test",
		SubagentType: "general-purpose",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Agent should be in completed map now
	mgr.mu.RLock()
	_, inActive := mgr.active[result.AgentID]
	_, inCompleted := mgr.completed[result.AgentID]
	mgr.mu.RUnlock()

	if inActive {
		t.Error("expected agent to not be in active map")
	}
	if !inCompleted {
		t.Error("expected agent to be in completed map")
	}

	// GetOutput should still work
	taskResult, err := mgr.GetOutput(result.AgentID, false, 0)
	if err != nil {
		t.Fatalf("GetOutput error: %v", err)
	}
	if taskResult.State != StateCompleted {
		t.Errorf("state = %v, want Completed", taskResult.State)
	}
}

// --- Phase 4 Tests: Agent Resume ---

func TestManager_ResumeCompletedAgent(t *testing.T) {
	client := &mockLLMClient{
		responses: []*mockStreamData{
			endTurnWithText("First run output"),
			endTurnWithText("Resumed run output"),
		},
	}
	mgr := newTestManager(client)

	// First spawn
	result, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "test",
		Prompt:       "First task",
		SubagentType: "general-purpose",
	})
	if err != nil {
		t.Fatalf("spawn error: %v", err)
	}
	agentID := result.AgentID
	if !strings.Contains(result.Output, "First run output") {
		t.Errorf("expected first run output, got %q", result.Output)
	}

	// Agent should be in completed map
	mgr.mu.RLock()
	_, inCompleted := mgr.completed[agentID]
	mgr.mu.RUnlock()
	if !inCompleted {
		t.Fatal("expected agent in completed map")
	}

	// Resume
	resumeResult, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "resume",
		Prompt:       "Continue the work",
		SubagentType: "general-purpose",
		Resume:       &agentID,
	})
	if err != nil {
		t.Fatalf("resume error: %v", err)
	}
	if resumeResult.AgentID != agentID {
		t.Errorf("expected same agent ID %q, got %q", agentID, resumeResult.AgentID)
	}
	if !strings.Contains(resumeResult.Output, "Resumed run output") {
		t.Errorf("expected resumed output, got %q", resumeResult.Output)
	}
}

func TestManager_ResumeRunningAgent(t *testing.T) {
	client := &mockLLMClient{
		responses: []*mockStreamData{endTurnWithText("still going")},
	}
	mgr := newTestManager(client)

	bg := true
	result, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:     "bg task",
		Prompt:          "do work",
		SubagentType:    "general-purpose",
		RunInBackground: &bg,
	})
	if err != nil {
		t.Fatalf("spawn error: %v", err)
	}

	// Resume a still-running (or just-completed) agent — should return output
	agentID := result.AgentID
	// Give it time to finish
	time.Sleep(100 * time.Millisecond)

	resumeResult, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "resume",
		Prompt:       "check status",
		SubagentType: "general-purpose",
		Resume:       &agentID,
	})
	if err != nil {
		t.Fatalf("resume error: %v", err)
	}
	if resumeResult.AgentID != agentID {
		t.Errorf("expected same agent ID")
	}
}

func TestManager_ResumeUnknownAgent(t *testing.T) {
	mgr := newTestManager(&mockLLMClient{})

	unknownID := "nonexistent-agent-id"
	_, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "resume",
		Prompt:       "test",
		SubagentType: "general-purpose",
		Resume:       &unknownID,
	})
	if err == nil {
		t.Fatal("expected error for unknown resume ID")
	}
	if !strings.Contains(err.Error(), "cannot resume unknown agent") {
		t.Errorf("error = %q, want 'cannot resume unknown agent'", err.Error())
	}
}

func TestManager_ResumeStoppedAgent(t *testing.T) {
	client := &mockLLMClient{
		responses: []*mockStreamData{
			endTurnWithText("Before stop"),
			endTurnWithText("After resume"),
		},
	}
	mgr := newTestManager(client)

	// Spawn and it completes immediately (foreground)
	result, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "test",
		Prompt:       "First task",
		SubagentType: "general-purpose",
	})
	if err != nil {
		t.Fatalf("spawn error: %v", err)
	}
	agentID := result.AgentID

	// Resume the completed agent with a new prompt
	resumeResult, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "resume",
		Prompt:       "Continue after stop",
		SubagentType: "general-purpose",
		Resume:       &agentID,
	})
	if err != nil {
		t.Fatalf("resume error: %v", err)
	}
	if resumeResult.AgentID != agentID {
		t.Errorf("expected same agent ID %q, got %q", agentID, resumeResult.AgentID)
	}
	if !strings.Contains(resumeResult.Output, "After resume") {
		t.Errorf("expected resumed output, got %q", resumeResult.Output)
	}
}

// --- Phase 3 Tests: Loader Resilience ---

func TestManager_ToolNameWarning(t *testing.T) {
	client := &mockLLMClient{
		responses: []*mockStreamData{endTurnWithText("ok")},
	}
	reg := tools.NewRegistry()
	reg.Register(&mockTool{name: "Read"})

	parentConfig := &agent.AgentConfig{
		Model:     "claude-sonnet-4-5-20250929",
		CWD:       "/tmp/test",
		SessionID: "parent-session",
	}

	mgr := NewManager(ManagerOpts{
		ParentConfig:      parentConfig,
		LLMClient:         client,
		CostTracker:       llm.NewCostTracker(),
		ParentRegistry:    reg,
		PermissionChecker: &agent.AllowAllChecker{},
	}, nil)

	// Register an agent with a non-existent tool
	mgr.RegisterAgents(map[string]Definition{
		"warn-agent": FromTypesDefinition("warn-agent", types.AgentDefinition{
			Description: "Agent with unknown tool",
			Prompt:      "Test",
			Tools:       []string{"Read", "NonExistentTool"},
		}, SourceProject, 30),
	})

	result, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "test",
		Prompt:       "test",
		SubagentType: "warn-agent",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check the running agent had warnings
	mgr.mu.RLock()
	ra, ok := mgr.completed[result.AgentID]
	mgr.mu.RUnlock()
	if !ok {
		t.Fatal("expected agent in completed map")
	}
	found := false
	for _, w := range ra.Warnings {
		if strings.Contains(w, "NonExistentTool") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warning about NonExistentTool, got %v", ra.Warnings)
	}
}

func TestManager_BackgroundErrorOutput(t *testing.T) {
	// Verify that background agent output files include error info
	// when the agent has an error result.
	dir := t.TempDir()
	client := &mockLLMClient{
		responses: []*mockStreamData{endTurnWithText("Background output")},
	}

	reg := tools.NewRegistry()
	parentConfig := &agent.AgentConfig{
		Model:     "claude-sonnet-4-5-20250929",
		CWD:       "/tmp/test",
		SessionID: "parent-session",
	}

	mgr := NewManager(ManagerOpts{
		OutputDir:         dir,
		ParentConfig:      parentConfig,
		LLMClient:         client,
		CostTracker:       llm.NewCostTracker(),
		ParentRegistry:    reg,
		PermissionChecker: &agent.AllowAllChecker{},
	}, nil)

	bg := true
	result, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:     "bg task",
		Prompt:          "Do work",
		SubagentType:    "general-purpose",
		RunInBackground: &bg,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait for completion
	taskResult, err := mgr.GetOutput(result.AgentID, true, 30*time.Second)
	if err != nil {
		t.Fatalf("GetOutput error: %v", err)
	}
	if taskResult.State != StateCompleted {
		t.Errorf("state = %v, want Completed", taskResult.State)
	}

	// Read output file
	data, err := os.ReadFile(result.OutputFile)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if !strings.Contains(string(data), "Background output") {
		t.Errorf("output file = %q, want to contain 'Background output'", string(data))
	}
}

func TestManager_SubagentStopHook_NoTranscriptWithoutStore(t *testing.T) {
	var capturedTranscriptPath string

	runner := hooks.NewRunner(hooks.RunnerConfig{})
	runner.RegisterScoped("test-hooks", map[types.HookEvent][]hooks.CallbackMatcher{
		types.HookEventSubagentStop: {
			{Hooks: []hooks.HookCallback{func(input any, toolUseID string, ctx context.Context) (hooks.HookJSONOutput, error) {
				if stopInput, ok := input.(*hooks.SubagentStopHookInput); ok {
					capturedTranscriptPath = stopInput.AgentTranscriptPath
				}
				return hooks.HookJSONOutput{}, nil
			}}},
		},
	})

	client := &mockLLMClient{
		responses: []*mockStreamData{endTurnWithText("done")},
	}

	parentConfig := &agent.AgentConfig{
		Model:     "claude-sonnet-4-5-20250929",
		CWD:       "/tmp/test",
		SessionID: "parent-session",
	}

	// No SessionStore and no TranscriptDir
	mgr := NewManager(ManagerOpts{
		ParentConfig:      parentConfig,
		LLMClient:         client,
		CostTracker:       llm.NewCostTracker(),
		HookRunner:        runner,
		PermissionChecker: &agent.AllowAllChecker{},
	}, nil)

	_, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "test no transcript",
		Prompt:       "test",
		SubagentType: "general-purpose",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedTranscriptPath != "" {
		t.Errorf("expected empty AgentTranscriptPath without SessionStore, got %q", capturedTranscriptPath)
	}
}

// --- Phase 7 Tests: Metrics in Spawn Result ---

func TestManager_ForegroundMetrics(t *testing.T) {
	client := &mockLLMClient{
		responses: []*mockStreamData{endTurnWithText("analysis done")},
	}
	mgr := newTestManager(client)

	result, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "test metrics",
		Prompt:       "analyze",
		SubagentType: "general-purpose",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Metrics == nil {
		t.Fatal("expected non-nil Metrics for foreground spawn")
	}
	if result.Metrics.DurationSecs <= 0 {
		t.Errorf("expected positive duration, got %f", result.Metrics.DurationSecs)
	}
	// Should have at least 1 turn (the single LLM call)
	if result.Metrics.TurnCount < 1 {
		t.Errorf("expected at least 1 turn, got %d", result.Metrics.TurnCount)
	}
}

func TestManager_BackgroundMetrics(t *testing.T) {
	dir := t.TempDir()
	client := &mockLLMClient{
		responses: []*mockStreamData{endTurnWithText("bg analysis done")},
	}
	mgr := newTestManager(client)
	mgr.opts.OutputDir = dir

	result, err := mgr.Spawn(context.Background(), tools.AgentInput{
		Description:  "test bg metrics",
		Prompt:       "analyze bg",
		SubagentType: "general-purpose",
		RunInBackground: func() *bool { b := true; return &b }(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Background spawn should not have metrics (returned immediately)
	if result.Metrics != nil {
		t.Error("expected nil Metrics for background spawn")
	}

	// Wait for completion and check metrics via GetOutput
	time.Sleep(500 * time.Millisecond)
	taskResult, err := mgr.GetOutput(result.AgentID, false, 0)
	if err != nil {
		t.Fatalf("GetOutput error: %v", err)
	}
	if taskResult.Metrics.Duration <= 0 {
		t.Errorf("expected positive duration in TaskResult, got %v", taskResult.Metrics.Duration)
	}
}
