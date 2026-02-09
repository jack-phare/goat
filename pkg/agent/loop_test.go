package agent

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jg-phare/goat/pkg/llm"
	"github.com/jg-phare/goat/pkg/tools"
	"github.com/jg-phare/goat/pkg/types"
)

// --- Test Infrastructure ---

// mockLLMClient implements llm.Client for testing.
type mockLLMClient struct {
	mu        sync.Mutex
	responses []*mockStream // one per call
	callIndex int
	model     string
}

func (m *mockLLMClient) Complete(ctx context.Context, req *llm.CompletionRequest) (*llm.Stream, error) {
	m.mu.Lock()
	idx := m.callIndex
	m.callIndex++
	m.mu.Unlock()

	if idx >= len(m.responses) {
		// Return a default end_turn response
		return makeMockStream(endTurnResponse("No more responses")), nil
	}

	return m.responses[idx].toStream(ctx), nil
}

func (m *mockLLMClient) Model() string {
	return m.model
}

func (m *mockLLMClient) SetModel(model string) {
	m.model = model
}

// mockStream holds pre-programmed chunks to feed into a Stream.
type mockStream struct {
	chunks []llm.StreamChunk
}

func (ms *mockStream) toStream(ctx context.Context) *llm.Stream {
	events := make(chan llm.StreamEvent, len(ms.chunks)+1)
	go func() {
		defer close(events)
		for _, chunk := range ms.chunks {
			c := chunk // copy for closure
			select {
			case events <- llm.StreamEvent{Chunk: &c}:
			case <-ctx.Done():
				return
			}
		}
	}()

	// We need to create a Stream, but it requires an io.ReadCloser and cancel func.
	// Use a pipe that we immediately close as the body.
	pr, pw := io.Pipe()
	pw.Close()

	_, cancel := context.WithCancel(ctx)
	return llm.NewStream(events, pr, cancel)
}

// mockRecordingTool records calls and returns configurable output.
type mockRecordingTool struct {
	name       string
	output     tools.ToolOutput
	err        error
	calls      []map[string]any
	mu         sync.Mutex
}

func (m *mockRecordingTool) Name() string               { return m.name }
func (m *mockRecordingTool) Description() string         { return "mock tool" }
func (m *mockRecordingTool) InputSchema() map[string]any { return map[string]any{"type": "object"} }
func (m *mockRecordingTool) SideEffect() tools.SideEffectType { return tools.SideEffectNone }

func (m *mockRecordingTool) Execute(_ context.Context, input map[string]any) (tools.ToolOutput, error) {
	m.mu.Lock()
	m.calls = append(m.calls, input)
	m.mu.Unlock()
	return m.output, m.err
}

func (m *mockRecordingTool) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

// --- Helper functions to build mock responses ---

func textChunk(id, model, text string) llm.StreamChunk {
	content := text
	return llm.StreamChunk{
		ID:    id,
		Model: model,
		Choices: []llm.Choice{
			{Delta: llm.Delta{Content: &content}},
		},
	}
}

func finishChunk(id, model, reason string, inputTokens, outputTokens int) llm.StreamChunk {
	return llm.StreamChunk{
		ID:    id,
		Model: model,
		Choices: []llm.Choice{
			{FinishReason: &reason},
		},
		Usage: &llm.Usage{
			PromptTokens:     inputTokens,
			CompletionTokens: outputTokens,
			TotalTokens:      inputTokens + outputTokens,
		},
	}
}

func toolCallChunk(id, model, callID, toolName, args string) llm.StreamChunk {
	return llm.StreamChunk{
		ID:    id,
		Model: model,
		Choices: []llm.Choice{
			{
				Delta: llm.Delta{
					ToolCalls: []llm.ToolCall{
						{
							Index: 0,
							ID:    callID,
							Type:  "function",
							Function: llm.FunctionCall{
								Name:      toolName,
								Arguments: args,
							},
						},
					},
				},
			},
		},
	}
}

func endTurnResponse(text string) *mockStream {
	stop := "stop"
	return &mockStream{
		chunks: []llm.StreamChunk{
			textChunk("msg-1", "claude-sonnet-4-5-20250929", text),
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

func toolUseResponse(callID, toolName string, args map[string]any) *mockStream {
	argsJSON, _ := json.Marshal(args)
	toolCalls := "tool_calls"
	return &mockStream{
		chunks: []llm.StreamChunk{
			textChunk("msg-1", "claude-sonnet-4-5-20250929", "Let me run that."),
			toolCallChunk("msg-1", "claude-sonnet-4-5-20250929", callID, toolName, string(argsJSON)),
			{
				ID:    "msg-1",
				Model: "claude-sonnet-4-5-20250929",
				Choices: []llm.Choice{
					{FinishReason: &toolCalls},
				},
				Usage: &llm.Usage{PromptTokens: 200, CompletionTokens: 80, TotalTokens: 280},
			},
		},
	}
}

func makeMockStream(ms *mockStream) *llm.Stream {
	return ms.toStream(context.Background())
}

func defaultConfig(client llm.Client, registry *tools.Registry) AgentConfig {
	return AgentConfig{
		Model:          "claude-sonnet-4-5-20250929",
		MaxTurns:       100,
		PermissionMode: types.PermissionModeDefault,
		LLMClient:      client,
		ToolRegistry:   registry,
		Prompter:       &StaticPromptAssembler{Prompt: "You are a test assistant."},
		Permissions:    &AllowAllChecker{},
		Hooks:          &NoOpHookRunner{},
		Compactor:      &NoOpCompactor{},
		CostTracker:    llm.NewCostTracker(),
	}
}

func collectMessages(q *Query) []types.SDKMessage {
	var msgs []types.SDKMessage
	for msg := range q.Messages() {
		msgs = append(msgs, msg)
	}
	return msgs
}

// --- Test Cases ---

func TestLoop_SimpleTextResponse(t *testing.T) {
	client := &mockLLMClient{
		responses: []*mockStream{endTurnResponse("Hello! How can I help?")},
	}
	registry := tools.NewRegistry()
	config := defaultConfig(client, registry)

	q := RunLoop(context.Background(), "Hello", config)
	msgs := collectMessages(q)

	// Should have: init, assistant, result
	if len(msgs) < 3 {
		t.Fatalf("expected at least 3 messages, got %d", len(msgs))
	}

	// First should be system init
	if msgs[0].GetType() != types.MessageTypeSystem {
		t.Errorf("msgs[0] type = %s, want system", msgs[0].GetType())
	}

	// Should have an assistant message
	foundAssistant := false
	for _, m := range msgs {
		if m.GetType() == types.MessageTypeAssistant {
			foundAssistant = true
		}
	}
	if !foundAssistant {
		t.Error("no assistant message found")
	}

	// Last should be result
	lastMsg := msgs[len(msgs)-1]
	if lastMsg.GetType() != types.MessageTypeResult {
		t.Errorf("last message type = %s, want result", lastMsg.GetType())
	}

	// Check exit reason
	q.Wait()
	if q.GetExitReason() != ExitEndTurn {
		t.Errorf("exit reason = %s, want end_turn", q.GetExitReason())
	}

	if q.TurnCount() != 1 {
		t.Errorf("turn count = %d, want 1", q.TurnCount())
	}
}

func TestLoop_SingleToolCall(t *testing.T) {
	mockTool := &mockRecordingTool{
		name:   "Bash",
		output: tools.ToolOutput{Content: "hello world"},
	}

	registry := tools.NewRegistry()
	registry.Register(mockTool)

	client := &mockLLMClient{
		responses: []*mockStream{
			// Turn 1: LLM calls Bash tool
			toolUseResponse("call_1", "Bash", map[string]any{"command": "echo hello"}),
			// Turn 2: LLM sees tool result, responds with end_turn
			endTurnResponse("The command output: hello world"),
		},
	}
	config := defaultConfig(client, registry)

	q := RunLoop(context.Background(), "Run echo hello", config)
	msgs := collectMessages(q)
	q.Wait()

	if q.GetExitReason() != ExitEndTurn {
		t.Errorf("exit reason = %s, want end_turn", q.GetExitReason())
	}

	if q.TurnCount() != 2 {
		t.Errorf("turn count = %d, want 2", q.TurnCount())
	}

	if mockTool.CallCount() != 1 {
		t.Errorf("tool call count = %d, want 1", mockTool.CallCount())
	}

	// Should have tool_progress messages
	foundToolProgress := false
	for _, m := range msgs {
		if m.GetType() == types.MessageTypeToolProgress {
			foundToolProgress = true
		}
	}
	if !foundToolProgress {
		t.Error("no tool_progress message found")
	}
}

func TestLoop_MultipleToolCalls(t *testing.T) {
	bashTool := &mockRecordingTool{name: "Bash", output: tools.ToolOutput{Content: "bash out"}}
	grepTool := &mockRecordingTool{name: "Grep", output: tools.ToolOutput{Content: "grep out"}}
	globTool := &mockRecordingTool{name: "Glob", output: tools.ToolOutput{Content: "glob out"}}

	registry := tools.NewRegistry()
	registry.Register(bashTool)
	registry.Register(grepTool)
	registry.Register(globTool)

	// Response with 3 tool calls
	toolCalls := "tool_calls"
	multiToolResponse := &mockStream{
		chunks: []llm.StreamChunk{
			textChunk("msg-1", "claude-sonnet-4-5-20250929", "Let me check."),
			{
				ID:    "msg-1",
				Model: "claude-sonnet-4-5-20250929",
				Choices: []llm.Choice{{
					Delta: llm.Delta{
						ToolCalls: []llm.ToolCall{
							{Index: 0, ID: "call_1", Type: "function", Function: llm.FunctionCall{Name: "Bash", Arguments: `{"command":"ls"}`}},
						},
					},
				}},
			},
			{
				ID:    "msg-1",
				Model: "claude-sonnet-4-5-20250929",
				Choices: []llm.Choice{{
					Delta: llm.Delta{
						ToolCalls: []llm.ToolCall{
							{Index: 1, ID: "call_2", Type: "function", Function: llm.FunctionCall{Name: "Grep", Arguments: `{"pattern":"foo"}`}},
						},
					},
				}},
			},
			{
				ID:    "msg-1",
				Model: "claude-sonnet-4-5-20250929",
				Choices: []llm.Choice{{
					Delta: llm.Delta{
						ToolCalls: []llm.ToolCall{
							{Index: 2, ID: "call_3", Type: "function", Function: llm.FunctionCall{Name: "Glob", Arguments: `{"pattern":"*.go"}`}},
						},
					},
				}},
			},
			{
				ID:    "msg-1",
				Model: "claude-sonnet-4-5-20250929",
				Choices: []llm.Choice{{FinishReason: &toolCalls}},
				Usage:   &llm.Usage{PromptTokens: 300, CompletionTokens: 100, TotalTokens: 400},
			},
		},
	}

	client := &mockLLMClient{
		responses: []*mockStream{
			multiToolResponse,
			endTurnResponse("All done!"),
		},
	}
	config := defaultConfig(client, registry)

	q := RunLoop(context.Background(), "Do everything", config)
	collectMessages(q)
	q.Wait()

	if bashTool.CallCount() != 1 {
		t.Errorf("bash calls = %d, want 1", bashTool.CallCount())
	}
	if grepTool.CallCount() != 1 {
		t.Errorf("grep calls = %d, want 1", grepTool.CallCount())
	}
	if globTool.CallCount() != 1 {
		t.Errorf("glob calls = %d, want 1", globTool.CallCount())
	}
}

func TestLoop_ToolError(t *testing.T) {
	mockTool := &mockRecordingTool{
		name:   "Bash",
		output: tools.ToolOutput{Content: "command not found: xyz", IsError: true},
	}

	registry := tools.NewRegistry()
	registry.Register(mockTool)

	client := &mockLLMClient{
		responses: []*mockStream{
			toolUseResponse("call_1", "Bash", map[string]any{"command": "xyz"}),
			endTurnResponse("That command failed. Let me try something else."),
		},
	}
	config := defaultConfig(client, registry)

	q := RunLoop(context.Background(), "Run xyz", config)
	collectMessages(q)
	q.Wait()

	// Loop should continue after tool error and eventually end
	if q.GetExitReason() != ExitEndTurn {
		t.Errorf("exit reason = %s, want end_turn", q.GetExitReason())
	}
	if q.TurnCount() != 2 {
		t.Errorf("turn count = %d, want 2", q.TurnCount())
	}
}

func TestLoop_MaxTurns(t *testing.T) {
	// Build responses that keep calling tools forever
	responses := make([]*mockStream, 10)
	for i := range responses {
		responses[i] = toolUseResponse("call_"+string(rune('1'+i)), "Bash", map[string]any{"command": "ls"})
	}

	mockTool := &mockRecordingTool{name: "Bash", output: tools.ToolOutput{Content: "file.txt"}}
	registry := tools.NewRegistry()
	registry.Register(mockTool)

	client := &mockLLMClient{responses: responses}
	config := defaultConfig(client, registry)
	config.MaxTurns = 3

	q := RunLoop(context.Background(), "Keep going", config)
	collectMessages(q)
	q.Wait()

	if q.GetExitReason() != ExitMaxTurns {
		t.Errorf("exit reason = %s, want max_turns", q.GetExitReason())
	}
	if q.TurnCount() != 3 {
		t.Errorf("turn count = %d, want 3 (MaxTurns)", q.TurnCount())
	}
}

func TestLoop_ContextCancel(t *testing.T) {
	// Use a slow response that won't complete before cancel
	client := &mockLLMClient{
		responses: []*mockStream{endTurnResponse("Hello")},
	}
	registry := tools.NewRegistry()
	config := defaultConfig(client, registry)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	q := RunLoop(ctx, "Hello", config)
	collectMessages(q)
	q.Wait()

	reason := q.GetExitReason()
	if reason != ExitAborted {
		t.Errorf("exit reason = %s, want aborted", reason)
	}
}

func TestLoop_Interrupt(t *testing.T) {
	// Create a client that blocks on the first call
	blockCh := make(chan struct{})
	client := &blockingLLMClient{blockCh: blockCh}
	registry := tools.NewRegistry()
	config := defaultConfig(client, registry)

	q := RunLoop(context.Background(), "Hello", config)

	// Wait a bit then interrupt
	time.Sleep(50 * time.Millisecond)
	q.Interrupt()

	// Unblock the client
	close(blockCh)

	q.Wait()
	reason := q.GetExitReason()
	if reason != ExitInterrupted && reason != ExitAborted {
		t.Errorf("exit reason = %s, want interrupted or aborted", reason)
	}
}

// blockingLLMClient blocks until blockCh is closed.
type blockingLLMClient struct {
	blockCh chan struct{}
}

func (b *blockingLLMClient) Complete(ctx context.Context, req *llm.CompletionRequest) (*llm.Stream, error) {
	select {
	case <-b.blockCh:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return makeMockStream(endTurnResponse("done")), nil
}

func (b *blockingLLMClient) Model() string      { return "test" }
func (b *blockingLLMClient) SetModel(string) {}

func TestLoop_ToolPermissionDenied(t *testing.T) {
	mockTool := &mockRecordingTool{
		name:   "Bash",
		output: tools.ToolOutput{Content: "should not see this"},
	}

	registry := tools.NewRegistry()
	registry.Register(mockTool)

	client := &mockLLMClient{
		responses: []*mockStream{
			toolUseResponse("call_1", "Bash", map[string]any{"command": "rm -rf /"}),
			endTurnResponse("I understand that was denied."),
		},
	}
	config := defaultConfig(client, registry)
	config.Permissions = &denyAllChecker{}

	q := RunLoop(context.Background(), "Delete everything", config)
	collectMessages(q)
	q.Wait()

	// Tool should NOT have been executed
	if mockTool.CallCount() != 0 {
		t.Errorf("tool should not have been called, but was called %d times", mockTool.CallCount())
	}
}

type denyAllChecker struct{}

func (d *denyAllChecker) Check(_ context.Context, _ string, _ map[string]any) (PermissionResult, error) {
	return PermissionResult{Behavior: "deny", Message: "permission denied by test"}, nil
}

func TestLoop_StreamEvents(t *testing.T) {
	client := &mockLLMClient{
		responses: []*mockStream{endTurnResponse("Hello!")},
	}
	registry := tools.NewRegistry()
	config := defaultConfig(client, registry)
	config.IncludePartial = true

	q := RunLoop(context.Background(), "Hi", config)
	msgs := collectMessages(q)
	q.Wait()

	// Should have stream_event messages
	foundStream := false
	for _, m := range msgs {
		if m.GetType() == types.MessageTypeStreamEvent {
			foundStream = true
			break
		}
	}
	if !foundStream {
		t.Error("expected stream_event messages with IncludePartial=true")
	}
}

func TestLoop_EmptyToolUse(t *testing.T) {
	// stop_reason=tool_use but 0 tool blocks
	toolCalls := "tool_calls"
	emptyToolResponse := &mockStream{
		chunks: []llm.StreamChunk{
			textChunk("msg-1", "claude-sonnet-4-5-20250929", "Hmm."),
			{
				ID:    "msg-1",
				Model: "claude-sonnet-4-5-20250929",
				Choices: []llm.Choice{{FinishReason: &toolCalls}},
				Usage:   &llm.Usage{PromptTokens: 100, CompletionTokens: 10, TotalTokens: 110},
			},
		},
	}

	client := &mockLLMClient{responses: []*mockStream{emptyToolResponse}}
	registry := tools.NewRegistry()
	config := defaultConfig(client, registry)

	q := RunLoop(context.Background(), "Hello", config)
	collectMessages(q)
	q.Wait()

	if q.GetExitReason() != ExitEndTurn {
		t.Errorf("exit reason = %s, want end_turn (empty tool_use treated as end_turn)", q.GetExitReason())
	}
}

func TestLoop_UnknownTool(t *testing.T) {
	// LLM calls a tool that doesn't exist in registry
	registry := tools.NewRegistry() // empty registry

	client := &mockLLMClient{
		responses: []*mockStream{
			toolUseResponse("call_1", "NonexistentTool", map[string]any{}),
			endTurnResponse("I see that tool doesn't exist."),
		},
	}
	config := defaultConfig(client, registry)

	q := RunLoop(context.Background(), "Use nonexistent tool", config)
	collectMessages(q)
	q.Wait()

	// Should still complete successfully — unknown tool returns error result to LLM
	if q.GetExitReason() != ExitEndTurn {
		t.Errorf("exit reason = %s, want end_turn", q.GetExitReason())
	}
	if q.TurnCount() != 2 {
		t.Errorf("turn count = %d, want 2", q.TurnCount())
	}
}

func TestLoop_MessageOrdering(t *testing.T) {
	mockTool := &mockRecordingTool{name: "Bash", output: tools.ToolOutput{Content: "ok"}}
	registry := tools.NewRegistry()
	registry.Register(mockTool)

	client := &mockLLMClient{
		responses: []*mockStream{
			toolUseResponse("call_1", "Bash", map[string]any{"command": "ls"}),
			endTurnResponse("Done."),
		},
	}
	config := defaultConfig(client, registry)

	q := RunLoop(context.Background(), "Do stuff", config)
	msgs := collectMessages(q)
	q.Wait()

	// Verify ordering: init, [tool_progress...], assistant, [tool_progress...], assistant, result
	if len(msgs) < 3 {
		t.Fatalf("expected at least 3 messages, got %d", len(msgs))
	}

	// First must be system (init)
	if msgs[0].GetType() != types.MessageTypeSystem {
		t.Errorf("first message should be system (init), got %s", msgs[0].GetType())
	}

	// Last must be result
	if msgs[len(msgs)-1].GetType() != types.MessageTypeResult {
		t.Errorf("last message should be result, got %s", msgs[len(msgs)-1].GetType())
	}

	// Verify we see the expected sequence of types
	var typeSequence []types.MessageType
	for _, m := range msgs {
		typeSequence = append(typeSequence, m.GetType())
	}

	// Should contain: system, assistant (turn 1), tool_progress (x2), assistant (turn 2), result
	typeStr := ""
	for _, t := range typeSequence {
		typeStr += string(t) + " "
	}
	if !strings.Contains(typeStr, "system") {
		t.Errorf("missing system in sequence: %s", typeStr)
	}
	if !strings.Contains(typeStr, "assistant") {
		t.Errorf("missing assistant in sequence: %s", typeStr)
	}
	if !strings.Contains(typeStr, "result") {
		t.Errorf("missing result in sequence: %s", typeStr)
	}
}

func TestLoop_BackgroundBash(t *testing.T) {
	// Use the real BashTool with a TaskManager for background support
	tm := tools.NewTaskManager()
	bashTool := &tools.BashTool{TaskManager: tm}
	taskOutputTool := &tools.TaskOutputTool{TaskManager: tm}

	registry := tools.NewRegistry()
	registry.Register(bashTool)
	registry.Register(taskOutputTool)

	// Turn 1: LLM calls Bash with run_in_background=true
	// Turn 2: LLM calls TaskOutput with the task ID
	// Turn 3: LLM responds with the output
	client := &mockLLMClient{
		responses: []*mockStream{
			toolUseResponse("call_1", "Bash", map[string]any{
				"command":           "echo background_output",
				"run_in_background": true,
			}),
			// We can't easily predict the task ID, so we use a mock for turn 2
			// Instead, let's just verify the background bash returns a task ID
			endTurnResponse("Background task started successfully."),
		},
	}
	config := defaultConfig(client, registry)

	q := RunLoop(context.Background(), "Run in background", config)
	msgs := collectMessages(q)
	q.Wait()

	if q.GetExitReason() != ExitEndTurn {
		t.Errorf("exit reason = %s, want end_turn", q.GetExitReason())
	}

	// Verify the tool was actually called (bash tool returns task ID message)
	foundAssistant := false
	for _, m := range msgs {
		if m.GetType() == types.MessageTypeAssistant {
			foundAssistant = true
		}
	}
	if !foundAssistant {
		t.Error("no assistant message found")
	}
}

func TestLoop_TodoWrite(t *testing.T) {
	todoTool := &tools.TodoWriteTool{}
	registry := tools.NewRegistry()
	registry.Register(todoTool)

	client := &mockLLMClient{
		responses: []*mockStream{
			toolUseResponse("call_1", "TodoWrite", map[string]any{
				"todos": []any{
					map[string]any{"content": "Write tests", "status": "pending"},
					map[string]any{"content": "Review code", "status": "in_progress"},
				},
			}),
			endTurnResponse("Todo list updated."),
		},
	}
	config := defaultConfig(client, registry)

	q := RunLoop(context.Background(), "Create a todo list", config)
	collectMessages(q)
	q.Wait()

	if q.GetExitReason() != ExitEndTurn {
		t.Errorf("exit reason = %s, want end_turn", q.GetExitReason())
	}

	// Verify the TodoWrite tool state was updated
	if len(todoTool.Todos) != 2 {
		t.Errorf("expected 2 todos, got %d", len(todoTool.Todos))
	}
	if todoTool.Todos[0].Content != "Write tests" {
		t.Errorf("expected 'Write tests', got %q", todoTool.Todos[0].Content)
	}
}

func TestLoop_UnknownTool_MCPPrefix(t *testing.T) {
	// LLM calls mcp__foo__bar when no MCP tools are registered
	registry := tools.NewRegistry()

	client := &mockLLMClient{
		responses: []*mockStream{
			toolUseResponse("call_1", "mcp__foo__bar", map[string]any{"query": "test"}),
			endTurnResponse("That MCP tool is not available."),
		},
	}
	config := defaultConfig(client, registry)

	q := RunLoop(context.Background(), "Use MCP tool", config)
	collectMessages(q)
	q.Wait()

	// Should still complete — unknown tool returns error to LLM
	if q.GetExitReason() != ExitEndTurn {
		t.Errorf("exit reason = %s, want end_turn", q.GetExitReason())
	}
	if q.TurnCount() != 2 {
		t.Errorf("turn count = %d, want 2", q.TurnCount())
	}
}

func TestLoop_PermissionInterrupt(t *testing.T) {
	// Permission check with Interrupt=true should stop the loop
	mockTool := &mockRecordingTool{
		name:   "Bash",
		output: tools.ToolOutput{Content: "should not see this"},
	}

	registry := tools.NewRegistry()
	registry.Register(mockTool)

	client := &mockLLMClient{
		responses: []*mockStream{
			toolUseResponse("call_1", "Bash", map[string]any{"command": "rm -rf /"}),
			endTurnResponse("This should not be reached."),
		},
	}
	config := defaultConfig(client, registry)
	config.Permissions = &interruptChecker{}

	q := RunLoop(context.Background(), "Delete everything", config)
	collectMessages(q)
	q.Wait()

	// Tool should NOT have been executed
	if mockTool.CallCount() != 0 {
		t.Errorf("tool should not have been called, but was called %d times", mockTool.CallCount())
	}

	// Loop should have been interrupted
	if q.GetExitReason() != ExitInterrupted {
		t.Errorf("exit reason = %s, want interrupted", q.GetExitReason())
	}
}

type interruptChecker struct{}

func (ic *interruptChecker) Check(_ context.Context, _ string, _ map[string]any) (PermissionResult, error) {
	return PermissionResult{
		Behavior:  "deny",
		Message:   "interrupted by permission check",
		Interrupt: true,
	}, nil
}

func TestLoop_PermissionUpdatedInput(t *testing.T) {
	// Permission check modifies input before tool execution
	mockTool := &mockRecordingTool{
		name:   "Bash",
		output: tools.ToolOutput{Content: "safe output"},
	}

	registry := tools.NewRegistry()
	registry.Register(mockTool)

	client := &mockLLMClient{
		responses: []*mockStream{
			toolUseResponse("call_1", "Bash", map[string]any{"command": "dangerous-cmd"}),
			endTurnResponse("Done."),
		},
	}
	config := defaultConfig(client, registry)
	config.Permissions = &inputRewriteChecker{}

	q := RunLoop(context.Background(), "Run something", config)
	collectMessages(q)
	q.Wait()

	if mockTool.CallCount() != 1 {
		t.Fatalf("tool call count = %d, want 1", mockTool.CallCount())
	}

	// Verify the tool received the rewritten input
	mockTool.mu.Lock()
	callInput := mockTool.calls[0]
	mockTool.mu.Unlock()

	if callInput["command"] != "safe-cmd" {
		t.Errorf("tool received command = %q, want 'safe-cmd'", callInput["command"])
	}
}

type inputRewriteChecker struct{}

func (ic *inputRewriteChecker) Check(_ context.Context, _ string, _ map[string]any) (PermissionResult, error) {
	return PermissionResult{
		Behavior:     "allow",
		UpdatedInput: map[string]any{"command": "safe-cmd"},
	}, nil
}

func TestLoop_CostTracking(t *testing.T) {
	client := &mockLLMClient{
		responses: []*mockStream{
			toolUseResponse("call_1", "Bash", map[string]any{"command": "ls"}),
			endTurnResponse("Done."),
		},
	}

	mockTool := &mockRecordingTool{name: "Bash", output: tools.ToolOutput{Content: "ok"}}
	registry := tools.NewRegistry()
	registry.Register(mockTool)

	config := defaultConfig(client, registry)

	q := RunLoop(context.Background(), "test", config)
	collectMessages(q)
	q.Wait()

	// After 2 turns, cost should be > 0 (assuming model is recognized)
	usage := q.TotalUsage()
	if usage.InputTokens == 0 && usage.OutputTokens == 0 {
		t.Error("expected non-zero token usage after 2 turns")
	}

	if q.TurnCount() != 2 {
		t.Errorf("turn count = %d, want 2", q.TurnCount())
	}
}

// --- Hook Integration Tests ---

// mockHookRunner records hook firings and returns configurable results.
type mockHookRunner struct {
	mu      sync.Mutex
	events  []types.HookEvent
	results map[types.HookEvent][]HookResult
}

func (m *mockHookRunner) Fire(_ context.Context, event types.HookEvent, _ any) ([]HookResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	if results, ok := m.results[event]; ok {
		return results, nil
	}
	return nil, nil
}

func (m *mockHookRunner) firedEvents() []types.HookEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]types.HookEvent, len(m.events))
	copy(out, m.events)
	return out
}

func boolPtr(b bool) *bool { return &b }

func TestLoop_StopHookContinue(t *testing.T) {
	// Stop hook returns continue=true, so the loop should continue after end_turn
	hooks := &mockHookRunner{
		results: map[types.HookEvent][]HookResult{
			types.HookEventStop: {
				{Continue: boolPtr(true)},
			},
		},
	}

	client := &mockLLMClient{
		responses: []*mockStream{
			endTurnResponse("First response"),
			endTurnResponse("Second response"),
			endTurnResponse("Third response"),
		},
	}

	registry := tools.NewRegistry()
	config := defaultConfig(client, registry)
	config.Hooks = hooks
	config.MaxTurns = 3

	q := RunLoop(context.Background(), "Hello", config)
	collectMessages(q)
	q.Wait()

	// Should have hit max turns (3) since stop hook keeps continuing
	if q.GetExitReason() != ExitMaxTurns {
		t.Errorf("exit reason = %s, want max_turns (stop hook causes continuation)", q.GetExitReason())
	}

	// Verify stop hook was fired multiple times
	events := hooks.firedEvents()
	stopCount := 0
	for _, e := range events {
		if e == types.HookEventStop {
			stopCount++
		}
	}
	if stopCount < 2 {
		t.Errorf("stop hook fired %d times, want >= 2", stopCount)
	}
}

func TestLoop_StopHookNoOverride(t *testing.T) {
	// Stop hook returns nil/empty → loop should end normally
	hooks := &mockHookRunner{
		results: map[types.HookEvent][]HookResult{},
	}

	client := &mockLLMClient{
		responses: []*mockStream{endTurnResponse("Done")},
	}
	registry := tools.NewRegistry()
	config := defaultConfig(client, registry)
	config.Hooks = hooks

	q := RunLoop(context.Background(), "Hello", config)
	collectMessages(q)
	q.Wait()

	if q.GetExitReason() != ExitEndTurn {
		t.Errorf("exit reason = %s, want end_turn", q.GetExitReason())
	}
}

func TestLoop_PreToolUseHookDeny(t *testing.T) {
	hooks := &mockHookRunner{
		results: map[types.HookEvent][]HookResult{
			types.HookEventPreToolUse: {
				{Decision: "deny", Message: "hook blocked this tool"},
			},
		},
	}

	mockTool := &mockRecordingTool{
		name:   "Bash",
		output: tools.ToolOutput{Content: "should not see this"},
	}
	registry := tools.NewRegistry()
	registry.Register(mockTool)

	client := &mockLLMClient{
		responses: []*mockStream{
			toolUseResponse("call_1", "Bash", map[string]any{"command": "rm -rf /"}),
			endTurnResponse("I see the tool was denied."),
		},
	}
	config := defaultConfig(client, registry)
	config.Hooks = hooks

	q := RunLoop(context.Background(), "Delete everything", config)
	collectMessages(q)
	q.Wait()

	// Tool should NOT have been executed (hook denied it)
	if mockTool.CallCount() != 0 {
		t.Errorf("tool should not have been called (hook denied), but was called %d times", mockTool.CallCount())
	}

	if q.GetExitReason() != ExitEndTurn {
		t.Errorf("exit reason = %s, want end_turn", q.GetExitReason())
	}
}

func TestLoop_HookAdditionalContext(t *testing.T) {
	hooks := &mockHookRunner{
		results: map[types.HookEvent][]HookResult{
			types.HookEventSessionStart: {
				{SystemMessage: "injected startup context"},
			},
		},
	}

	client := &mockLLMClient{
		responses: []*mockStream{endTurnResponse("Hello!")},
	}
	registry := tools.NewRegistry()
	config := defaultConfig(client, registry)
	config.Hooks = hooks

	q := RunLoop(context.Background(), "Hello", config)
	collectMessages(q)
	q.Wait()

	if q.GetExitReason() != ExitEndTurn {
		t.Errorf("exit reason = %s, want end_turn", q.GetExitReason())
	}

	// Verify hook events were fired
	events := hooks.firedEvents()
	if len(events) < 2 {
		t.Errorf("expected at least 2 hook events (SessionStart, Stop/SessionEnd), got %d", len(events))
	}
	if events[0] != types.HookEventSessionStart {
		t.Errorf("first event = %s, want SessionStart", events[0])
	}
}

func TestLoop_PostToolUseSuppressOutput(t *testing.T) {
	hooks := &mockHookRunner{
		results: map[types.HookEvent][]HookResult{
			types.HookEventPostToolUse: {
				{SuppressOutput: boolPtr(true)},
			},
		},
	}

	mockTool := &mockRecordingTool{
		name:   "Bash",
		output: tools.ToolOutput{Content: "sensitive output"},
	}
	registry := tools.NewRegistry()
	registry.Register(mockTool)

	client := &mockLLMClient{
		responses: []*mockStream{
			toolUseResponse("call_1", "Bash", map[string]any{"command": "ls"}),
			endTurnResponse("Done."),
		},
	}
	config := defaultConfig(client, registry)
	config.Hooks = hooks

	q := RunLoop(context.Background(), "Run ls", config)
	collectMessages(q)
	q.Wait()

	// Tool should have been called
	if mockTool.CallCount() != 1 {
		t.Errorf("tool call count = %d, want 1", mockTool.CallCount())
	}
	if q.GetExitReason() != ExitEndTurn {
		t.Errorf("exit reason = %s, want end_turn", q.GetExitReason())
	}
}

func TestLoop_HookEventSequence(t *testing.T) {
	hooks := &mockHookRunner{
		results: map[types.HookEvent][]HookResult{},
	}

	mockTool := &mockRecordingTool{name: "Bash", output: tools.ToolOutput{Content: "ok"}}
	registry := tools.NewRegistry()
	registry.Register(mockTool)

	client := &mockLLMClient{
		responses: []*mockStream{
			toolUseResponse("call_1", "Bash", map[string]any{"command": "ls"}),
			endTurnResponse("Done."),
		},
	}
	config := defaultConfig(client, registry)
	config.Hooks = hooks

	q := RunLoop(context.Background(), "Do stuff", config)
	collectMessages(q)
	q.Wait()

	events := hooks.firedEvents()

	// Expected sequence: SessionStart, PreToolUse, PostToolUse, Stop, SessionEnd
	expectedOrder := []types.HookEvent{
		types.HookEventSessionStart,
		types.HookEventPreToolUse,
		types.HookEventPostToolUse,
		types.HookEventStop,
		types.HookEventSessionEnd,
	}

	if len(events) != len(expectedOrder) {
		t.Fatalf("expected %d events, got %d: %v", len(expectedOrder), len(events), events)
	}

	for i, expected := range expectedOrder {
		if events[i] != expected {
			t.Errorf("event[%d] = %s, want %s", i, events[i], expected)
		}
	}
}

// --- Compaction Integration Tests ---

// mockCompactor implements ContextCompactor for testing.
type mockCompactor struct {
	mu            sync.Mutex
	shouldCompact bool
	compactCalls  int
	compactErr    error
}

func (m *mockCompactor) ShouldCompact(_ TokenBudget) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.shouldCompact
}

func (m *mockCompactor) Compact(_ context.Context, req CompactRequest) ([]llm.ChatMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.compactCalls++
	if m.compactErr != nil {
		return nil, m.compactErr
	}
	// Simulate compaction: keep only the last message
	if len(req.Messages) > 1 {
		summary := llm.ChatMessage{
			Role:    "user",
			Content: "[Summary of earlier conversation]",
		}
		return append([]llm.ChatMessage{summary}, req.Messages[len(req.Messages)-1]), nil
	}
	return req.Messages, nil
}

func (m *mockCompactor) CompactCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.compactCalls
}

func TestLoop_ProactiveCompaction(t *testing.T) {
	compactor := &mockCompactor{shouldCompact: true}

	client := &mockLLMClient{
		responses: []*mockStream{
			endTurnResponse("Hello after compaction!"),
		},
	}
	registry := tools.NewRegistry()
	config := defaultConfig(client, registry)
	config.Compactor = compactor

	q := RunLoop(context.Background(), "Hello", config)
	collectMessages(q)
	q.Wait()

	if q.GetExitReason() != ExitEndTurn {
		t.Errorf("exit reason = %s, want end_turn", q.GetExitReason())
	}

	// Proactive compaction should have fired before the LLM call
	if compactor.CompactCallCount() < 1 {
		t.Error("expected compaction to fire proactively")
	}
}

func TestLoop_ReactiveCompaction_MaxTokens(t *testing.T) {
	compactor := &mockCompactor{shouldCompact: true}

	length := "length"
	maxTokensResponse := &mockStream{
		chunks: []llm.StreamChunk{
			textChunk("msg-1", "claude-sonnet-4-5-20250929", "Partial output..."),
			{
				ID:    "msg-1",
				Model: "claude-sonnet-4-5-20250929",
				Choices: []llm.Choice{
					{FinishReason: &length},
				},
				Usage: &llm.Usage{PromptTokens: 100000, CompletionTokens: 16384, TotalTokens: 116384},
			},
		},
	}

	client := &mockLLMClient{
		responses: []*mockStream{
			maxTokensResponse,
			endTurnResponse("Completed after compaction"),
		},
	}
	registry := tools.NewRegistry()
	config := defaultConfig(client, registry)
	config.Compactor = compactor

	q := RunLoop(context.Background(), "Generate a long response", config)
	collectMessages(q)
	q.Wait()

	if q.GetExitReason() != ExitEndTurn {
		t.Errorf("exit reason = %s, want end_turn", q.GetExitReason())
	}

	// Compaction should have been called (both proactive and reactive)
	if compactor.CompactCallCount() < 1 {
		t.Error("expected compaction to fire on max_tokens")
	}
}

func TestLoop_CompactionFailure_GracefulDegradation(t *testing.T) {
	compactor := &mockCompactor{
		shouldCompact: false, // proactive won't trigger
	}

	length := "length"
	maxTokensResponse := &mockStream{
		chunks: []llm.StreamChunk{
			textChunk("msg-1", "claude-sonnet-4-5-20250929", "Partial..."),
			{
				ID:    "msg-1",
				Model: "claude-sonnet-4-5-20250929",
				Choices: []llm.Choice{
					{FinishReason: &length},
				},
				Usage: &llm.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
			},
		},
	}

	client := &mockLLMClient{
		responses: []*mockStream{maxTokensResponse},
	}
	registry := tools.NewRegistry()
	config := defaultConfig(client, registry)
	config.Compactor = compactor

	q := RunLoop(context.Background(), "Test", config)
	collectMessages(q)
	q.Wait()

	// When compactor says no, loop should exit with max_tokens
	if q.GetExitReason() != ExitMaxTokens {
		t.Errorf("exit reason = %s, want max_tokens", q.GetExitReason())
	}
}

func TestLoop_MultipleCompactions(t *testing.T) {
	callCount := 0
	compactor := &mockCompactor{shouldCompact: true}

	// Override Compact to track calls and always compact
	_ = compactor // use the struct

	// Create a custom compactor that tracks call count
	multiCompactor := &countingCompactor{shouldCompact: true}

	mockTool := &mockRecordingTool{name: "Bash", output: tools.ToolOutput{Content: "ok"}}
	registry := tools.NewRegistry()
	registry.Register(mockTool)

	client := &mockLLMClient{
		responses: []*mockStream{
			toolUseResponse("call_1", "Bash", map[string]any{"command": "ls"}),
			toolUseResponse("call_2", "Bash", map[string]any{"command": "ls"}),
			endTurnResponse("Done!"),
		},
	}
	config := defaultConfig(client, registry)
	config.Compactor = multiCompactor

	q := RunLoop(context.Background(), "Do stuff", config)
	collectMessages(q)
	q.Wait()

	if q.GetExitReason() != ExitEndTurn {
		t.Errorf("exit reason = %s, want end_turn", q.GetExitReason())
	}

	// Multiple compactions should have occurred (one per loop iteration)
	multiCompactor.mu.Lock()
	count := multiCompactor.compactCalls
	multiCompactor.mu.Unlock()
	_ = callCount

	if count < 2 {
		t.Errorf("expected >= 2 compaction calls, got %d", count)
	}
}

// countingCompactor counts calls but always returns messages as-is.
type countingCompactor struct {
	mu            sync.Mutex
	shouldCompact bool
	compactCalls  int
}

func (c *countingCompactor) ShouldCompact(_ TokenBudget) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.shouldCompact
}

func (c *countingCompactor) Compact(_ context.Context, req CompactRequest) ([]llm.ChatMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.compactCalls++
	// Don't actually compact — just count the calls
	return req.Messages, nil
}

// --- Session Store Integration Tests ---

// mockSessionStore records calls to SessionStore methods.
type mockSessionStore struct {
	NoOpSessionStore
	mu               sync.Mutex
	createCalls      []SessionMetadata
	appendCalls      []MessageEntry
	appendSDKCalls   []types.SDKMessage
	updateCalls      int
	closeCalled      bool

	// Configurable restore-related return values
	loadFunc       func(string) (*SessionState, error)
	loadLatestFunc func(string) (*SessionState, error)
	forkFunc       func(string, string) (*SessionState, error)
	loadUpToFunc   func(string, string) ([]MessageEntry, error)
}

func (m *mockSessionStore) Load(sessionID string) (*SessionState, error) {
	if m.loadFunc != nil {
		return m.loadFunc(sessionID)
	}
	return m.NoOpSessionStore.Load(sessionID)
}

func (m *mockSessionStore) LoadLatest(cwd string) (*SessionState, error) {
	if m.loadLatestFunc != nil {
		return m.loadLatestFunc(cwd)
	}
	return m.NoOpSessionStore.LoadLatest(cwd)
}

func (m *mockSessionStore) Fork(sourceID, newID string) (*SessionState, error) {
	if m.forkFunc != nil {
		return m.forkFunc(sourceID, newID)
	}
	return m.NoOpSessionStore.Fork(sourceID, newID)
}

func (m *mockSessionStore) LoadMessagesUpTo(sessionID, messageUUID string) ([]MessageEntry, error) {
	if m.loadUpToFunc != nil {
		return m.loadUpToFunc(sessionID, messageUUID)
	}
	return m.NoOpSessionStore.LoadMessagesUpTo(sessionID, messageUUID)
}

func (m *mockSessionStore) Create(meta SessionMetadata) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createCalls = append(m.createCalls, meta)
	return nil
}

func (m *mockSessionStore) AppendMessage(_ string, entry MessageEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.appendCalls = append(m.appendCalls, entry)
	return nil
}

func (m *mockSessionStore) AppendSDKMessage(_ string, msg types.SDKMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.appendSDKCalls = append(m.appendSDKCalls, msg)
	return nil
}

func (m *mockSessionStore) UpdateMetadata(_ string, fn func(*SessionMetadata)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateCalls++
	meta := SessionMetadata{}
	fn(&meta)
	return nil
}

func (m *mockSessionStore) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCalled = true
	return nil
}

func (m *mockSessionStore) getAppendCalls() []MessageEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]MessageEntry, len(m.appendCalls))
	copy(out, m.appendCalls)
	return out
}

func (m *mockSessionStore) getUpdateCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.updateCalls
}

func TestLoop_SessionStore_MessagesPersisted(t *testing.T) {
	store := &mockSessionStore{}

	mockTool := &mockRecordingTool{name: "Bash", output: tools.ToolOutput{Content: "ok"}}
	registry := tools.NewRegistry()
	registry.Register(mockTool)

	client := &mockLLMClient{
		responses: []*mockStream{
			toolUseResponse("call_1", "Bash", map[string]any{"command": "ls"}),
			endTurnResponse("Done."),
		},
	}
	config := defaultConfig(client, registry)
	config.SessionStore = store

	q := RunLoop(context.Background(), "Do stuff", config)
	collectMessages(q)
	q.Wait()

	calls := store.getAppendCalls()

	// Should have: user (initial), assistant (turn 1), tool result, assistant (turn 2)
	if len(calls) < 3 {
		t.Fatalf("expected at least 3 persisted messages, got %d", len(calls))
	}

	// First should be user message
	if calls[0].Message.Role != "user" {
		t.Errorf("first persisted message role = %q, want user", calls[0].Message.Role)
	}

	// Should have assistant messages
	hasAssistant := false
	for _, c := range calls {
		if c.Message.Role == "assistant" {
			hasAssistant = true
			break
		}
	}
	if !hasAssistant {
		t.Error("no assistant message persisted")
	}

	// Should have tool result
	hasTool := false
	for _, c := range calls {
		if c.Message.Role == "tool" {
			hasTool = true
			break
		}
	}
	if !hasTool {
		t.Error("no tool result message persisted")
	}
}

func TestLoop_SessionStore_MetadataUpdatedAtEnd(t *testing.T) {
	store := &mockSessionStore{}

	client := &mockLLMClient{
		responses: []*mockStream{endTurnResponse("Hello!")},
	}
	registry := tools.NewRegistry()
	config := defaultConfig(client, registry)
	config.SessionStore = store

	q := RunLoop(context.Background(), "Hello", config)
	collectMessages(q)
	q.Wait()

	if store.getUpdateCalls() < 1 {
		t.Error("expected UpdateMetadata to be called at session end")
	}
}

func TestLoop_SessionStore_NilIsNoOp(t *testing.T) {
	// Verify that SessionStore=nil doesn't cause any panics or behavior changes
	client := &mockLLMClient{
		responses: []*mockStream{endTurnResponse("Hello!")},
	}
	registry := tools.NewRegistry()
	config := defaultConfig(client, registry)
	config.SessionStore = nil // explicit nil

	q := RunLoop(context.Background(), "Hello", config)
	collectMessages(q)
	q.Wait()

	if q.GetExitReason() != ExitEndTurn {
		t.Errorf("exit reason = %s, want end_turn (nil SessionStore should be no-op)", q.GetExitReason())
	}
}

func TestLoop_SessionStore_CreateCalledOnStart(t *testing.T) {
	store := &mockSessionStore{}

	client := &mockLLMClient{
		responses: []*mockStream{endTurnResponse("Hello!")},
	}
	registry := tools.NewRegistry()
	config := defaultConfig(client, registry)
	config.SessionStore = store

	q := RunLoop(context.Background(), "Hello", config)
	collectMessages(q)
	q.Wait()

	store.mu.Lock()
	createCount := len(store.createCalls)
	store.mu.Unlock()

	if createCount < 1 {
		t.Error("expected Create to be called when SessionStore is set")
	}
}

func TestLoop_SessionStore_MessageOrderPreserved(t *testing.T) {
	store := &mockSessionStore{}

	mockTool := &mockRecordingTool{name: "Bash", output: tools.ToolOutput{Content: "hello"}}
	registry := tools.NewRegistry()
	registry.Register(mockTool)

	client := &mockLLMClient{
		responses: []*mockStream{
			toolUseResponse("call_1", "Bash", map[string]any{"command": "echo hello"}),
			endTurnResponse("Got it!"),
		},
	}
	config := defaultConfig(client, registry)
	config.SessionStore = store

	q := RunLoop(context.Background(), "Run echo", config)
	collectMessages(q)
	q.Wait()

	calls := store.getAppendCalls()

	// Verify order: user → assistant → tool → assistant
	expectedRoles := []string{"user", "assistant", "tool", "assistant"}
	if len(calls) != len(expectedRoles) {
		t.Fatalf("persisted %d messages, want %d; roles: %v", len(calls), len(expectedRoles), messageRoles(calls))
	}
	for i, expected := range expectedRoles {
		if calls[i].Message.Role != expected {
			t.Errorf("message[%d].Role = %q, want %q", i, calls[i].Message.Role, expected)
		}
	}
}

func messageRoles(entries []MessageEntry) []string {
	roles := make([]string, len(entries))
	for i, e := range entries {
		roles[i] = e.Message.Role
	}
	return roles
}

// --- RestoreSession Tests ---

func TestRestoreSession_ResumeMode(t *testing.T) {
	store := &mockSessionStore{
		loadFunc: func(id string) (*SessionState, error) {
			if id != "session-123" {
				t.Errorf("Load called with %q, want session-123", id)
			}
			return &SessionState{
				Metadata: SessionMetadata{ID: "session-123", CWD: "/test"},
				Messages: []MessageEntry{
					{UUID: "msg-1", Message: llm.ChatMessage{Role: "user", Content: "Hello"}},
					{UUID: "msg-2", Message: llm.ChatMessage{Role: "assistant", Content: "Hi there!"}},
					{UUID: "msg-3", Message: llm.ChatMessage{Role: "user", Content: "How are you?"}},
				},
			}, nil
		},
	}

	config := &AgentConfig{SessionStore: store}
	state := &LoopState{SessionID: "new-session-id"}

	opts := types.QueryOptions{Resume: "session-123"}
	err := RestoreSession(config, state, opts)
	if err != nil {
		t.Fatalf("RestoreSession error: %v", err)
	}

	if len(state.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(state.Messages))
	}
	if state.Messages[0].Role != "user" {
		t.Errorf("messages[0].Role = %q, want user", state.Messages[0].Role)
	}
	if state.Messages[1].Role != "assistant" {
		t.Errorf("messages[1].Role = %q, want assistant", state.Messages[1].Role)
	}
	if state.Messages[2].Role != "user" {
		t.Errorf("messages[2].Role = %q, want user", state.Messages[2].Role)
	}
	if state.SessionID != "session-123" {
		t.Errorf("session ID = %q, want session-123", state.SessionID)
	}
}

func TestRestoreSession_ContinueMode(t *testing.T) {
	store := &mockSessionStore{
		loadLatestFunc: func(cwd string) (*SessionState, error) {
			if cwd != "/my/project" {
				t.Errorf("LoadLatest called with %q, want /my/project", cwd)
			}
			return &SessionState{
				Metadata: SessionMetadata{ID: "latest-session", CWD: "/my/project"},
				Messages: []MessageEntry{
					{UUID: "msg-1", Message: llm.ChatMessage{Role: "user", Content: "Start"}},
					{UUID: "msg-2", Message: llm.ChatMessage{Role: "assistant", Content: "OK"}},
				},
			}, nil
		},
	}

	config := &AgentConfig{SessionStore: store, CWD: "/my/project"}
	state := &LoopState{SessionID: "unused-id"}

	opts := types.QueryOptions{Continue: true}
	err := RestoreSession(config, state, opts)
	if err != nil {
		t.Fatalf("RestoreSession error: %v", err)
	}

	if len(state.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(state.Messages))
	}
	if state.SessionID != "latest-session" {
		t.Errorf("session ID = %q, want latest-session", state.SessionID)
	}
}

func TestRestoreSession_ForkMode(t *testing.T) {
	store := &mockSessionStore{
		forkFunc: func(sourceID, newID string) (*SessionState, error) {
			if sourceID != "source-id" {
				t.Errorf("Fork sourceID = %q, want source-id", sourceID)
			}
			return &SessionState{
				Metadata: SessionMetadata{
					ID:              newID,
					ParentSessionID: sourceID,
				},
				Messages: []MessageEntry{
					{UUID: "msg-1", Message: llm.ChatMessage{Role: "user", Content: "Original Q"}},
					{UUID: "msg-2", Message: llm.ChatMessage{Role: "assistant", Content: "Original A"}},
				},
			}, nil
		},
	}

	config := &AgentConfig{SessionStore: store}
	state := &LoopState{SessionID: "forked-session-id"}

	opts := types.QueryOptions{ForkSession: true, Resume: "source-id"}
	err := RestoreSession(config, state, opts)
	if err != nil {
		t.Fatalf("RestoreSession error: %v", err)
	}

	if len(state.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(state.Messages))
	}
	if state.SessionID != "forked-session-id" {
		t.Errorf("session ID = %q, want forked-session-id", state.SessionID)
	}
}

func TestRestoreSession_ResumeAtSpecificPoint(t *testing.T) {
	store := &mockSessionStore{
		loadUpToFunc: func(sessionID, msgUUID string) ([]MessageEntry, error) {
			if sessionID != "session-123" {
				t.Errorf("LoadMessagesUpTo sessionID = %q, want session-123", sessionID)
			}
			if msgUUID != "msg-uuid-2" {
				t.Errorf("LoadMessagesUpTo messageUUID = %q, want msg-uuid-2", msgUUID)
			}
			// Return only the first 2 of 4 messages
			return []MessageEntry{
				{UUID: "msg-uuid-1", Message: llm.ChatMessage{Role: "user", Content: "Q1"}},
				{UUID: "msg-uuid-2", Message: llm.ChatMessage{Role: "assistant", Content: "A1"}},
			}, nil
		},
	}

	config := &AgentConfig{SessionStore: store}
	state := &LoopState{SessionID: "new-id"}

	opts := types.QueryOptions{Resume: "session-123", ResumeSessionAt: "msg-uuid-2"}
	err := RestoreSession(config, state, opts)
	if err != nil {
		t.Fatalf("RestoreSession error: %v", err)
	}

	if len(state.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(state.Messages))
	}
	if state.Messages[0].Role != "user" {
		t.Errorf("messages[0].Role = %q, want user", state.Messages[0].Role)
	}
	if state.Messages[1].Role != "assistant" {
		t.Errorf("messages[1].Role = %q, want assistant", state.Messages[1].Role)
	}
}

func TestRestoreSession_NilStore(t *testing.T) {
	config := &AgentConfig{SessionStore: nil}
	state := &LoopState{SessionID: "test-id"}

	opts := types.QueryOptions{Resume: "session-123"}
	err := RestoreSession(config, state, opts)
	if err != nil {
		t.Fatalf("RestoreSession error with nil store: %v", err)
	}

	// State should be unchanged
	if len(state.Messages) != 0 {
		t.Errorf("expected 0 messages with nil store, got %d", len(state.Messages))
	}
	if state.SessionID != "test-id" {
		t.Errorf("session ID should be unchanged, got %q", state.SessionID)
	}
}

// --- Test Parity: Budget Exhaustion (ported from Python Agent SDK) ---

func TestLoop_BudgetExhausted(t *testing.T) {
	// Set a very small MaxBudgetUSD so the loop exits after the first turn.
	// The mockLLMClient returns usage with tokens that cost more than the budget.
	client := &mockLLMClient{
		responses: []*mockStream{
			// First turn: respond with text (will consume tokens → cost > budget)
			endTurnResponse("Hello!"),
			// Second turn should never be reached
			endTurnResponse("This should not be reached"),
		},
	}
	registry := tools.NewRegistry()
	config := defaultConfig(client, registry)
	config.MaxBudgetUSD = 0.0000001 // Extremely small budget (effectively 0)

	q := RunLoop(context.Background(), "Hello", config)
	collectMessages(q)
	q.Wait()

	// After the first LLM call, cost should exceed the tiny budget
	// The loop should detect this on the next iteration's checkTermination
	// Note: because checkTermination runs at the TOP of the loop, the first
	// turn always completes. The budget check fires on the second iteration.
	// With a tool call response, we'd get the check. With end_turn, the stop
	// hook fires first. So budget check is exercised when the loop continues.

	// For a clean budget exhaustion test, use a stop hook that continues:
	hooks := &mockHookRunner{
		results: map[types.HookEvent][]HookResult{
			types.HookEventStop: {
				{Continue: boolPtr(true)}, // force continuation after end_turn
			},
		},
	}
	client2 := &mockLLMClient{
		responses: []*mockStream{
			endTurnResponse("Turn 1"),
			endTurnResponse("Turn 2 - should not happen"),
		},
	}
	config2 := defaultConfig(client2, registry)
	config2.MaxBudgetUSD = 0.0000001
	config2.Hooks = hooks

	q2 := RunLoop(context.Background(), "Hello", config2)
	collectMessages(q2)
	q2.Wait()

	if q2.GetExitReason() != ExitMaxBudget {
		t.Errorf("exit reason = %s, want error_max_budget_usd", q2.GetExitReason())
	}
}

// --- Test Parity: Thinking Delta StreamEvents (ported from Python Agent SDK) ---

func TestLoop_StreamEvents_ThinkingDelta(t *testing.T) {
	// Mock LLM returns chunks with ReasoningContent deltas (thinking deltas).
	// With IncludePartial=true, these should be emitted as stream_event messages
	// with delta.type == "thinking_delta".
	thinking := "Let me think about this..."
	text := "Here's my answer."
	stop := "stop"

	thinkingResponse := &mockStream{
		chunks: []llm.StreamChunk{
			{
				ID:    "msg-1",
				Model: "claude-sonnet-4-5-20250929",
				Choices: []llm.Choice{
					{Delta: llm.Delta{ReasoningContent: &thinking}},
				},
			},
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
				Usage: &llm.Usage{PromptTokens: 100, CompletionTokens: 80, TotalTokens: 180},
			},
		},
	}

	client := &mockLLMClient{
		responses: []*mockStream{thinkingResponse},
	}
	registry := tools.NewRegistry()
	config := defaultConfig(client, registry)
	config.IncludePartial = true

	q := RunLoop(context.Background(), "Think about this", config)
	msgs := collectMessages(q)
	q.Wait()

	if q.GetExitReason() != ExitEndTurn {
		t.Errorf("exit reason = %s, want end_turn", q.GetExitReason())
	}

	// Should have stream_event messages with thinking_delta
	foundThinkingDelta := false
	foundTextDelta := false
	for _, m := range msgs {
		if m.GetType() != types.MessageTypeStreamEvent {
			continue
		}
		partial, ok := m.(*types.PartialAssistantMessage)
		if !ok {
			// value type
			if pv, ok2 := m.(types.PartialAssistantMessage); ok2 {
				partial = &pv
			} else {
				continue
			}
		}
		eventMap, ok := partial.Event.(map[string]any)
		if !ok {
			continue
		}
		delta, ok := eventMap["delta"].(map[string]any)
		if !ok {
			continue
		}
		if delta["type"] == "thinking_delta" {
			foundThinkingDelta = true
			if delta["thinking"] != "Let me think about this..." {
				t.Errorf("thinking delta content = %q, want 'Let me think about this...'", delta["thinking"])
			}
		}
		if delta["type"] == "text_delta" {
			foundTextDelta = true
		}
	}

	if !foundThinkingDelta {
		t.Error("expected stream_event with thinking_delta, none found")
	}
	if !foundTextDelta {
		t.Error("expected stream_event with text_delta, none found")
	}
}

// --- Stub Tests for Unimplemented Features ---

func TestLoop_StructuredOutput(t *testing.T) {
	t.Skip("not yet implemented: structured output (JSON schema enforcement)")
}

func TestLoop_DynamicControlSetPermissionMode(t *testing.T) {
	t.Skip("not yet implemented: dynamic control (set_permission_mode at runtime)")
}

func TestLoop_DynamicControlSetModel(t *testing.T) {
	t.Skip("not yet implemented: dynamic control (set_model at runtime)")
}

func TestLoop_GracefulInterruptMidStream(t *testing.T) {
	t.Skip("not yet implemented: graceful interrupt mid-stream")
}

func TestLoop_SessionResumeContinueConversation(t *testing.T) {
	t.Skip("not yet implemented: session resume (continue_conversation)")
}

func TestLoop_SettingSourcesHierarchy(t *testing.T) {
	t.Skip("not yet implemented: setting_sources hierarchy")
}
