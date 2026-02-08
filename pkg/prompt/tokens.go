package prompt

import (
	"fmt"

	"github.com/jg-phare/goat/pkg/agent"
)

// EstimateTokens returns an approximate token count for a string.
// Uses the ~4 characters per token heuristic for Claude models.
func EstimateTokens(text string) int {
	return len(text) / 4
}

// Validate checks that the assembled prompt fits within the given token budget.
func (a *Assembler) Validate(config *agent.AgentConfig, maxTokens int) error {
	prompt := a.Assemble(config)
	tokens := EstimateTokens(prompt)
	if tokens > maxTokens {
		return fmt.Errorf("system prompt %d tokens exceeds budget %d", tokens, maxTokens)
	}
	return nil
}
