package context

import (
	"fmt"

	"github.com/jg-phare/goat/pkg/llm"
)

// TokenEstimator estimates token counts for text and messages.
type TokenEstimator interface {
	Estimate(text string) int
	EstimateMessages(messages []llm.ChatMessage) int
}

// SimpleEstimator uses the ~4 characters per token heuristic.
type SimpleEstimator struct{}

// Estimate returns an approximate token count for a string.
func (e *SimpleEstimator) Estimate(text string) int {
	return len(text) / 4
}

// EstimateMessages returns an approximate total token count for a message slice.
func (e *SimpleEstimator) EstimateMessages(messages []llm.ChatMessage) int {
	total := 0
	for _, msg := range messages {
		total += e.Estimate(ContentString(msg))
		total += 4 // overhead per message (role, separators)
	}
	return total
}

// ContentString extracts the text content from a ChatMessage.
func ContentString(msg llm.ChatMessage) string {
	switch c := msg.Content.(type) {
	case string:
		return c
	case nil:
		return ""
	case []any:
		// Handle []ContentPart-like structures serialized as []any
		var sb []byte
		for _, part := range c {
			if m, ok := part.(map[string]any); ok {
				if text, ok := m["text"].(string); ok {
					sb = append(sb, text...)
				}
			}
		}
		return string(sb)
	default:
		return fmt.Sprintf("%v", c)
	}
}
