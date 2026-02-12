package agent

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/jg-phare/goat/pkg/llm"
)

func TestSessionMemoryTracker_ShouldExtract_FirstTime(t *testing.T) {
	tracker := NewSessionMemoryTracker("/tmp/test", nil, nil)

	// Not enough tokens yet
	tracker.TrackTokens(5000, 0)
	if tracker.ShouldExtract() {
		t.Error("should not extract at 5000 tokens (first threshold is 10000)")
	}

	// Hit the threshold
	tracker.TrackTokens(5000, 0)
	if !tracker.ShouldExtract() {
		t.Error("should extract at 10000 tokens (first extraction threshold)")
	}
}

func TestSessionMemoryTracker_ShouldExtract_Subsequent(t *testing.T) {
	tracker := NewSessionMemoryTracker("/tmp/test", nil, nil)

	// Do first extraction
	tracker.TrackTokens(10000, 0)
	if !tracker.ShouldExtract() {
		t.Fatal("should extract at first threshold")
	}
	tracker.Reset() // simulate successful extraction

	// Subsequent threshold is 5000
	tracker.TrackTokens(3000, 0)
	if tracker.ShouldExtract() {
		t.Error("should not extract at 3000 tokens (subsequent threshold is 5000)")
	}

	tracker.TrackTokens(2000, 0)
	if !tracker.ShouldExtract() {
		t.Error("should extract at 5000 tokens (subsequent threshold)")
	}
}

func TestSessionMemoryTracker_ShouldExtract_ToolCallThreshold(t *testing.T) {
	tracker := NewSessionMemoryTracker("/tmp/test", nil, nil)

	// First extraction done
	tracker.TrackTokens(10000, 0)
	tracker.Reset()

	// Tool calls trigger extraction
	tracker.TrackToolCall()
	tracker.TrackToolCall()
	if tracker.ShouldExtract() {
		t.Error("should not extract at 2 tool calls")
	}

	tracker.TrackToolCall()
	if !tracker.ShouldExtract() {
		t.Error("should extract at 3 tool calls")
	}
}

func TestSessionMemoryTracker_Reset(t *testing.T) {
	tracker := NewSessionMemoryTracker("/tmp/test", nil, nil)

	tracker.TrackTokens(15000, 0)
	tracker.TrackToolCall()
	tracker.TrackToolCall()
	tracker.TrackToolCall()

	tracker.Reset()

	if tracker.ShouldExtract() {
		t.Error("should not extract after reset")
	}
	if !tracker.firstExtractionDone {
		t.Error("firstExtractionDone should be true after reset")
	}
}

func TestSessionMemoryTracker_Extract_WritesSummary(t *testing.T) {
	sessionDir := t.TempDir()

	stop := "stop"
	text := "Session summary: worked on memory system."
	chunks := []llm.StreamChunk{
		{
			ID:    "msg-1",
			Model: "claude-haiku-4-5-20251001",
			Choices: []llm.Choice{
				{Delta: llm.Delta{Content: &text}},
			},
		},
		{
			ID:    "msg-1",
			Model: "claude-haiku-4-5-20251001",
			Choices: []llm.Choice{
				{FinishReason: &stop},
			},
			Usage: &llm.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
		},
	}

	client := &memoryMockClient{chunks: chunks}
	tracker := NewSessionMemoryTracker(sessionDir, client, nil)

	messages := []llm.ChatMessage{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
	}

	err := tracker.Extract(context.Background(), messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify summary was written
	summaryPath := filepath.Join(sessionDir, "session-memory", "summary.md")
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("summary file should exist: %v", err)
	}
	if string(data) != "Session summary: worked on memory system." {
		t.Errorf("unexpected summary: %q", string(data))
	}
}

func TestSessionMemoryTracker_Extract_NoClient(t *testing.T) {
	tracker := NewSessionMemoryTracker(t.TempDir(), nil, nil)

	err := tracker.Extract(context.Background(), nil)
	if err != nil {
		t.Fatalf("should not error with nil client: %v", err)
	}
}

func TestSessionMemoryTracker_ConcurrentExtraction(t *testing.T) {
	tracker := NewSessionMemoryTracker(t.TempDir(), nil, nil)

	// Simulate already extracting
	tracker.mu.Lock()
	tracker.extracting = true
	tracker.mu.Unlock()

	if tracker.ShouldExtract() {
		t.Error("should not trigger extraction while one is in progress")
	}
}

// memoryMockClient provides a mock LLM client for session memory tests.
type memoryMockClient struct {
	chunks []llm.StreamChunk
}

func (m *memoryMockClient) Complete(ctx context.Context, req *llm.CompletionRequest) (*llm.Stream, error) {
	events := make(chan llm.StreamEvent, len(m.chunks)+1)
	go func() {
		defer close(events)
		for _, chunk := range m.chunks {
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
	return llm.NewStream(events, pr, cancel), nil
}

func (m *memoryMockClient) Model() string       { return "claude-haiku-4-5-20251001" }
func (m *memoryMockClient) SetModel(_ string)    {}
