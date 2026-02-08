package context

import "github.com/jg-phare/goat/pkg/llm"

// calculateSplitPoint determines where to split the conversation into
// compact zone (to summarize) and preserve zone (to keep verbatim).
// It walks backward from the end, accumulating tokens until the preserve
// budget is exceeded. Returns the index where the preserve zone starts.
//
// The split point is adjusted to never break a tool_use/tool_result pair:
// if the split falls between an assistant message with tool_calls and its
// corresponding tool result messages, the split is moved backward to keep
// the pair together.
func calculateSplitPoint(messages []llm.ChatMessage, preserveBudget int, estimator TokenEstimator) int {
	if len(messages) == 0 {
		return 0
	}

	tokens := 0
	splitIdx := len(messages) // default: compact everything
	for i := len(messages) - 1; i >= 0; i-- {
		tokens += estimator.Estimate(ContentString(messages[i])) + 4 // +4 overhead
		if tokens > preserveBudget {
			splitIdx = i + 1
			break
		}
		if i == 0 {
			// All messages fit in the preserve budget — compact nothing
			return 0
		}
	}

	// Ensure we don't compact everything (keep at least 1 message in preserve zone)
	if splitIdx >= len(messages) {
		splitIdx = len(messages) - 1
	}

	// Ensure at least 1 message in compact zone
	if splitIdx < 1 {
		return 1
	}

	// Adjust split to not break tool_use/tool_result pairs.
	// If the message at splitIdx is a "tool" message (tool result), walk backward
	// to include the preceding assistant message with tool_calls.
	splitIdx = adjustSplitForToolPairs(messages, splitIdx)

	return splitIdx
}

// adjustSplitForToolPairs moves the split index backward if it would
// separate an assistant tool_use message from its tool result messages.
func adjustSplitForToolPairs(messages []llm.ChatMessage, splitIdx int) int {
	if splitIdx >= len(messages) || splitIdx <= 0 {
		return splitIdx
	}

	// If the message at splitIdx is a tool result, we need to find the
	// assistant message that initiated the tool call and include it in the preserve zone.
	for splitIdx > 0 && messages[splitIdx].Role == "tool" {
		splitIdx--
	}

	// If we landed on an assistant message with tool_calls, and the next messages
	// are tool results, include the assistant message in the preserve zone too.
	if splitIdx > 0 && messages[splitIdx].Role == "assistant" && len(messages[splitIdx].ToolCalls) > 0 {
		splitIdx--
	}

	// Ensure we still have at least 1 message in the compact zone
	if splitIdx < 1 {
		return 1
	}

	return splitIdx
}
