package types

import (
	"encoding/json"
	"strings"
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

// --- Test Parity: ThinkingBlock Unmarshal (ported from Python Agent SDK) ---

func TestUnmarshalSDKMessage_AssistantMessageThinking(t *testing.T) {
	// JSON with type="assistant", content array containing thinking + text blocks.
	// Verifies thinking blocks survive JSON round-trip through UnmarshalSDKMessage.
	orig := AssistantMessage{
		BaseMessage: BaseMessage{UUID: uuid.New(), SessionID: "s1"},
		Type:        MessageTypeAssistant,
		Message: BetaMessage{
			ID: "msg_1", Type: "message", Role: "assistant",
			Content: []ContentBlock{
				{Type: "thinking", Thinking: "Let me analyze this problem step by step..."},
				{Type: "text", Text: "Here's my answer based on analysis"},
			},
			Usage: BetaUsage{InputTokens: 200, OutputTokens: 100},
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

	if len(am.Message.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(am.Message.Content))
	}

	// Verify thinking block
	if am.Message.Content[0].Type != "thinking" {
		t.Errorf("content[0].Type = %q, want thinking", am.Message.Content[0].Type)
	}
	if am.Message.Content[0].Thinking != "Let me analyze this problem step by step..." {
		t.Errorf("content[0].Thinking = %q", am.Message.Content[0].Thinking)
	}

	// Verify text block
	if am.Message.Content[1].Type != "text" {
		t.Errorf("content[1].Type = %q, want text", am.Message.Content[1].Type)
	}
	if am.Message.Content[1].Text != "Here's my answer based on analysis" {
		t.Errorf("content[1].Text = %q", am.Message.Content[1].Text)
	}

	// Verify ordering is preserved
	if am.Message.Content[0].Type != "thinking" || am.Message.Content[1].Type != "text" {
		t.Error("content block ordering not preserved: expected thinking then text")
	}
}

func TestContentBlock_UnmarshalJSON_Thinking(t *testing.T) {
	// Direct ContentBlock unmarshal from JSON
	data := []byte(`{"type":"thinking","thinking":"deep thought"}`)
	var cb ContentBlock
	if err := json.Unmarshal(data, &cb); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if cb.Type != "thinking" {
		t.Errorf("Type = %q, want thinking", cb.Type)
	}
	if cb.Thinking != "deep thought" {
		t.Errorf("Thinking = %q, want 'deep thought'", cb.Thinking)
	}

	// Verify other fields are zero values
	if cb.Text != "" {
		t.Errorf("Text should be empty, got %q", cb.Text)
	}
	if cb.ID != "" {
		t.Errorf("ID should be empty, got %q", cb.ID)
	}
	if cb.Name != "" {
		t.Errorf("Name should be empty, got %q", cb.Name)
	}
	if cb.Input != nil {
		t.Errorf("Input should be nil, got %v", cb.Input)
	}
}

// --- Stub Tests for Unimplemented Features ---

func TestUnmarshalSDKMessage_PartialMessageStreaming(t *testing.T) {
	t.Skip("not yet implemented: partial message streaming (content_block_delta)")
}

// --- Test Parity: Message Parser (ported from Python Agent SDK) ---

func TestUnmarshalSDKMessage_UserMessageUUID(t *testing.T) {
	// Verify UUID field is preserved through marshal/unmarshal
	id := uuid.New()
	orig := UserMessage{
		BaseMessage: BaseMessage{UUID: id, SessionID: "s1"},
		Type:        MessageTypeUser,
		Message:     MessageParam{Role: "user", Content: "hello"},
	}
	data := mustMarshal(t, orig)

	msg, err := UnmarshalSDKMessage(data)
	if err != nil {
		t.Fatalf("UnmarshalSDKMessage: %v", err)
	}
	um := msg.(*UserMessage)
	if um.UUID != id {
		t.Errorf("UUID = %s, want %s", um.UUID, id)
	}
	if um.GetSessionID() != "s1" {
		t.Errorf("SessionID = %q, want s1", um.GetSessionID())
	}
}

func TestUnmarshalSDKMessage_UserMessageToolResult(t *testing.T) {
	// UserMessage with content containing tool result blocks
	orig := UserMessage{
		BaseMessage: BaseMessage{UUID: uuid.New(), SessionID: "s1"},
		Type:        MessageTypeUser,
		Message: MessageParam{
			Role: "user",
			Content: []map[string]any{
				{"type": "tool_result", "tool_use_id": "call_123", "content": "tool output here"},
			},
		},
	}
	data := mustMarshal(t, orig)

	msg, err := UnmarshalSDKMessage(data)
	if err != nil {
		t.Fatalf("UnmarshalSDKMessage: %v", err)
	}
	um := msg.(*UserMessage)
	content, ok := um.Message.Content.([]any)
	if !ok {
		t.Fatalf("expected []any content, got %T", um.Message.Content)
	}
	if len(content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(content))
	}
	block, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("expected map content block, got %T", content[0])
	}
	if block["type"] != "tool_result" {
		t.Errorf("block type = %q, want tool_result", block["type"])
	}
	if block["tool_use_id"] != "call_123" {
		t.Errorf("tool_use_id = %q, want call_123", block["tool_use_id"])
	}
}

func TestUnmarshalSDKMessage_UserMessageToolResultError(t *testing.T) {
	// ToolResult with is_error=true
	orig := UserMessage{
		BaseMessage: BaseMessage{UUID: uuid.New(), SessionID: "s1"},
		Type:        MessageTypeUser,
		Message: MessageParam{
			Role: "user",
			Content: []map[string]any{
				{"type": "tool_result", "tool_use_id": "call_456", "content": "error occurred", "is_error": true},
			},
		},
	}
	data := mustMarshal(t, orig)

	msg, err := UnmarshalSDKMessage(data)
	if err != nil {
		t.Fatalf("UnmarshalSDKMessage: %v", err)
	}
	um := msg.(*UserMessage)
	content := um.Message.Content.([]any)
	block := content[0].(map[string]any)
	if block["is_error"] != true {
		t.Errorf("is_error = %v, want true", block["is_error"])
	}
}

func TestUnmarshalSDKMessage_UserMessageMixedContent(t *testing.T) {
	// UserMessage with 4 mixed content blocks in order
	orig := UserMessage{
		BaseMessage: BaseMessage{UUID: uuid.New(), SessionID: "s1"},
		Type:        MessageTypeUser,
		Message: MessageParam{
			Role: "user",
			Content: []map[string]any{
				{"type": "text", "text": "first"},
				{"type": "tool_result", "tool_use_id": "call_1", "content": "output1"},
				{"type": "text", "text": "middle"},
				{"type": "tool_result", "tool_use_id": "call_2", "content": "output2"},
			},
		},
	}
	data := mustMarshal(t, orig)

	msg, err := UnmarshalSDKMessage(data)
	if err != nil {
		t.Fatalf("UnmarshalSDKMessage: %v", err)
	}
	um := msg.(*UserMessage)
	content := um.Message.Content.([]any)
	if len(content) != 4 {
		t.Fatalf("expected 4 content blocks, got %d", len(content))
	}
	// Verify order preserved
	block0 := content[0].(map[string]any)
	block1 := content[1].(map[string]any)
	if block0["type"] != "text" {
		t.Errorf("block[0] type = %q, want text", block0["type"])
	}
	if block1["type"] != "tool_result" {
		t.Errorf("block[1] type = %q, want tool_result", block1["type"])
	}
}

func TestUnmarshalSDKMessage_UserMessageParentToolUse(t *testing.T) {
	// UserMessage inside a subagent with parent_tool_use_id
	parentID := "toolu_parent_123"
	orig := UserMessage{
		BaseMessage:     BaseMessage{UUID: uuid.New(), SessionID: "s1"},
		Type:            MessageTypeUser,
		Message:         MessageParam{Role: "user", Content: "subagent input"},
		ParentToolUseID: &parentID,
	}
	data := mustMarshal(t, orig)

	msg, err := UnmarshalSDKMessage(data)
	if err != nil {
		t.Fatalf("UnmarshalSDKMessage: %v", err)
	}
	um := msg.(*UserMessage)
	if um.ParentToolUseID == nil {
		t.Fatal("expected non-nil ParentToolUseID")
	}
	if *um.ParentToolUseID != parentID {
		t.Errorf("ParentToolUseID = %q, want %q", *um.ParentToolUseID, parentID)
	}
}

func TestUnmarshalSDKMessage_UserMessageToolUseResult(t *testing.T) {
	// UserMessage with nested ToolUseResult metadata
	orig := UserMessage{
		BaseMessage: BaseMessage{UUID: uuid.New(), SessionID: "s1"},
		Type:        MessageTypeUser,
		Message:     MessageParam{Role: "user", Content: "done editing"},
		ToolUseResult: map[string]any{
			"type":     "tool_result",
			"tool_name": "Edit",
			"old_file":  "original.go",
			"new_file":  "modified.go",
		},
	}
	data := mustMarshal(t, orig)

	msg, err := UnmarshalSDKMessage(data)
	if err != nil {
		t.Fatalf("UnmarshalSDKMessage: %v", err)
	}
	um := msg.(*UserMessage)
	tur, ok := um.ToolUseResult.(map[string]any)
	if !ok {
		t.Fatalf("ToolUseResult type = %T, want map[string]any", um.ToolUseResult)
	}
	if tur["tool_name"] != "Edit" {
		t.Errorf("tool_name = %v, want Edit", tur["tool_name"])
	}
}

func TestUnmarshalSDKMessage_UserMessageStringContent(t *testing.T) {
	// Content as a plain string (not array)
	data := []byte(`{"type":"user","uuid":"` + uuid.New().String() + `","session_id":"s1","message":{"role":"user","content":"plain string content"}}`)

	msg, err := UnmarshalSDKMessage(data)
	if err != nil {
		t.Fatalf("UnmarshalSDKMessage: %v", err)
	}
	um := msg.(*UserMessage)
	str, ok := um.Message.Content.(string)
	if !ok {
		t.Fatalf("expected string content, got %T", um.Message.Content)
	}
	if str != "plain string content" {
		t.Errorf("content = %q, want 'plain string content'", str)
	}
}

func TestUnmarshalSDKMessage_AssistantMessageParentToolUse(t *testing.T) {
	// AssistantMessage inside a subagent with parent_tool_use_id
	parentID := "toolu_parent_456"
	orig := AssistantMessage{
		BaseMessage:     BaseMessage{UUID: uuid.New(), SessionID: "s1"},
		Type:            MessageTypeAssistant,
		Message:         BetaMessage{ID: "msg_1", Type: "message", Role: "assistant", Content: []ContentBlock{{Type: "text", Text: "subagent response"}}},
		ParentToolUseID: &parentID,
	}
	data := mustMarshal(t, orig)

	msg, err := UnmarshalSDKMessage(data)
	if err != nil {
		t.Fatalf("UnmarshalSDKMessage: %v", err)
	}
	am := msg.(*AssistantMessage)
	if am.ParentToolUseID == nil {
		t.Fatal("expected non-nil ParentToolUseID")
	}
	if *am.ParentToolUseID != parentID {
		t.Errorf("ParentToolUseID = %q, want %q", *am.ParentToolUseID, parentID)
	}
}

func TestUnmarshalSDKMessage_AssistantMessageError(t *testing.T) {
	// AssistantMessage with various error types
	tests := []struct {
		name  string
		err   AssistantError
	}{
		{"authentication_failed", ErrAuthenticationFailed},
		{"rate_limit", ErrRateLimit},
		{"unknown", ErrUnknown},
		{"billing_error", ErrBillingError},
		{"server_error", ErrServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errVal := tt.err
			orig := AssistantMessage{
				BaseMessage: BaseMessage{UUID: uuid.New(), SessionID: "s1"},
				Type:        MessageTypeAssistant,
				Message:     BetaMessage{ID: "msg_err", Type: "message", Role: "assistant"},
				Error:       &errVal,
			}
			data := mustMarshal(t, orig)

			msg, err := UnmarshalSDKMessage(data)
			if err != nil {
				t.Fatalf("UnmarshalSDKMessage: %v", err)
			}
			am := msg.(*AssistantMessage)
			if am.Error == nil {
				t.Fatal("expected non-nil Error")
			}
			if *am.Error != tt.err {
				t.Errorf("Error = %q, want %q", *am.Error, tt.err)
			}
		})
	}
}

func TestUnmarshalSDKMessage_ErrorContainsData(t *testing.T) {
	// When UnmarshalSDKMessage gets an unknown type, the error should be informative
	data := []byte(`{"type":"bogus_type_12345","uuid":"abc","extra":"stuff"}`)
	_, err := UnmarshalSDKMessage(data)
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
	// Error message should contain the type that was problematic
	errStr := err.Error()
	if !strings.Contains(errStr, "bogus_type_12345") {
		t.Errorf("error %q should contain the unknown type 'bogus_type_12345'", errStr)
	}
}

func TestUnmarshalSDKMessage_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
	}{
		{
			name:    "user_missing_message",
			json:    `{"type":"user"}`,
			wantErr: false, // Go zero-value: no error, just empty MessageParam
		},
		{
			name:    "assistant_missing_message",
			json:    `{"type":"assistant"}`,
			wantErr: false, // Go zero-value: BetaMessage will be zero
		},
		{
			name:    "result_missing_subtype",
			json:    `{"type":"result"}`,
			wantErr: false, // Subtype will be zero value ""
		},
		{
			name:    "missing_type_entirely",
			json:    `{}`,
			wantErr: true, // empty type → unknown
		},
		{
			name:    "system_missing_subtype",
			json:    `{"type":"system"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := UnmarshalSDKMessage([]byte(tt.json))
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
