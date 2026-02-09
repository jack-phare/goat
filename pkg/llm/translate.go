package llm

import (
	"strings"

	"github.com/jg-phare/goat/pkg/types"
)

// translateFinishReason converts OpenAI finish_reason to Anthropic stop_reason.
func translateFinishReason(fr string) string {
	switch fr {
	case "stop":
		return "end_turn"
	case "tool_calls":
		return "tool_use"
	case "length":
		return "max_tokens"
	default:
		return fr // pass through unknown values
	}
}

// translateUsage converts OpenAI Usage to Anthropic BetaUsage.
func translateUsage(u *Usage) types.BetaUsage {
	if u == nil {
		return types.BetaUsage{}
	}
	return types.BetaUsage{
		InputTokens:              u.PromptTokens,
		OutputTokens:             u.CompletionTokens,
		CacheReadInputTokens:     u.CacheReadInputTokens,
		CacheCreationInputTokens: u.CacheCreationInputTokens,
	}
}

// toRequestModel prepends the LiteLLM "anthropic/" prefix for bare Claude model names.
// Models that already contain a "/" (provider-prefixed) or non-Claude models pass through as-is.
func toRequestModel(model string) string {
	// Already provider-prefixed (e.g. "anthropic/claude-...", "openai/gpt-4o")
	if strings.Contains(model, "/") {
		return model
	}
	// Bare Claude model name → add anthropic/ prefix for LiteLLM compatibility
	if strings.HasPrefix(model, "claude-") {
		return "anthropic/" + model
	}
	// Non-Claude model (e.g. "gpt-4o-mini", "llama-3.3-70b-versatile") → pass through
	return model
}

// fromResponseModel strips provider prefixes (e.g. "anthropic/", "openai/") from response model IDs.
func fromResponseModel(model string) string {
	if idx := strings.Index(model, "/"); idx >= 0 {
		return model[idx+1:]
	}
	return model
}

// ToBetaMessage converts the accumulated CompletionResponse to an Anthropic-equivalent BetaMessage.
func (r *CompletionResponse) ToBetaMessage() types.BetaMessage {
	stopReason := r.StopReason
	return types.BetaMessage{
		ID:         r.ID,
		Type:       "message",
		Role:       "assistant",
		Content:    r.Content,
		Model:      fromResponseModel(r.Model),
		StopReason: &stopReason,
		Usage:      r.Usage,
	}
}
