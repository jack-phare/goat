package agent

import (
	"github.com/jg-phare/goat/pkg/llm"
	"github.com/jg-phare/goat/pkg/types"
)

// ExitReason describes why the agentic loop terminated.
type ExitReason string

const (
	ExitEndTurn     ExitReason = "end_turn"
	ExitMaxTurns    ExitReason = "max_turns"
	ExitMaxBudget   ExitReason = "error_max_budget_usd"
	ExitInterrupted ExitReason = "interrupted"
	ExitMaxTokens   ExitReason = "max_tokens"
	ExitAborted     ExitReason = "aborted"
)

// LoopState tracks the mutable state of a running agentic loop.
type LoopState struct {
	SessionID     string
	Messages      []llm.ChatMessage // conversation history in OpenAI format
	TurnCount     int
	TotalUsage    types.BetaUsage
	TotalCostUSD  float64
	IsInterrupted bool
	ExitReason    ExitReason
}

// addUsage accumulates token usage from an LLM response.
func (s *LoopState) addUsage(usage types.BetaUsage) {
	s.TotalUsage.InputTokens += usage.InputTokens
	s.TotalUsage.OutputTokens += usage.OutputTokens
	s.TotalUsage.CacheReadInputTokens += usage.CacheReadInputTokens
	s.TotalUsage.CacheCreationInputTokens += usage.CacheCreationInputTokens
}
