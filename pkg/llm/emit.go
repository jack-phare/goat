package llm

import (
	"github.com/google/uuid"
	"github.com/jg-phare/goat/pkg/types"
)

// EmitStreamEvent wraps a StreamChunk as an SDKPartialAssistantMessage-equivalent.
func EmitStreamEvent(chunk *StreamChunk, parentToolUseID *string, sessionID string) types.PartialAssistantMessage {
	return types.PartialAssistantMessage{
		BaseMessage:     types.BaseMessage{UUID: uuid.New(), SessionID: sessionID},
		Type:            types.MessageTypeStreamEvent,
		Event:           chunkToStreamEvent(chunk),
		ParentToolUseID: parentToolUseID,
	}
}

// EmitAssistantMessage wraps a CompletionResponse as an SDKAssistantMessage.
func EmitAssistantMessage(resp *CompletionResponse, parentToolUseID *string, sessionID string, err *types.AssistantError) types.AssistantMessage {
	return types.AssistantMessage{
		BaseMessage:     types.BaseMessage{UUID: uuid.New(), SessionID: sessionID},
		Type:            types.MessageTypeAssistant,
		Message:         resp.ToBetaMessage(),
		ParentToolUseID: parentToolUseID,
		Error:           err,
	}
}

// chunkToStreamEvent converts an OpenAI chunk to an Anthropic-equivalent event representation.
func chunkToStreamEvent(chunk *StreamChunk) map[string]any {
	event := map[string]any{
		"id":      chunk.ID,
		"model":   chunk.Model,
		"created": chunk.Created,
	}

	if len(chunk.Choices) > 0 {
		choice := chunk.Choices[0]
		delta := choice.Delta

		if delta.Content != nil {
			event["type"] = "content_block_delta"
			event["delta"] = map[string]any{
				"type": "text_delta",
				"text": *delta.Content,
			}
		} else if delta.ReasoningContent != nil {
			event["type"] = "content_block_delta"
			event["delta"] = map[string]any{
				"type":     "thinking_delta",
				"thinking": *delta.ReasoningContent,
			}
		} else if len(delta.ToolCalls) > 0 {
			event["type"] = "content_block_delta"
			tc := delta.ToolCalls[0]
			event["delta"] = map[string]any{
				"type":         "input_json_delta",
				"partial_json": tc.Function.Arguments,
			}
		} else if choice.FinishReason != nil {
			event["type"] = "message_delta"
			event["delta"] = map[string]any{
				"stop_reason": translateFinishReason(*choice.FinishReason),
			}
		}
	}

	if chunk.Usage != nil {
		event["usage"] = map[string]any{
			"input_tokens":  chunk.Usage.PromptTokens,
			"output_tokens": chunk.Usage.CompletionTokens,
		}
	}

	return event
}
