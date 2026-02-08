package context

import (
	"strings"
	"testing"

	"github.com/jg-phare/goat/pkg/llm"
)

func TestSimpleEstimator_Estimate(t *testing.T) {
	est := &SimpleEstimator{}
	tests := []struct {
		name string
		text string
		want int
	}{
		{"empty", "", 0},
		{"short", "hi", 0},
		{"4 chars", "abcd", 1},
		{"100 chars", strings.Repeat("a", 100), 25},
		{"1000 chars", strings.Repeat("a", 1000), 250},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := est.Estimate(tt.text)
			if got != tt.want {
				t.Errorf("Estimate(%q) = %d, want %d", tt.text[:min(len(tt.text), 20)], got, tt.want)
			}
		})
	}
}

func TestSimpleEstimator_EstimateMessages(t *testing.T) {
	est := &SimpleEstimator{}
	tests := []struct {
		name string
		msgs []llm.ChatMessage
		want int
	}{
		{"empty", nil, 0},
		{"single user message", []llm.ChatMessage{
			{Role: "user", Content: strings.Repeat("a", 100)},
		}, 29}, // 25 (text) + 4 (overhead)
		{"two messages", []llm.ChatMessage{
			{Role: "user", Content: strings.Repeat("a", 100)},
			{Role: "assistant", Content: strings.Repeat("b", 200)},
		}, 79}, // (25+4) + (50+4) - wait that's 83, let me recalculate
		// 100/4 = 25 + 4 = 29, 200/4 = 50 + 4 = 54, total = 83
		{"nil content", []llm.ChatMessage{
			{Role: "assistant", Content: nil},
		}, 4}, // 0 + 4 overhead
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := est.EstimateMessages(tt.msgs)
			if tt.name == "two messages" {
				if got != 83 {
					t.Errorf("EstimateMessages() = %d, want 83", got)
				}
				return
			}
			if got != tt.want {
				t.Errorf("EstimateMessages() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestContentString(t *testing.T) {
	tests := []struct {
		name    string
		msg     llm.ChatMessage
		want    string
	}{
		{"string content", llm.ChatMessage{Content: "hello"}, "hello"},
		{"nil content", llm.ChatMessage{Content: nil}, ""},
		{"empty string", llm.ChatMessage{Content: ""}, ""},
		{"array content with text", llm.ChatMessage{
			Content: []any{
				map[string]any{"type": "text", "text": "hello "},
				map[string]any{"type": "text", "text": "world"},
			},
		}, "hello world"},
		{"array content no text", llm.ChatMessage{
			Content: []any{
				map[string]any{"type": "image_url"},
			},
		}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ContentString(tt.msg)
			if got != tt.want {
				t.Errorf("ContentString() = %q, want %q", got, tt.want)
			}
		})
	}
}
