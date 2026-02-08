package context

import (
	"strings"
	"testing"

	"github.com/jg-phare/goat/pkg/llm"
)

func TestCalculateSplitPoint(t *testing.T) {
	est := &SimpleEstimator{}

	tests := []struct {
		name           string
		messages       []llm.ChatMessage
		preserveBudget int
		wantIdx        int
	}{
		{
			"empty messages",
			nil,
			1000,
			0,
		},
		{
			"single message - fits budget",
			[]llm.ChatMessage{{Role: "user", Content: "hi"}},
			1000,
			0, // nothing to compact
		},
		{
			"all messages fit in preserve budget",
			[]llm.ChatMessage{
				{Role: "user", Content: "hello"},
				{Role: "assistant", Content: "hi there"},
			},
			10000,
			0, // nothing to compact
		},
		{
			"split in middle",
			[]llm.ChatMessage{
				{Role: "user", Content: strings.Repeat("a", 400)},     // ~100 tokens + 4 = 104
				{Role: "assistant", Content: strings.Repeat("b", 400)}, // ~100 tokens + 4 = 104
				{Role: "user", Content: strings.Repeat("c", 400)},     // ~100 tokens + 4 = 104
				{Role: "assistant", Content: strings.Repeat("d", 400)}, // ~100 tokens + 4 = 104
			},
			250, // budget for ~2.4 messages → preserve last 2
			2,
		},
		{
			"compact everything except last",
			[]llm.ChatMessage{
				{Role: "user", Content: strings.Repeat("a", 4000)},
				{Role: "assistant", Content: strings.Repeat("b", 4000)},
				{Role: "user", Content: strings.Repeat("c", 400)},
			},
			150, // only last message fits
			2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateSplitPoint(tt.messages, tt.preserveBudget, est)
			if got != tt.wantIdx {
				t.Errorf("calculateSplitPoint() = %d, want %d", got, tt.wantIdx)
			}
		})
	}
}

func TestAdjustSplitForToolPairs(t *testing.T) {
	messages := []llm.ChatMessage{
		{Role: "user", Content: "do stuff"},                       // 0
		{Role: "assistant", Content: "ok", ToolCalls: []llm.ToolCall{{ID: "c1"}}}, // 1
		{Role: "tool", ToolCallID: "c1", Content: "result1"},     // 2
		{Role: "assistant", Content: "done"},                      // 3
		{Role: "user", Content: "next"},                           // 4
	}

	tests := []struct {
		name     string
		splitIdx int
		want     int
	}{
		{"split at user msg - no adjustment", 0, 0},
		{"split at assistant with tools", 1, 1},
		{"split at tool result - adjust backward", 2, 1},
		{"split at regular assistant - no change", 3, 3},
		{"split at end user - no change", 4, 4},
		{"split beyond range", 5, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adjustSplitForToolPairs(messages, tt.splitIdx)
			if got != tt.want {
				t.Errorf("adjustSplitForToolPairs(_, %d) = %d, want %d", tt.splitIdx, got, tt.want)
			}
		})
	}
}

func TestSplitPreservesToolPairs(t *testing.T) {
	est := &SimpleEstimator{}

	// Create messages where the natural split would fall between tool_use and tool_result
	messages := []llm.ChatMessage{
		{Role: "user", Content: strings.Repeat("a", 800)},                                       // 0: ~200 tokens
		{Role: "assistant", Content: strings.Repeat("b", 800)},                                   // 1: ~200 tokens
		{Role: "user", Content: strings.Repeat("c", 800)},                                       // 2: ~200 tokens
		{Role: "assistant", Content: "running", ToolCalls: []llm.ToolCall{{ID: "c1"}}},           // 3: small
		{Role: "tool", ToolCallID: "c1", Content: strings.Repeat("d", 200)},                      // 4: ~50 tokens
		{Role: "assistant", Content: strings.Repeat("e", 200)},                                    // 5: ~50 tokens
	}

	// Budget that would naturally split between msg 3 and 4
	budget := 120 // fits msgs 4+5 but not 3+4+5

	splitIdx := calculateSplitPoint(messages, budget, est)

	// Should not split between msg 3 (tool_use) and msg 4 (tool result)
	// The tool result at index 4 means we need to keep 3 and 4 together
	if splitIdx == 4 {
		t.Error("split should not fall between tool_use and tool_result")
	}
}
