package agent

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"github.com/jg-phare/goat/pkg/llm"
)

const (
	firstExtractionThreshold = 10_000 // tokens before first extraction
	subsequentTokenThreshold = 5_000  // tokens between subsequent extractions
	toolCallThreshold        = 3      // tool calls between extractions
)

// SessionMemoryTracker tracks when to extract session memory summaries
// and runs background LLM-powered extractions.
type SessionMemoryTracker struct {
	mu sync.Mutex

	tokensSinceExtraction    int
	toolCallsSinceExtraction int
	totalTokens              int
	firstExtractionDone      bool

	sessionDir string
	llmClient  llm.Client
	assembler  SystemPromptAssembler

	// extracting indicates a background extraction is in progress
	extracting bool
}

// NewSessionMemoryTracker creates a tracker for the given session directory.
func NewSessionMemoryTracker(sessionDir string, client llm.Client, assembler SystemPromptAssembler) *SessionMemoryTracker {
	return &SessionMemoryTracker{
		sessionDir: sessionDir,
		llmClient:  client,
		assembler:  assembler,
	}
}

// TrackTokens records input and output token usage.
func (t *SessionMemoryTracker) TrackTokens(input, output int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	total := input + output
	t.tokensSinceExtraction += total
	t.totalTokens += total
}

// TrackToolCall records that a tool was executed.
func (t *SessionMemoryTracker) TrackToolCall() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.toolCallsSinceExtraction++
}

// ShouldExtract returns true when enough tokens or tool calls have accumulated
// to warrant a session memory extraction.
func (t *SessionMemoryTracker) ShouldExtract() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.extracting {
		return false // already running
	}

	if !t.firstExtractionDone {
		return t.tokensSinceExtraction >= firstExtractionThreshold
	}

	return t.tokensSinceExtraction >= subsequentTokenThreshold ||
		t.toolCallsSinceExtraction >= toolCallThreshold
}

// Reset clears the counters after a successful extraction.
func (t *SessionMemoryTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tokensSinceExtraction = 0
	t.toolCallsSinceExtraction = 0
	t.firstExtractionDone = true
	t.extracting = false
}

// Extract runs a background LLM call to generate a session memory summary.
// The summary is written to {sessionDir}/session-memory/summary.md.
func (t *SessionMemoryTracker) Extract(ctx context.Context, messages []llm.ChatMessage) error {
	t.mu.Lock()
	if t.extracting {
		t.mu.Unlock()
		return nil // already in progress
	}
	t.extracting = true
	t.mu.Unlock()

	defer func() {
		t.Reset()
	}()

	if t.llmClient == nil || t.sessionDir == "" {
		return nil
	}

	// Build extraction prompt
	extractionPrompt := "Summarize the key decisions, context, and progress of this conversation in a concise format suitable for loading into a new session. Focus on: goals, decisions made, files modified, patterns discovered, and open questions."

	// Build a simple request using the conversation history
	req := llm.BuildCompletionRequest(
		llm.ClientConfig{
			Model:     "claude-haiku-4-5-20251001",
			MaxTokens: 4096,
		},
		extractionPrompt,
		messages,
		nil,
		llm.LoopState{},
	)

	stream, err := t.llmClient.Complete(ctx, req)
	if err != nil {
		t.mu.Lock()
		t.extracting = false
		t.mu.Unlock()
		return err
	}

	resp, err := stream.Accumulate()
	if err != nil {
		t.mu.Lock()
		t.extracting = false
		t.mu.Unlock()
		return err
	}

	// Extract text content from response
	var summary string
	for _, block := range resp.Content {
		if block.Type == "text" {
			summary += block.Text
		}
	}
	if summary == "" {
		return nil
	}

	// Write summary to session-memory directory
	memDir := filepath.Join(t.sessionDir, "session-memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(memDir, "summary.md"), []byte(summary), 0o644)
}
