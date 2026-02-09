package agent

import (
	"strings"
	"testing"

	"github.com/jg-phare/goat/pkg/llm"
)

func TestPruneOldToolResults_TruncatesOld(t *testing.T) {
	longContent := strings.Repeat("x", 2000)
	msgs := []llm.ChatMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "I'll check."},
		{Role: "tool", ToolCallID: "call_1", Content: longContent},
		{Role: "assistant", Content: "Done."},
	}

	result := pruneOldToolResults(msgs, 1)

	// The tool message (index 2) is outside the preserved window (last 1 msg)
	content, _ := result[2].Content.(string)
	if len(content) > 300 {
		t.Errorf("expected tool content to be truncated, got %d chars", len(content))
	}
	if !strings.Contains(content, "[output truncated]") {
		t.Error("expected truncation indicator")
	}
}

func TestPruneOldToolResults_PreservesRecent(t *testing.T) {
	longContent := strings.Repeat("x", 2000)
	msgs := []llm.ChatMessage{
		{Role: "user", Content: "hello"},
		{Role: "tool", ToolCallID: "call_1", Content: longContent},
	}

	result := pruneOldToolResults(msgs, 10) // preserve more than we have

	// Nothing should be truncated since everything is within the preserve window
	content, _ := result[1].Content.(string)
	if len(content) != 2000 {
		t.Errorf("expected content to be preserved, got %d chars", len(content))
	}
}

func TestPruneOldToolResults_ShortContentUntouched(t *testing.T) {
	msgs := []llm.ChatMessage{
		{Role: "user", Content: "hello"},
		{Role: "tool", ToolCallID: "call_1", Content: "short output"},
	}

	result := pruneOldToolResults(msgs, 0) // preserve nothing

	content, _ := result[1].Content.(string)
	if content != "short output" {
		t.Errorf("expected short content unchanged, got %q", content)
	}
}

func TestPruneOldToolResults_PreservesToolCallID(t *testing.T) {
	longContent := strings.Repeat("x", 2000)
	msgs := []llm.ChatMessage{
		{Role: "tool", ToolCallID: "call_abc", Content: longContent},
	}

	result := pruneOldToolResults(msgs, 0)

	if result[0].ToolCallID != "call_abc" {
		t.Errorf("expected ToolCallID preserved, got %q", result[0].ToolCallID)
	}
}

func TestPruneOldToolResults_NegativePreserve(t *testing.T) {
	longContent := strings.Repeat("x", 2000)
	msgs := []llm.ChatMessage{
		{Role: "tool", ToolCallID: "call_1", Content: longContent},
	}

	result := pruneOldToolResults(msgs, -5)

	content, _ := result[0].Content.(string)
	if !strings.Contains(content, "[output truncated]") {
		t.Error("negative preserveRecent should default to 0")
	}
}
