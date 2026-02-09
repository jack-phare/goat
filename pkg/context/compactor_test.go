package context

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/llm"
	"github.com/jg-phare/goat/pkg/types"
)

// mockHookRunner records hook events for verification.
type mockHookRunner struct {
	mu     sync.Mutex
	events []types.HookEvent
}

func (m *mockHookRunner) Fire(_ context.Context, event types.HookEvent, _ any) ([]agent.HookResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil, nil
}

func (m *mockHookRunner) firedEvents() []types.HookEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]types.HookEvent, len(m.events))
	copy(out, m.events)
	return out
}

func TestCompactor_ShouldCompact(t *testing.T) {
	c := NewCompactor(CompactorConfig{ThresholdPct: 0.80})

	tests := []struct {
		name   string
		budget agent.TokenBudget
		want   bool
	}{
		{
			"under threshold",
			agent.TokenBudget{ContextLimit: 200_000, SystemPromptTkns: 10_000, MessageTkns: 100_000, MaxOutputTkns: 16384},
			false,
		},
		{
			"at threshold",
			agent.TokenBudget{ContextLimit: 200_000, SystemPromptTkns: 10_000, MessageTkns: 140_000, MaxOutputTkns: 10_000},
			false, // exactly 80% is not > 80%
		},
		{
			"over threshold",
			agent.TokenBudget{ContextLimit: 200_000, SystemPromptTkns: 10_000, MessageTkns: 150_000, MaxOutputTkns: 16384},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.ShouldCompact(tt.budget)
			if got != tt.want {
				t.Errorf("ShouldCompact() = %v, want %v (utilization=%.2f)", got, tt.want, tt.budget.UtilizationPct())
			}
		})
	}
}

func TestCompactor_Compact_WithSummary(t *testing.T) {
	hooks := &mockHookRunner{}
	client := &mockSummaryClient{summary: "Summary of early conversation."}

	c := NewCompactor(CompactorConfig{
		LLMClient:     client,
		HookRunner:    hooks,
		SummaryModel:  "claude-haiku-4-5-20251001",
		ThresholdPct:  0.80,
		PreserveRatio: 0.40,
	})

	// Build messages: enough that split will happen
	messages := make([]llm.ChatMessage, 10)
	for i := range messages {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		messages[i] = llm.ChatMessage{
			Role:    role,
			Content: strings.Repeat(fmt.Sprintf("msg%d-", i), 200),
		}
	}

	ch := make(chan types.SDKMessage, 10)
	budget := agent.TokenBudget{
		ContextLimit:     1000,
		SystemPromptTkns: 50,
		MaxOutputTkns:    100,
		MessageTkns:      900,
	}

	compacted, err := c.Compact(context.Background(), agent.CompactRequest{
		Messages:  messages,
		Model:     "claude-sonnet-4-5-20250929",
		Budget:    budget,
		Trigger:   "auto",
		SessionID: "test-session",
		EmitCh:    ch,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have fewer messages than original
	if len(compacted) >= len(messages) {
		t.Errorf("compacted length %d should be less than original %d", len(compacted), len(messages))
	}

	// First message should be the summary
	if !strings.Contains(ContentString(compacted[0]), "Summary of early conversation") {
		t.Errorf("first compacted message should be summary, got: %s", ContentString(compacted[0])[:100])
	}

	// LLM should have been called once
	if client.calls != 1 {
		t.Errorf("expected 1 LLM call, got %d", client.calls)
	}

	// Hooks should have been fired
	events := hooks.firedEvents()
	if len(events) != 2 {
		t.Fatalf("expected 2 hook events, got %d: %v", len(events), events)
	}
	if events[0] != types.HookEventPreCompact {
		t.Errorf("first hook = %s, want PreCompact", events[0])
	}
	if events[1] != types.HookEventSessionStart {
		t.Errorf("second hook = %s, want SessionStart", events[1])
	}

	// CompactBoundaryMessage should have been emitted
	select {
	case msg := <-ch:
		cbm, ok := msg.(*types.CompactBoundaryMessage)
		if !ok {
			t.Fatalf("expected CompactBoundaryMessage, got %T", msg)
		}
		if cbm.CompactMetadata.Trigger != "auto" {
			t.Errorf("trigger = %s, want auto", cbm.CompactMetadata.Trigger)
		}
		if cbm.CompactMetadata.PreTokens <= 0 {
			t.Errorf("pre_tokens = %d, should be > 0", cbm.CompactMetadata.PreTokens)
		}
	default:
		t.Error("no CompactBoundaryMessage emitted")
	}
}

func TestCompactor_Compact_FallbackOnError(t *testing.T) {
	hooks := &mockHookRunner{}
	client := &mockSummaryClient{err: fmt.Errorf("LLM unavailable")}

	c := NewCompactor(CompactorConfig{
		LLMClient:     client,
		HookRunner:    hooks,
		PreserveRatio: 0.40,
	})

	messages := make([]llm.ChatMessage, 6)
	for i := range messages {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		messages[i] = llm.ChatMessage{
			Role:    role,
			Content: strings.Repeat(fmt.Sprintf("msg%d-", i), 200),
		}
	}

	ch := make(chan types.SDKMessage, 10)
	compacted, err := c.Compact(context.Background(), agent.CompactRequest{
		Messages:  messages,
		Model:     "claude-sonnet-4-5-20250929",
		Budget:    agent.TokenBudget{ContextLimit: 1000, MaxOutputTkns: 100, MessageTkns: 900},
		Trigger:   "auto",
		SessionID: "test-session",
		EmitCh:    ch,
	})
	if err != nil {
		t.Fatalf("should not return error on summary failure, got: %v", err)
	}

	// Should fall back to truncation (just preserve zone)
	if len(compacted) >= len(messages) {
		t.Errorf("should have truncated, compacted=%d, original=%d", len(compacted), len(messages))
	}

	// No summary message — first message should be from the preserve zone
	// (not a summary prefix)
	first := ContentString(compacted[0])
	if strings.Contains(first, "Previous conversation summary") {
		t.Error("fallback should not contain summary prefix")
	}
}

func TestCompactor_Compact_SingleMessage(t *testing.T) {
	c := NewCompactor(CompactorConfig{})

	messages := []llm.ChatMessage{
		{Role: "user", Content: "hello"},
	}

	compacted, err := c.Compact(context.Background(), agent.CompactRequest{
		Messages: messages,
		Budget:   agent.TokenBudget{ContextLimit: 1000},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(compacted) != 1 {
		t.Errorf("single message should not be compacted, got %d", len(compacted))
	}
}

func TestCompactor_Compact_NoLLMClient(t *testing.T) {
	hooks := &mockHookRunner{}
	c := NewCompactor(CompactorConfig{
		HookRunner:    hooks,
		PreserveRatio: 0.40,
	})

	messages := make([]llm.ChatMessage, 6)
	for i := range messages {
		messages[i] = llm.ChatMessage{
			Role:    "user",
			Content: strings.Repeat("x", 500),
		}
	}

	ch := make(chan types.SDKMessage, 10)
	compacted, err := c.Compact(context.Background(), agent.CompactRequest{
		Messages:  messages,
		Budget:    agent.TokenBudget{ContextLimit: 1000, MaxOutputTkns: 100, MessageTkns: 900},
		Trigger:   "manual",
		SessionID: "s1",
		EmitCh:    ch,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Without LLM client, should do simple truncation
	if len(compacted) >= len(messages) {
		t.Error("should have truncated without LLM client")
	}
}

func TestCompactor_MustCompact(t *testing.T) {
	c := NewCompactor(CompactorConfig{CriticalPct: 0.95})

	tests := []struct {
		name   string
		budget agent.TokenBudget
		want   bool
	}{
		{
			"under critical",
			agent.TokenBudget{ContextLimit: 200_000, SystemPromptTkns: 10_000, MessageTkns: 150_000, MaxOutputTkns: 16384},
			false, // ~88%
		},
		{
			"at critical boundary",
			agent.TokenBudget{ContextLimit: 200_000, SystemPromptTkns: 10_000, MessageTkns: 170_000, MaxOutputTkns: 10_000},
			false, // exactly 95%
		},
		{
			"over critical",
			agent.TokenBudget{ContextLimit: 200_000, SystemPromptTkns: 10_000, MessageTkns: 180_000, MaxOutputTkns: 16384},
			true, // ~103% (overflow)
		},
		{
			"96 percent",
			agent.TokenBudget{ContextLimit: 100_000, SystemPromptTkns: 5_000, MessageTkns: 82_000, MaxOutputTkns: 10_000},
			true, // 97%
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.MustCompact(tt.budget)
			if got != tt.want {
				t.Errorf("MustCompact() = %v, want %v (utilization=%.2f)", got, tt.want, tt.budget.UtilizationPct())
			}
		})
	}
}

func TestCompactor_Compact_CustomInstructionsFromHook(t *testing.T) {
	hookRunner := &hookRunnerWithCustomInstructions{
		instructions: "Focus on code changes and ignore chitchat",
	}
	client := &mockSummaryClient{summary: "Summary with custom instructions."}

	c := NewCompactor(CompactorConfig{
		LLMClient:     client,
		HookRunner:    hookRunner,
		PreserveRatio: 0.40,
	})

	messages := make([]llm.ChatMessage, 6)
	for i := range messages {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		messages[i] = llm.ChatMessage{
			Role:    role,
			Content: strings.Repeat(fmt.Sprintf("msg%d-", i), 200),
		}
	}

	ch := make(chan types.SDKMessage, 10)
	_, err := c.Compact(context.Background(), agent.CompactRequest{
		Messages:  messages,
		Budget:    agent.TokenBudget{ContextLimit: 1000, MaxOutputTkns: 100, MessageTkns: 900},
		Trigger:   "auto",
		SessionID: "test",
		EmitCh:    ch,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify that the LLM was called with custom instructions
	if client.lastPrompt == "" {
		t.Fatal("expected LLM to be called")
	}
	if !strings.Contains(client.lastPrompt, "Focus on code changes") {
		t.Errorf("custom instructions not passed to summary prompt; got:\n%s", client.lastPrompt[:200])
	}
}

// hookRunnerWithCustomInstructions returns custom_instructions from PreCompact hook.
type hookRunnerWithCustomInstructions struct {
	instructions string
}

func (h *hookRunnerWithCustomInstructions) Fire(_ context.Context, event types.HookEvent, _ any) ([]agent.HookResult, error) {
	if event == types.HookEventPreCompact {
		return []agent.HookResult{
			{
				HookSpecificOutput: map[string]any{
					"custom_instructions": h.instructions,
				},
			},
		}, nil
	}
	return nil, nil
}

func TestNewCompactor_Defaults(t *testing.T) {
	c := NewCompactor(CompactorConfig{})

	if c.thresholdPct != 0.80 {
		t.Errorf("default threshold = %f, want 0.80", c.thresholdPct)
	}
	if c.criticalPct != 0.95 {
		t.Errorf("default critical = %f, want 0.95", c.criticalPct)
	}
	if c.preserveRatio != 0.40 {
		t.Errorf("default preserveRatio = %f, want 0.40", c.preserveRatio)
	}
	if c.summaryModel != "claude-haiku-4-5-20251001" {
		t.Errorf("default summaryModel = %s, want claude-haiku-4-5-20251001", c.summaryModel)
	}
	if c.estimator == nil {
		t.Error("default estimator should not be nil")
	}
	if c.hooks == nil {
		t.Error("default hooks should not be nil")
	}
}
