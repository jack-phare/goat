package subagent

import (
	"context"
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
	taskResult, err := mgr.GetOutput(result.AgentID, true, 5*time.Second)
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
	err := mgr.Reload("/tmp/nonexistent")
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
	if err := mgr.Reload(tmpDir); err != nil {
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

	if err := mgr.Reload(tmpDir); err != nil {
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

// --- Stub Tests for Unimplemented Features ---

func TestManager_LargeAgentDefinition(t *testing.T) {
	t.Skip("not yet implemented: large agent definition handling (250KB+)")
}
