package types

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return data
}

func TestUnmarshalSDKMessage_AssistantMessage(t *testing.T) {
	orig := AssistantMessage{
		BaseMessage: BaseMessage{UUID: uuid.New(), SessionID: "s1"},
		Type:        MessageTypeAssistant,
		Message: BetaMessage{
			ID: "msg_1", Type: "message", Role: "assistant",
			Content: []ContentBlock{{Type: "text", Text: "hello"}},
			Usage:   BetaUsage{InputTokens: 10, OutputTokens: 5},
		},
	}
	data := mustMarshal(t, orig)

	msg, err := UnmarshalSDKMessage(data)
	if err != nil {
		t.Fatalf("UnmarshalSDKMessage: %v", err)
	}
	am, ok := msg.(*AssistantMessage)
	if !ok {
		t.Fatalf("expected *AssistantMessage, got %T", msg)
	}
	if am.GetType() != MessageTypeAssistant {
		t.Errorf("GetType() = %q", am.GetType())
	}
	if am.Message.ID != "msg_1" {
		t.Errorf("Message.ID = %q", am.Message.ID)
	}
	if am.UUID != orig.UUID {
		t.Errorf("UUID mismatch")
	}
}

func TestUnmarshalSDKMessage_UserMessage(t *testing.T) {
	orig := UserMessage{
		BaseMessage: BaseMessage{UUID: uuid.New(), SessionID: "s1"},
		Type:        MessageTypeUser,
		Message:     MessageParam{Role: "user", Content: "hi"},
	}
	data := mustMarshal(t, orig)

	msg, err := UnmarshalSDKMessage(data)
	if err != nil {
		t.Fatalf("UnmarshalSDKMessage: %v", err)
	}
	um, ok := msg.(*UserMessage)
	if !ok {
		t.Fatalf("expected *UserMessage, got %T", msg)
	}
	if um.GetType() != MessageTypeUser {
		t.Errorf("GetType() = %q", um.GetType())
	}
}

func TestUnmarshalSDKMessage_UserMessageReplay(t *testing.T) {
	orig := UserMessageReplay{
		UserMessage: UserMessage{
			BaseMessage: BaseMessage{UUID: uuid.New(), SessionID: "s1"},
			Type:        MessageTypeUser,
			Message:     MessageParam{Role: "user", Content: "replayed"},
		},
		IsReplay: true,
	}
	data := mustMarshal(t, orig)

	msg, err := UnmarshalSDKMessage(data)
	if err != nil {
		t.Fatalf("UnmarshalSDKMessage: %v", err)
	}
	replay, ok := msg.(*UserMessageReplay)
	if !ok {
		t.Fatalf("expected *UserMessageReplay, got %T", msg)
	}
	if !replay.IsReplay {
		t.Error("IsReplay should be true")
	}
	if replay.GetType() != MessageTypeUser {
		t.Errorf("GetType() = %q", replay.GetType())
	}
}

func TestUnmarshalSDKMessage_ResultMessage(t *testing.T) {
	orig := ResultMessage{
		BaseMessage:  BaseMessage{UUID: uuid.New(), SessionID: "s1"},
		Type:         MessageTypeResult,
		Subtype:      ResultSubtypeSuccess,
		Result:       "done",
		TotalCostUSD: 0.05,
		NumTurns:     3,
	}
	data := mustMarshal(t, orig)

	msg, err := UnmarshalSDKMessage(data)
	if err != nil {
		t.Fatalf("UnmarshalSDKMessage: %v", err)
	}
	rm, ok := msg.(*ResultMessage)
	if !ok {
		t.Fatalf("expected *ResultMessage, got %T", msg)
	}
	if rm.GetType() != MessageTypeResult {
		t.Errorf("GetType() = %q", rm.GetType())
	}
	if rm.Result != "done" {
		t.Errorf("Result = %q", rm.Result)
	}
}

func TestUnmarshalSDKMessage_PartialAssistantMessage(t *testing.T) {
	orig := PartialAssistantMessage{
		BaseMessage: BaseMessage{UUID: uuid.New(), SessionID: "s1"},
		Type:        MessageTypeStreamEvent,
		Event:       map[string]any{"type": "content_block_delta"},
	}
	data := mustMarshal(t, orig)

	msg, err := UnmarshalSDKMessage(data)
	if err != nil {
		t.Fatalf("UnmarshalSDKMessage: %v", err)
	}
	if _, ok := msg.(*PartialAssistantMessage); !ok {
		t.Fatalf("expected *PartialAssistantMessage, got %T", msg)
	}
}

func TestUnmarshalSDKMessage_ToolProgressMessage(t *testing.T) {
	orig := ToolProgressMessage{
		BaseMessage:        BaseMessage{UUID: uuid.New(), SessionID: "s1"},
		Type:               MessageTypeToolProgress,
		ToolUseID:          "tu_1",
		ToolName:           "Bash",
		ElapsedTimeSeconds: 5.5,
	}
	data := mustMarshal(t, orig)

	msg, err := UnmarshalSDKMessage(data)
	if err != nil {
		t.Fatalf("UnmarshalSDKMessage: %v", err)
	}
	tp, ok := msg.(*ToolProgressMessage)
	if !ok {
		t.Fatalf("expected *ToolProgressMessage, got %T", msg)
	}
	if tp.ToolName != "Bash" {
		t.Errorf("ToolName = %q", tp.ToolName)
	}
}

func TestUnmarshalSDKMessage_AuthStatusMessage(t *testing.T) {
	orig := AuthStatusMessage{
		BaseMessage:      BaseMessage{UUID: uuid.New(), SessionID: "s1"},
		Type:             MessageTypeAuthStatus,
		IsAuthenticating: true,
		Output:           []string{"authenticating..."},
	}
	data := mustMarshal(t, orig)

	msg, err := UnmarshalSDKMessage(data)
	if err != nil {
		t.Fatalf("UnmarshalSDKMessage: %v", err)
	}
	as, ok := msg.(*AuthStatusMessage)
	if !ok {
		t.Fatalf("expected *AuthStatusMessage, got %T", msg)
	}
	if !as.IsAuthenticating {
		t.Error("IsAuthenticating should be true")
	}
}

func TestUnmarshalSDKMessage_ToolUseSummaryMessage(t *testing.T) {
	orig := ToolUseSummaryMessage{
		BaseMessage:         BaseMessage{UUID: uuid.New(), SessionID: "s1"},
		Type:                MessageTypeToolUseSummary,
		Summary:             "ran bash",
		PrecedingToolUseIDs: []string{"tu_1", "tu_2"},
	}
	data := mustMarshal(t, orig)

	msg, err := UnmarshalSDKMessage(data)
	if err != nil {
		t.Fatalf("UnmarshalSDKMessage: %v", err)
	}
	ts, ok := msg.(*ToolUseSummaryMessage)
	if !ok {
		t.Fatalf("expected *ToolUseSummaryMessage, got %T", msg)
	}
	if ts.Summary != "ran bash" {
		t.Errorf("Summary = %q", ts.Summary)
	}
}

func TestUnmarshalSDKMessage_SystemSubtypes(t *testing.T) {
	tests := []struct {
		name    string
		msg     SDKMessage
		subtype SystemSubtype
	}{
		{
			name: "init",
			msg: &SystemInitMessage{
				BaseMessage: BaseMessage{UUID: uuid.New(), SessionID: "s1"},
				Type:        MessageTypeSystem,
				Subtype:     SystemSubtypeInit,
				Model:       "claude",
			},
			subtype: SystemSubtypeInit,
		},
		{
			name: "status",
			msg: &StatusMessage{
				BaseMessage: BaseMessage{UUID: uuid.New(), SessionID: "s1"},
				Type:        MessageTypeSystem,
				Subtype:     SystemSubtypeStatus,
			},
			subtype: SystemSubtypeStatus,
		},
		{
			name: "compact_boundary",
			msg: &CompactBoundaryMessage{
				BaseMessage:     BaseMessage{UUID: uuid.New(), SessionID: "s1"},
				Type:            MessageTypeSystem,
				Subtype:         SystemSubtypeCompactBoundary,
				CompactMetadata: CompactMetadata{Trigger: "auto", PreTokens: 50000},
			},
			subtype: SystemSubtypeCompactBoundary,
		},
		{
			name: "hook_started",
			msg: &HookStartedMessage{
				BaseMessage: BaseMessage{UUID: uuid.New(), SessionID: "s1"},
				Type:        MessageTypeSystem,
				Subtype:     SystemSubtypeHookStarted,
				HookID:      "h1",
			},
			subtype: SystemSubtypeHookStarted,
		},
		{
			name: "hook_progress",
			msg: &HookProgressMessage{
				BaseMessage: BaseMessage{UUID: uuid.New(), SessionID: "s1"},
				Type:        MessageTypeSystem,
				Subtype:     SystemSubtypeHookProgress,
				HookID:      "h1",
			},
			subtype: SystemSubtypeHookProgress,
		},
		{
			name: "hook_response",
			msg: &HookResponseMessage{
				BaseMessage: BaseMessage{UUID: uuid.New(), SessionID: "s1"},
				Type:        MessageTypeSystem,
				Subtype:     SystemSubtypeHookResponse,
				HookID:      "h1",
				Outcome:     "success",
			},
			subtype: SystemSubtypeHookResponse,
		},
		{
			name: "files_persisted",
			msg: &FilesPersistedEvent{
				BaseMessage: BaseMessage{UUID: uuid.New(), SessionID: "s1"},
				Type:        MessageTypeSystem,
				Subtype:     SystemSubtypeFilesPersisted,
				ProcessedAt: "2026-01-01T00:00:00Z",
			},
			subtype: SystemSubtypeFilesPersisted,
		},
		{
			name: "task_notification",
			msg: &TaskNotificationMessage{
				BaseMessage: BaseMessage{UUID: uuid.New(), SessionID: "s1"},
				Type:        MessageTypeSystem,
				Subtype:     SystemSubtypeTaskNotification,
				TaskID:      "t1",
				Status:      "completed",
			},
			subtype: SystemSubtypeTaskNotification,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := mustMarshal(t, tt.msg)

			msg, err := UnmarshalSDKMessage(data)
			if err != nil {
				t.Fatalf("UnmarshalSDKMessage: %v", err)
			}
			if msg.GetType() != MessageTypeSystem {
				t.Errorf("GetType() = %q, want system", msg.GetType())
			}
		})
	}
}

func TestUnmarshalSDKMessage_UnknownType(t *testing.T) {
	data := []byte(`{"type":"unknown_type"}`)
	_, err := UnmarshalSDKMessage(data)
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestUnmarshalSDKMessage_SystemMissingSubtype(t *testing.T) {
	data := []byte(`{"type":"system"}`)
	_, err := UnmarshalSDKMessage(data)
	if err == nil {
		t.Fatal("expected error for system message without subtype")
	}
}

func TestUnmarshalSDKMessage_UnknownSystemSubtype(t *testing.T) {
	data := []byte(`{"type":"system","subtype":"unknown_sub"}`)
	_, err := UnmarshalSDKMessage(data)
	if err == nil {
		t.Fatal("expected error for unknown system subtype")
	}
}

func TestUnmarshalSDKMessage_InvalidJSON(t *testing.T) {
	_, err := UnmarshalSDKMessage([]byte(`{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
