package types

import (
	"testing"

	"github.com/google/uuid"
)

func TestNewAssistantMessage(t *testing.T) {
	msg := NewAssistantMessage(
		BetaMessage{ID: "msg_1", Type: "message", Role: "assistant"},
		nil,
		"session-1",
	)

	if msg.UUID == uuid.Nil {
		t.Error("UUID should not be nil")
	}
	if msg.SessionID != "session-1" {
		t.Errorf("SessionID = %q", msg.SessionID)
	}
	if msg.GetType() != MessageTypeAssistant {
		t.Errorf("GetType() = %q", msg.GetType())
	}
	if msg.Message.ID != "msg_1" {
		t.Errorf("Message.ID = %q", msg.Message.ID)
	}
	if msg.ParentToolUseID != nil {
		t.Error("ParentToolUseID should be nil")
	}
}

func TestNewAssistantMessage_WithParent(t *testing.T) {
	parent := "toolu_123"
	msg := NewAssistantMessage(BetaMessage{}, &parent, "s1")

	if msg.ParentToolUseID == nil || *msg.ParentToolUseID != "toolu_123" {
		t.Error("ParentToolUseID not set correctly")
	}
}

func TestNewUserMessage(t *testing.T) {
	msg := NewUserMessage("hello", "session-1")

	if msg.UUID == uuid.Nil {
		t.Error("UUID should not be nil")
	}
	if msg.GetType() != MessageTypeUser {
		t.Errorf("GetType() = %q", msg.GetType())
	}
	if msg.Message.Role != "user" {
		t.Errorf("Message.Role = %q", msg.Message.Role)
	}
	if msg.Message.Content != "hello" {
		t.Errorf("Message.Content = %v", msg.Message.Content)
	}
}

func TestNewResultSuccess(t *testing.T) {
	msg := NewResultSuccess("done", 5, 0.05, BetaUsage{InputTokens: 100}, nil, 1000, 800, "s1")

	if msg.UUID == uuid.Nil {
		t.Error("UUID should not be nil")
	}
	if msg.GetType() != MessageTypeResult {
		t.Errorf("GetType() = %q", msg.GetType())
	}
	if msg.Subtype != ResultSubtypeSuccess {
		t.Errorf("Subtype = %q", msg.Subtype)
	}
	if msg.IsError {
		t.Error("IsError should be false")
	}
	if msg.Result != "done" {
		t.Errorf("Result = %q", msg.Result)
	}
	if msg.NumTurns != 5 {
		t.Errorf("NumTurns = %d", msg.NumTurns)
	}
}

func TestNewResultError(t *testing.T) {
	msg := NewResultError(
		ResultSubtypeErrorMaxTurns,
		[]string{"max turns exceeded"},
		10, 0.10, BetaUsage{}, nil, 5000, 4000, "s1",
	)

	if msg.UUID == uuid.Nil {
		t.Error("UUID should not be nil")
	}
	if msg.Subtype != ResultSubtypeErrorMaxTurns {
		t.Errorf("Subtype = %q", msg.Subtype)
	}
	if !msg.IsError {
		t.Error("IsError should be true")
	}
	if len(msg.Errors) != 1 || msg.Errors[0] != "max turns exceeded" {
		t.Errorf("Errors = %v", msg.Errors)
	}
}

func TestNewSystemInit(t *testing.T) {
	msg := NewSystemInit("claude-opus", "1.0.0", "/home", PermissionModeDefault, "s1")

	if msg.UUID == uuid.Nil {
		t.Error("UUID should not be nil")
	}
	if msg.GetType() != MessageTypeSystem {
		t.Errorf("GetType() = %q", msg.GetType())
	}
	if msg.Subtype != SystemSubtypeInit {
		t.Errorf("Subtype = %q", msg.Subtype)
	}
	if msg.Model != "claude-opus" {
		t.Errorf("Model = %q", msg.Model)
	}
	if msg.ClaudeCodeVersion != "1.0.0" {
		t.Errorf("ClaudeCodeVersion = %q", msg.ClaudeCodeVersion)
	}
}

func TestNewCompactBoundary(t *testing.T) {
	msg := NewCompactBoundary("auto", 50000, "s1")

	if msg.UUID == uuid.Nil {
		t.Error("UUID should not be nil")
	}
	if msg.GetType() != MessageTypeSystem {
		t.Errorf("GetType() = %q", msg.GetType())
	}
	if msg.Subtype != SystemSubtypeCompactBoundary {
		t.Errorf("Subtype = %q", msg.Subtype)
	}
	if msg.CompactMetadata.Trigger != "auto" {
		t.Errorf("Trigger = %q", msg.CompactMetadata.Trigger)
	}
	if msg.CompactMetadata.PreTokens != 50000 {
		t.Errorf("PreTokens = %d", msg.CompactMetadata.PreTokens)
	}
}

func TestConstructors_UniqueUUIDs(t *testing.T) {
	msg1 := NewUserMessage("a", "s1")
	msg2 := NewUserMessage("b", "s1")

	if msg1.UUID == msg2.UUID {
		t.Error("two constructors should produce different UUIDs")
	}
}
