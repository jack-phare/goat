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

// toRequestModel prepends the LiteLLM provider prefix.
func toRequestModel(model string) string {
	if strings.HasPrefix(model, "anthropic/") {
		return model
	}
	return "anthropic/" + model
}

// fromResponseModel strips the LiteLLM provider prefix.
func fromResponseModel(model string) string {
	return strings.TrimPrefix(model, "anthropic/")
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
