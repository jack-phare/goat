package llm

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jg-phare/goat/pkg/types"
)

func TestEmitStreamEvent(t *testing.T) {
	content := "Hello"
	chunk := &StreamChunk{
		ID:      "chatcmpl-1",
		Model:   "claude",
		Created: 1234,
		Choices: []Choice{
			{Index: 0, Delta: Delta{Content: &content}},
		},
	}

	parentID := "toolu_123"
	msg := EmitStreamEvent(chunk, &parentID, "session-1")

	if msg.Type != types.MessageTypeStreamEvent {
		t.Errorf("Type = %q, want stream_event", msg.Type)
	}
	if msg.UUID == uuid.Nil {
		t.Error("UUID should not be nil")
	}
	if msg.SessionID != "session-1" {
		t.Errorf("SessionID = %q", msg.SessionID)
	}
	if msg.ParentToolUseID == nil || *msg.ParentToolUseID != "toolu_123" {
		t.Error("ParentToolUseID not forwarded")
	}
	if msg.Event == nil {
		t.Error("Event should not be nil")
	}
}

func TestEmitAssistantMessage(t *testing.T) {
	t.Run("success case", func(t *testing.T) {
		resp := &CompletionResponse{
			ID:    "chatcmpl-1",
			Model: "anthropic/claude-opus-4-5-20250514",
			Content: []types.ContentBlock{
				{Type: "text", Text: "Hello!"},
			},
			FinishReason: "stop",
			StopReason:   "end_turn",
			Usage:        types.BetaUsage{InputTokens: 100, OutputTokens: 50},
		}

		msg := EmitAssistantMessage(resp, nil, "session-1", nil)

		if msg.Type != types.MessageTypeAssistant {
			t.Errorf("Type = %q, want assistant", msg.Type)
		}
		if msg.UUID == uuid.Nil {
			t.Error("UUID should not be nil")
		}
		if msg.ParentToolUseID != nil {
			t.Error("ParentToolUseID should be nil for main agent")
		}
		if msg.Error != nil {
			t.Error("Error should be nil on success")
		}
		if msg.SessionID != "session-1" {
			t.Errorf("SessionID = %q", msg.SessionID)
		}

		if msg.Message.ID != "chatcmpl-1" {
			t.Errorf("Message.ID = %q", msg.Message.ID)
		}
	})

	t.Run("error case", func(t *testing.T) {
		resp := &CompletionResponse{
			ID:    "chatcmpl-2",
			Model: "claude",
		}

		errVal := types.ErrRateLimit
		msg := EmitAssistantMessage(resp, nil, "session-1", &errVal)

		if msg.Error == nil || *msg.Error != types.ErrRateLimit {
			t.Errorf("Error = %v, want rate_limit", msg.Error)
		}
	})
}
