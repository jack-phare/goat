package llm

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jg-phare/goat/pkg/types"
)

// mockTool implements the Tool interface for testing.
type mockTool struct {
	name        string
	description string
	schema      map[string]any
}

func (m *mockTool) ToolName() string           { return m.name }
func (m *mockTool) Description() string        { return m.description }
func (m *mockTool) InputSchema() map[string]any { return m.schema }

func TestBuildCompletionRequest(t *testing.T) {
	t.Run("basic request", func(t *testing.T) {
		config := ClientConfig{
			Model:     "claude-opus-4-5-20250514",
			MaxTokens: 8192,
		}

		messages := []ChatMessage{
			{Role: "user", Content: "Hello"},
		}

		req := BuildCompletionRequest(config, "You are helpful.", messages, nil, LoopState{})

		if req.Model != "anthropic/claude-opus-4-5-20250514" {
			t.Errorf("Model = %q, want anthropic/ prefix", req.Model)
		}
		if !req.Stream {
			t.Error("Stream should be true")
		}
		if req.MaxTokens != 8192 {
			t.Errorf("MaxTokens = %d, want 8192", req.MaxTokens)
		}
		// System prompt + 1 user message = 2 messages
		if len(req.Messages) != 2 {
			t.Fatalf("len(Messages) = %d, want 2", len(req.Messages))
		}
		if req.Messages[0].Role != "system" {
			t.Errorf("Messages[0].Role = %q, want system", req.Messages[0].Role)
		}
		if req.Messages[0].Content != "You are helpful." {
			t.Errorf("Messages[0].Content = %v", req.Messages[0].Content)
		}
	})

	t.Run("with tools", func(t *testing.T) {
		config := ClientConfig{Model: "claude-opus-4-5-20250514", MaxTokens: 8192}
		tools := []Tool{
			&mockTool{
				name:        "Bash",
				description: "Run a command",
				schema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"command": map[string]any{"type": "string"},
					},
				},
			},
		}

		req := BuildCompletionRequest(config, "sys", nil, tools, LoopState{})

		if len(req.Tools) != 1 {
			t.Fatalf("len(Tools) = %d, want 1", len(req.Tools))
		}
		if req.Tools[0].Type != "function" {
			t.Errorf("Tools[0].Type = %q, want function", req.Tools[0].Type)
		}
		if req.Tools[0].Function.Name != "Bash" {
			t.Errorf("Tools[0].Function.Name = %q, want Bash", req.Tools[0].Function.Name)
		}
	})

	t.Run("extra_body with thinking", func(t *testing.T) {
		config := ClientConfig{
			Model:             "claude-opus-4-5-20250514",
			MaxTokens:         8192,
			MaxThinkingTokens: 10000,
		}

		req := BuildCompletionRequest(config, "sys", nil, nil, LoopState{})

		if req.ExtraBody == nil {
			t.Fatal("ExtraBody should not be nil with MaxThinkingTokens set")
		}
		thinking, ok := req.ExtraBody["thinking"].(map[string]any)
		if !ok {
			t.Fatal("ExtraBody[thinking] not a map")
		}
		if thinking["type"] != "enabled" {
			t.Errorf("thinking.type = %v", thinking["type"])
		}
		if thinking["budget_tokens"] != 10000 {
			t.Errorf("thinking.budget_tokens = %v", thinking["budget_tokens"])
		}
	})

	t.Run("extra_body with betas and metadata", func(t *testing.T) {
		config := ClientConfig{
			Model:     "claude-opus-4-5-20250514",
			MaxTokens: 8192,
			Betas:     []string{"context-1m-2025-08-07"},
		}

		req := BuildCompletionRequest(config, "sys", nil, nil, LoopState{SessionID: "session-123"})

		if req.ExtraBody == nil {
			t.Fatal("ExtraBody should not be nil")
		}

		betas, ok := req.ExtraBody["betas"].([]string)
		if !ok {
			t.Fatal("ExtraBody[betas] not []string")
		}
		if len(betas) != 1 || betas[0] != "context-1m-2025-08-07" {
			t.Errorf("betas = %v", betas)
		}

		metadata, ok := req.ExtraBody["metadata"].(map[string]any)
		if !ok {
			t.Fatal("ExtraBody[metadata] not a map")
		}
		if metadata["user_id"] != "session-123" {
			t.Errorf("metadata.user_id = %v", metadata["user_id"])
		}
	})

	t.Run("extra_body serialization", func(t *testing.T) {
		config := ClientConfig{
			Model:             "claude-opus-4-5-20250514",
			MaxTokens:         8192,
			MaxThinkingTokens: 5000,
			Betas:             []string{"beta-1"},
		}

		req := BuildCompletionRequest(config, "sys", nil, nil, LoopState{SessionID: "sess"})

		// Ensure it serializes properly
		data, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}

		var result map[string]any
		json.Unmarshal(data, &result)

		eb, ok := result["extra_body"].(map[string]any)
		if !ok {
			t.Fatal("extra_body not in serialized JSON")
		}
		// Verify thinking, betas, metadata all present
		if eb["thinking"] == nil {
			t.Error("thinking missing from extra_body")
		}
		if eb["betas"] == nil {
			t.Error("betas missing from extra_body")
		}
		if eb["metadata"] == nil {
			t.Error("metadata missing from extra_body")
		}
	})

	t.Run("no extra_body when not needed", func(t *testing.T) {
		config := ClientConfig{Model: "claude-opus-4-5-20250514", MaxTokens: 8192}
		req := BuildCompletionRequest(config, "sys", nil, nil, LoopState{})

		if req.ExtraBody != nil {
			t.Error("ExtraBody should be nil when no extra fields are set")
		}
	})
}

func TestToolResult_MetadataDoesNotAffectConversion(t *testing.T) {
	// Metadata should not appear in converted tool messages
	results := []ToolResult{
		{
			ToolUseID: "call_1",
			Content:   "file content here",
			Metadata: &ToolResultMetadata{
				FilePaths:    []string{"/tmp/foo.go"},
				WasTruncated: false,
				OriginalLen:  17,
			},
		},
		{
			ToolUseID: "call_2",
			Content:   "other content",
			// nil metadata
		},
	}

	msgs := ConvertToToolMessages(results)

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	// Verify content and tool call IDs are preserved
	if msgs[0].ToolCallID != "call_1" || msgs[0].Content != "file content here" {
		t.Errorf("msg[0]: ToolCallID=%q Content=%v", msgs[0].ToolCallID, msgs[0].Content)
	}
	if msgs[1].ToolCallID != "call_2" || msgs[1].Content != "other content" {
		t.Errorf("msg[1]: ToolCallID=%q Content=%v", msgs[1].ToolCallID, msgs[1].Content)
	}

	// Verify metadata is not serialized in the message
	for _, msg := range msgs {
		data, _ := json.Marshal(msg)
		dataStr := string(data)
		if strings.Contains(dataStr, "FilePaths") || strings.Contains(dataStr, "WasTruncated") || strings.Contains(dataStr, "OriginalLen") {
			t.Errorf("metadata leaked into serialized message: %s", dataStr)
		}
	}
}

func TestConvertAssistantToOpenAI(t *testing.T) {
	t.Run("text only", func(t *testing.T) {
		cm := convertAssistantToOpenAI("Hello world", nil)
		if cm.Role != "assistant" {
			t.Errorf("Role = %q", cm.Role)
		}
		if cm.Content != "Hello world" {
			t.Errorf("Content = %v", cm.Content)
		}
		if len(cm.ToolCalls) != 0 {
			t.Errorf("ToolCalls should be empty")
		}
	})

	t.Run("with tool calls", func(t *testing.T) {
		blocks := []types.ContentBlock{
			{Type: "tool_use", ID: "call_1", Name: "Bash", Input: map[string]any{"command": "ls"}},
		}
		cm := convertAssistantToOpenAI("Let me check.", blocks)

		if cm.Content != "Let me check." {
			t.Errorf("Content = %v", cm.Content)
		}
		if len(cm.ToolCalls) != 1 {
			t.Fatalf("len(ToolCalls) = %d, want 1", len(cm.ToolCalls))
		}
		if cm.ToolCalls[0].ID != "call_1" {
			t.Errorf("ToolCalls[0].ID = %q", cm.ToolCalls[0].ID)
		}
		if cm.ToolCalls[0].Function.Name != "Bash" {
			t.Errorf("ToolCalls[0].Function.Name = %q", cm.ToolCalls[0].Function.Name)
		}
		// Verify arguments are JSON string
		var args map[string]any
		json.Unmarshal([]byte(cm.ToolCalls[0].Function.Arguments), &args)
		if args["command"] != "ls" {
			t.Errorf("arguments.command = %v", args["command"])
		}
	})
}
