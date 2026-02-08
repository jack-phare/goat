package context

import "github.com/jg-phare/goat/pkg/llm"

// PruneOldToolResults replaces verbose tool result content (>1000 chars)
// with truncated versions, except for the most recent `preserveRecent` messages.
// This is a lighter-weight alternative to full compaction.
func PruneOldToolResults(messages []llm.ChatMessage, preserveRecent int) []llm.ChatMessage {
	if preserveRecent < 0 {
		preserveRecent = 0
	}

	result := make([]llm.ChatMessage, len(messages))
	copy(result, messages)

	pruneEnd := len(result) - preserveRecent
	if pruneEnd < 0 {
		pruneEnd = 0
	}

	for i := 0; i < pruneEnd; i++ {
		if result[i].Role == "tool" {
			content := ContentString(result[i])
			if len(content) > 1000 {
				result[i] = llm.ChatMessage{
					Role:       "tool",
					ToolCallID: result[i].ToolCallID,
					Content:    truncateToolOutput(content, 200),
				}
			}
		}
	}

	return result
}

// truncateToolOutput truncates content to maxLen characters, appending an
// ellipsis indicator.
func truncateToolOutput(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen] + "\n... [output truncated]"
}
