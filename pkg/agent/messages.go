package agent

import (
	"encoding/json"

	"github.com/jg-phare/goat/pkg/llm"
	"github.com/jg-phare/goat/pkg/types"
)

// responseToAssistantMessage converts a CompletionResponse to an OpenAI assistant ChatMessage
// for inclusion in the conversation history.
func responseToAssistantMessage(resp *llm.CompletionResponse) llm.ChatMessage {
	cm := llm.ChatMessage{Role: "assistant"}

	var text string
	var toolBlocks []types.ContentBlock

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			text = block.Text
		case "tool_use":
			toolBlocks = append(toolBlocks, block)
		}
	}

	if text != "" {
		cm.Content = text
	}

	for _, block := range toolBlocks {
		args, _ := json.Marshal(block.Input)
		cm.ToolCalls = append(cm.ToolCalls, llm.ToolCall{
			ID:   block.ID,
			Type: "function",
			Function: llm.FunctionCall{
				Name:      block.Name,
				Arguments: string(args),
			},
		})
	}

	return cm
}

// toolResultsToMessages converts tool execution results to OpenAI "tool" role messages.
func toolResultsToMessages(results []llm.ToolResult) []llm.ChatMessage {
	return llm.ConvertToToolMessages(results)
}

// extractToolUseBlocks pulls out all tool_use content blocks from a CompletionResponse.
func extractToolUseBlocks(resp *llm.CompletionResponse) []types.ContentBlock {
	var blocks []types.ContentBlock
	for _, b := range resp.Content {
		if b.Type == "tool_use" {
			blocks = append(blocks, b)
		}
	}
	return blocks
}
