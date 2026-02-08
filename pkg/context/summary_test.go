package context

import (
	"context"
	"io"
	"testing"

	"github.com/jg-phare/goat/pkg/llm"
)

// mockSummaryClient is a mock LLM client that returns a pre-configured summary.
type mockSummaryClient struct {
	summary string
	err     error
	calls   int
}

func (m *mockSummaryClient) Complete(_ context.Context, req *llm.CompletionRequest) (*llm.Stream, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}

	// Build a stream that returns a single text chunk and finishes
	content := m.summary
	stop := "stop"
	events := make(chan llm.StreamEvent, 3)
	events <- llm.StreamEvent{Chunk: &llm.StreamChunk{
		ID:    "sum-1",
		Model: "claude-haiku-4-5-20251001",
		Choices: []llm.Choice{{
			Delta: llm.Delta{Content: &content},
		}},
	}}
	events <- llm.StreamEvent{Chunk: &llm.StreamChunk{
		ID:    "sum-1",
		Model: "claude-haiku-4-5-20251001",
		Choices: []llm.Choice{{
			FinishReason: &stop,
		}},
		Usage: &llm.Usage{PromptTokens: 500, CompletionTokens: 100, TotalTokens: 600},
	}}
	close(events)

	pr, pw := io.Pipe()
	pw.Close()
	_, cancel := context.WithCancel(context.Background())
	return llm.NewStream(events, pr, cancel), nil
}

func (m *mockSummaryClient) Model() string      { return "claude-haiku-4-5-20251001" }
func (m *mockSummaryClient) SetModel(_ string)   {}

func TestGenerateSummary(t *testing.T) {
	client := &mockSummaryClient{summary: "## Summary\nUser asked about Go testing."}

	messages := []llm.ChatMessage{
		{Role: "user", Content: "How do I test Go code?"},
		{Role: "assistant", Content: "Use the testing package with go test."},
	}

	summary, err := generateSummary(context.Background(), messages, client, "claude-haiku-4-5-20251001", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary != "## Summary\nUser asked about Go testing." {
		t.Errorf("unexpected summary: %s", summary)
	}

	if client.calls != 1 {
		t.Errorf("expected 1 LLM call, got %d", client.calls)
	}
}

func TestGenerateSummary_WithCustomInstructions(t *testing.T) {
	client := &mockSummaryClient{summary: "Custom summary"}

	messages := []llm.ChatMessage{
		{Role: "user", Content: "hello"},
	}

	instructions := "Focus on code changes only"
	summary, err := generateSummary(context.Background(), messages, client, "claude-haiku-4-5-20251001", &instructions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary != "Custom summary" {
		t.Errorf("unexpected summary: %s", summary)
	}
}

func TestBuildCompactionPrompt(t *testing.T) {
	messages := []llm.ChatMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}

	prompt := buildCompactionPrompt(messages, nil)

	// Should contain the system prompt
	if !contains(prompt, "Summarize the following conversation") {
		t.Error("missing compaction system prompt")
	}

	// Should contain message content
	if !contains(prompt, "[user]: hello") {
		t.Error("missing user message")
	}
	if !contains(prompt, "[assistant]: hi there") {
		t.Error("missing assistant message")
	}
}

func TestBuildCompactionPrompt_LongContentTruncated(t *testing.T) {
	longContent := make([]byte, 5000)
	for i := range longContent {
		longContent[i] = 'x'
	}

	messages := []llm.ChatMessage{
		{Role: "user", Content: string(longContent)},
	}

	prompt := buildCompactionPrompt(messages, nil)

	// The long content should be truncated
	if !contains(prompt, "...") {
		t.Error("expected truncation indicator in prompt")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && len(s) >= len(substr) && containsHelper(s, substr)
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
