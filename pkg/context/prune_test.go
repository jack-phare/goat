package context

import (
	"strings"
	"testing"

	"github.com/jg-phare/goat/pkg/llm"
)

func TestPruneOldToolResults(t *testing.T) {
	longOutput := strings.Repeat("x", 2000) // > 1000 chars

	messages := []llm.ChatMessage{
		{Role: "user", Content: "run ls"},
		{Role: "assistant", Content: "ok"},
		{Role: "tool", ToolCallID: "c1", Content: longOutput},  // old, should be pruned
		{Role: "assistant", Content: "got it"},
		{Role: "tool", ToolCallID: "c2", Content: longOutput},  // recent, preserved
		{Role: "assistant", Content: "done"},
	}

	result := PruneOldToolResults(messages, 2) // preserve last 2

	// Message at index 2 (old tool result) should be truncated
	oldContent := ContentString(result[2])
	if len(oldContent) > 300 {
		t.Errorf("old tool result should be truncated, got length %d", len(oldContent))
	}
	if !strings.Contains(oldContent, "[output truncated]") {
		t.Error("truncated content should have truncation indicator")
	}

	// Message at index 4 (recent tool result) should be preserved
	recentContent := ContentString(result[4])
	if len(recentContent) != 2000 {
		t.Errorf("recent tool result should be preserved, got length %d", len(recentContent))
	}

	// Non-tool messages should be unchanged
	if ContentString(result[0]) != "run ls" {
		t.Error("user message should be unchanged")
	}
}

func TestPruneOldToolResults_ShortContent(t *testing.T) {
	messages := []llm.ChatMessage{
		{Role: "tool", ToolCallID: "c1", Content: "short output"}, // < 1000 chars
	}

	result := PruneOldToolResults(messages, 0)

	// Short content should not be modified
	if ContentString(result[0]) != "short output" {
		t.Error("short tool output should not be modified")
	}
}

func TestPruneOldToolResults_NoToolMessages(t *testing.T) {
	messages := []llm.ChatMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
	}

	result := PruneOldToolResults(messages, 0)

	if len(result) != 2 {
		t.Errorf("expected 2 messages, got %d", len(result))
	}
	if ContentString(result[0]) != "hello" {
		t.Error("message should be unchanged")
	}
}

func TestPruneOldToolResults_NegativePreserve(t *testing.T) {
	longOutput := strings.Repeat("x", 2000)
	messages := []llm.ChatMessage{
		{Role: "tool", ToolCallID: "c1", Content: longOutput},
	}

	result := PruneOldToolResults(messages, -1) // negative treated as 0

	// Should still prune
	if len(ContentString(result[0])) > 300 {
		t.Error("should prune even with negative preserveRecent")
	}
}

func TestTruncateToolOutput(t *testing.T) {
	tests := []struct {
		name    string
		content string
		maxLen  int
		wantLen int
	}{
		{"short", "hello", 200, 5},
		{"exact", strings.Repeat("a", 200), 200, 200},
		{"long", strings.Repeat("a", 500), 200, 200 + len("\n... [output truncated]")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateToolOutput(tt.content, tt.maxLen)
			if len(got) != tt.wantLen {
				t.Errorf("truncateToolOutput length = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}
