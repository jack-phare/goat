package llm

import (
	"encoding/json"

	"github.com/jg-phare/goat/pkg/types"
)

// LoopState carries per-request state from the agentic loop.
type LoopState struct {
	SessionID string
}

// Tool is the interface that tools must implement for request construction.
type Tool interface {
	ToolName() string
	Description() string
	InputSchema() map[string]any
}

// BuildCompletionRequest assembles a full CompletionRequest from loop state.
func BuildCompletionRequest(config ClientConfig, systemPrompt string, messages []ChatMessage, tools []Tool, loopState LoopState) *CompletionRequest {
	req := &CompletionRequest{
		Model:         toRequestModel(config.Model),
		Stream:        true,
		MaxTokens:     config.MaxTokens,
		StreamOptions: &StreamOptions{IncludeUsage: true},
	}

	// System prompt as first message
	req.Messages = append(req.Messages, ChatMessage{
		Role:    "system",
		Content: systemPrompt,
	})

	// Append conversation messages
	req.Messages = append(req.Messages, messages...)

	// Tool definitions
	for _, tool := range tools {
		req.Tools = append(req.Tools, ToolDefinition{
			Type: "function",
			Function: FunctionDef{
				Name:        tool.ToolName(),
				Description: tool.Description(),
				Parameters:  tool.InputSchema(),
			},
		})
	}

	// LiteLLM passthrough for Anthropic-specific fields.
	// Only populated when there are provider-specific fields to send;
	// standard OpenAI-compatible providers reject unknown top-level fields.
	extraBody := map[string]any{}

	if config.MaxThinkingTokens > 0 {
		extraBody["thinking"] = map[string]any{
			"type":          "enabled",
			"budget_tokens": config.MaxThinkingTokens,
		}
	}

	if len(config.Betas) > 0 {
		extraBody["betas"] = config.Betas
	}

	// Only attach metadata when other extra_body fields are present
	// (indicates LiteLLM proxy usage where extra_body is supported).
	if len(extraBody) > 0 && loopState.SessionID != "" {
		extraBody["metadata"] = map[string]any{
			"user_id": loopState.SessionID,
		}
	}

	if len(extraBody) > 0 {
		req.ExtraBody = extraBody
	}

	return req
}

// ConvertToToolMessages converts internal tool_result content blocks to OpenAI "tool" messages.
func ConvertToToolMessages(toolResults []ToolResult) []ChatMessage {
	msgs := make([]ChatMessage, 0, len(toolResults))
	for _, tr := range toolResults {
		msgs = append(msgs, ChatMessage{
			Role:       "tool",
			ToolCallID: tr.ToolUseID,
			Content:    tr.Content,
		})
	}
	return msgs
}

// ToolResult represents a tool execution result for conversion.
type ToolResult struct {
	ToolUseID string
	Content   string
}

// convertAssistantToOpenAI converts internal content blocks to an OpenAI assistant message.
func convertAssistantToOpenAI(textContent string, toolUseBlocks []types.ContentBlock) ChatMessage {
	cm := ChatMessage{Role: "assistant"}

	if textContent != "" {
		cm.Content = textContent
	}

	for _, block := range toolUseBlocks {
		args, _ := json.Marshal(block.Input)
		cm.ToolCalls = append(cm.ToolCalls, ToolCall{
			ID:   block.ID,
			Type: "function",
			Function: FunctionCall{
				Name:      block.Name,
				Arguments: string(args),
			},
		})
	}

	return cm
}
