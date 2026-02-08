package llm

import (
	"testing"

	"github.com/jg-phare/goat/pkg/types"
)

func TestTranslateFinishReason(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"stop", "end_turn"},
		{"tool_calls", "tool_use"},
		{"length", "max_tokens"},
		{"unknown_reason", "unknown_reason"},
		{"", ""},
		{"content_filter", "content_filter"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := translateFinishReason(tt.input)
			if got != tt.expected {
				t.Errorf("translateFinishReason(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestTranslateUsage(t *testing.T) {
	t.Run("nil usage", func(t *testing.T) {
		got := translateUsage(nil)
		if got != (types.BetaUsage{}) {
			t.Errorf("translateUsage(nil) = %+v, want zero BetaUsage", got)
		}
	})

	t.Run("full usage", func(t *testing.T) {
		u := &Usage{
			PromptTokens:             1234,
			CompletionTokens:         567,
			TotalTokens:              1801,
			CacheReadInputTokens:     100,
			CacheCreationInputTokens: 50,
		}
		got := translateUsage(u)
		expected := types.BetaUsage{
			InputTokens:              1234,
			OutputTokens:             567,
			CacheReadInputTokens:     100,
			CacheCreationInputTokens: 50,
		}
		if got != expected {
			t.Errorf("translateUsage() = %+v, want %+v", got, expected)
		}
	})

	t.Run("zero usage", func(t *testing.T) {
		u := &Usage{}
		got := translateUsage(u)
		if got != (types.BetaUsage{}) {
			t.Errorf("translateUsage(&Usage{}) = %+v, want zero BetaUsage", got)
		}
	})
}

func TestToRequestModel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"claude-opus-4-5-20250514", "anthropic/claude-opus-4-5-20250514"},
		{"anthropic/claude-opus-4-5-20250514", "anthropic/claude-opus-4-5-20250514"}, // idempotent
		{"claude-sonnet-4-5-20250929", "anthropic/claude-sonnet-4-5-20250929"},
		{"", "anthropic/"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := toRequestModel(tt.input)
			if got != tt.expected {
				t.Errorf("toRequestModel(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFromResponseModel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"anthropic/claude-opus-4-5-20250514", "claude-opus-4-5-20250514"},
		{"claude-opus-4-5-20250514", "claude-opus-4-5-20250514"}, // no prefix
		{"anthropic/", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := fromResponseModel(tt.input)
			if got != tt.expected {
				t.Errorf("fromResponseModel(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestToBetaMessage(t *testing.T) {
	resp := &CompletionResponse{
		ID:    "chatcmpl-123",
		Model: "anthropic/claude-opus-4-5-20250514",
		Content: []types.ContentBlock{
			{Type: "text", Text: "Hello, world!"},
		},
		FinishReason: "stop",
		StopReason:   "end_turn",
		Usage: types.BetaUsage{
			InputTokens:  100,
			OutputTokens: 50,
		},
	}

	got := resp.ToBetaMessage()

	if got.ID != "chatcmpl-123" {
		t.Errorf("ID = %q, want %q", got.ID, "chatcmpl-123")
	}
	if got.Type != "message" {
		t.Errorf("Type = %q, want %q", got.Type, "message")
	}
	if got.Role != "assistant" {
		t.Errorf("Role = %q, want %q", got.Role, "assistant")
	}
	if got.Model != "claude-opus-4-5-20250514" {
		t.Errorf("Model = %q, want %q", got.Model, "claude-opus-4-5-20250514")
	}
	if got.StopReason == nil || *got.StopReason != "end_turn" {
		t.Errorf("StopReason = %v, want %q", got.StopReason, "end_turn")
	}
	if len(got.Content) != 1 || got.Content[0].Text != "Hello, world!" {
		t.Errorf("Content = %+v, want single text block", got.Content)
	}
	if got.Usage.InputTokens != 100 || got.Usage.OutputTokens != 50 {
		t.Errorf("Usage = %+v, want {100, 50, 0, 0}", got.Usage)
	}
}
