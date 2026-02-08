package context

import (
	"context"
	"fmt"
	"strings"

	"github.com/jg-phare/goat/pkg/llm"
)

// compactionPrompt is the system prompt for the compaction summary LLM call.
const compactionPrompt = `Summarize the following conversation, preserving:
1. Key decisions and their rationale
2. File paths and code changes made
3. Unresolved questions or pending tasks
4. User preferences and constraints mentioned
5. Tool outputs that are still relevant

Be concise but complete. Use structured format with sections.`

// generateSummary calls the LLM to produce a summary of the given messages.
func generateSummary(ctx context.Context, messages []llm.ChatMessage, client llm.Client, model string, customInstructions *string) (string, error) {
	prompt := buildCompactionPrompt(messages, customInstructions)

	req := &llm.CompletionRequest{
		Model:     model,
		Stream:    true,
		MaxTokens: 4096,
		Messages: []llm.ChatMessage{
			{Role: "user", Content: prompt},
		},
	}

	stream, err := client.Complete(ctx, req)
	if err != nil {
		return "", fmt.Errorf("compaction summary LLM call: %w", err)
	}

	text, err := drainStreamText(stream)
	if err != nil {
		return "", fmt.Errorf("compaction summary drain: %w", err)
	}

	return text, nil
}

// buildCompactionPrompt constructs the user message for the summary LLM call.
func buildCompactionPrompt(messages []llm.ChatMessage, customInstructions *string) string {
	var sb strings.Builder
	sb.WriteString(compactionPrompt)

	if customInstructions != nil && *customInstructions != "" {
		sb.WriteString("\n\nAdditional instructions: ")
		sb.WriteString(*customInstructions)
	}

	sb.WriteString("\n\n--- CONVERSATION TO SUMMARIZE ---\n")
	for _, msg := range messages {
		content := ContentString(msg)
		if len(content) > 2000 {
			content = content[:2000] + "..."
		}
		sb.WriteString(fmt.Sprintf("[%s]: %s\n\n", msg.Role, content))
	}

	return sb.String()
}

// drainStreamText reads all chunks from a Stream and returns the concatenated text content.
func drainStreamText(stream *llm.Stream) (string, error) {
	resp, err := stream.Accumulate()
	if err != nil {
		return "", err
	}

	var text strings.Builder
	for _, block := range resp.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}

	return text.String(), nil
}
