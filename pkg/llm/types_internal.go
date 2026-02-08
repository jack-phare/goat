package llm

import "github.com/jg-phare/goat/pkg/types"

// CompletionResponse is the accumulated result of a streaming completion.
type CompletionResponse struct {
	ID           string               // Message ID (e.g. "chatcmpl-xxx")
	Model        string               // Actual model used (from response)
	Content      []types.ContentBlock // Accumulated content blocks (text, tool_use, thinking)
	ToolCalls    []ToolCall           // Extracted tool calls (OpenAI format, for reference)
	FinishReason string               // OpenAI finish_reason: "stop"|"tool_calls"|"length"
	StopReason   string               // Translated Anthropic stop_reason: "end_turn"|"tool_use"|"max_tokens"
	Usage        types.BetaUsage      // Token usage (translated to Anthropic format)
}
