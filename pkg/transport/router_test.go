package transport

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/llm"
	"github.com/jg-phare/goat/pkg/tools"
	"github.com/jg-phare/goat/pkg/types"
)

// --- Test Infrastructure (mirrors agent package test helpers) ---

type mockStream struct {
	chunks []llm.StreamChunk
}

func (ms *mockStream) toStream(ctx context.Context) *llm.Stream {
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

type mockLLMClient struct {
	mu        sync.Mutex
	responses []*mockStream
	callIndex int
}

func (m *mockLLMClient) Complete(ctx context.Context, req *llm.CompletionRequest) (*llm.Stream, error) {
	m.mu.Lock()
	idx := m.callIndex
	m.callIndex++
	m.mu.Unlock()

	if idx >= len(m.responses) {
		return endTurnResponse("No more responses").toStream(ctx), nil
	}

	return m.responses[idx].toStream(ctx), nil
}

func (m *mockLLMClient) Model() string      { return "test" }
func (m *mockLLMClient) SetModel(string) {}

func endTurnResponse(text string) *mockStream {
	content := text
	stop := "stop"
	return &mockStream{
		chunks: []llm.StreamChunk{
			{
				ID:    "msg-1",
				Model: "claude-sonnet-4-5-20250929",
				Choices: []llm.Choice{
					{Delta: llm.Delta{Content: &content}},
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

func defaultConfig(client llm.Client) agent.AgentConfig {
	return agent.AgentConfig{
		Model:          "claude-sonnet-4-5-20250929",
		MaxTurns:       100,
		MultiTurn:      true,
		PermissionMode: types.PermissionModeDefault,
		LLMClient:      client,
		ToolRegistry:   tools.NewRegistry(),
		Prompter:       &agent.StaticPromptAssembler{Prompt: "You are a test assistant."},
		Permissions:    &agent.AllowAllChecker{},
		Hooks:          &agent.NoOpHookRunner{},
		Compactor:      &agent.NoOpCompactor{},
		CostTracker:    llm.NewCostTracker(),
	}
}

// --- Router Tests ---

func TestRouter_UserMessageRouted(t *testing.T) {
	client := &mockLLMClient{
		responses: []*mockStream{
			endTurnResponse("Hello!"),
			endTurnResponse("I can help with Go!"),
		},
	}
	config := defaultConfig(client)

	q := agent.RunLoop(context.Background(), "Hello", config)
	tr := NewChannelTransport(64)
	router := NewRouter(tr, q)

	// Run router in background
	routerDone := make(chan error, 1)
	go func() {
		routerDone <- router.Run()
	}()

	// Wait for first turn to produce output
	time.Sleep(100 * time.Millisecond)

	// Read first output messages
	outputCount := 0
	go func() {
		for range tr.Output() {
			outputCount++
		}
	}()

	// Send a user message
	userMsg := InputMessage{
		Type:    "user_message",
		Payload: json.RawMessage(`"Can you help with Go?"`),
	}
	payload, _ := json.Marshal(userMsg)
	tr.Send(TransportMessage{
		Type:    TMsgOutput,
		Payload: payload,
	})

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Close the transport to signal we're done
	tr.EndInput()

	// Wait for router to finish
	select {
	case err := <-routerDone:
		if err != nil {
			t.Logf("router error (may be expected): %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("router did not finish")
	}

	if q.TurnCount() < 1 {
		t.Errorf("turn count = %d, want >= 1", q.TurnCount())
	}
}

func TestRouter_ControlRequest(t *testing.T) {
	client := &mockLLMClient{
		responses: []*mockStream{
			endTurnResponse("Hello!"),
		},
	}
	config := defaultConfig(client)

	q := agent.RunLoop(context.Background(), "Hello", config)
	tr := NewChannelTransport(64)
	router := NewRouter(tr, q)

	routerDone := make(chan error, 1)
	go func() {
		routerDone <- router.Run()
	}()

	// Drain output
	go func() {
		for range tr.Output() {
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// Send a control request via the transport
	ctrlReq := types.ControlRequest{
		Type:      "control_request",
		RequestID: "ctrl-1",
		Request: types.ControlRequestInner{
			Subtype: types.ControlSubtypeSetModel,
			Model:   "claude-opus-4-6",
		},
	}
	inputMsg := InputMessage{
		Type:    "control_request",
		Payload: mustMarshal(ctrlReq),
	}
	payload, _ := json.Marshal(inputMsg)
	tr.Send(TransportMessage{
		Type:    TMsgOutput,
		Payload: payload,
	})

	// Give time for processing
	time.Sleep(200 * time.Millisecond)

	tr.EndInput()

	select {
	case <-routerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("router did not finish")
	}
}

func TestRouter_TransportClose_TriggersQueryClose(t *testing.T) {
	client := &mockLLMClient{
		responses: []*mockStream{
			endTurnResponse("Hello!"),
		},
	}
	config := defaultConfig(client)

	q := agent.RunLoop(context.Background(), "Hello", config)
	tr := NewChannelTransport(64)
	router := NewRouter(tr, q)

	routerDone := make(chan error, 1)
	go func() {
		routerDone <- router.Run()
	}()

	// Drain output
	go func() {
		for range tr.Output() {
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// Close the transport — should trigger query close
	tr.EndInput()
	tr.Close()

	select {
	case <-routerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("router did not finish after transport close")
	}

	// Query should be done
	select {
	case <-func() chan struct{} { ch := make(chan struct{}); go func() { q.Wait(); close(ch) }(); return ch }():
	case <-time.After(5 * time.Second):
		t.Fatal("query did not finish after transport close")
	}
}

func TestRouter_QueryDone_TriggersRouterShutdown(t *testing.T) {
	client := &mockLLMClient{
		responses: []*mockStream{
			endTurnResponse("Hello!"),
		},
	}
	// Use one-shot mode (not multi-turn) so query finishes immediately
	config := defaultConfig(client)
	config.MultiTurn = false

	q := agent.RunLoop(context.Background(), "Hello", config)
	tr := NewChannelTransport(64)
	router := NewRouter(tr, q)

	routerDone := make(chan error, 1)
	go func() {
		routerDone <- router.Run()
	}()

	// Drain output
	go func() {
		for range tr.Output() {
		}
	}()

	// Query will finish on its own (one-shot mode)
	select {
	case <-routerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("router did not finish when query completed")
	}

	// Transport should be closed
	if tr.IsReady() {
		t.Error("transport should not be ready after router shutdown")
	}
}

func TestRouter_RawPayloadAsUserMessage(t *testing.T) {
	// When the payload doesn't have a type field, treat it as a raw user message
	client := &mockLLMClient{
		responses: []*mockStream{
			endTurnResponse("Hello!"),
			endTurnResponse("Got your raw message."),
		},
	}
	config := defaultConfig(client)

	q := agent.RunLoop(context.Background(), "Hello", config)
	tr := NewChannelTransport(64)
	router := NewRouter(tr, q)

	routerDone := make(chan error, 1)
	go func() {
		routerDone <- router.Run()
	}()

	go func() {
		for range tr.Output() {
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// Send raw text (not wrapped in InputMessage)
	tr.Send(TransportMessage{
		Type:    TMsgOutput,
		Payload: json.RawMessage(`"raw user text"`),
	})

	time.Sleep(200 * time.Millisecond)

	tr.EndInput()

	select {
	case <-routerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("router did not finish")
	}
}
