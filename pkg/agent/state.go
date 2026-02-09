package agent

import (
	"github.com/jg-phare/goat/pkg/llm"
	"github.com/jg-phare/goat/pkg/types"
)

// ExitReason describes why the agentic loop terminated.
type ExitReason string

const (
	ExitEndTurn       ExitReason = "end_turn"
	ExitStopSequence  ExitReason = "stop_sequence"
	ExitMaxTurns      ExitReason = "max_turns"
	ExitMaxBudget     ExitReason = "error_max_budget_usd"
	ExitInterrupted   ExitReason = "interrupted"
	ExitMaxTokens     ExitReason = "max_tokens"
	ExitAborted       ExitReason = "aborted"
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

	// Dynamic model override (set via control command, empty = use config.Model)
	Model             string
	MaxThinkingTokens int
	StopSequence      string // the stop sequence value if stop_sequence reason

	// PendingAdditionalContext collects context from hooks to inject
	// into the system prompt on the next LLM call.
	PendingAdditionalContext []string
}

// addUsage accumulates token usage from an LLM response.
func (s *LoopState) addUsage(usage types.BetaUsage) {
	s.TotalUsage.InputTokens += usage.InputTokens
	s.TotalUsage.OutputTokens += usage.OutputTokens
	s.TotalUsage.CacheReadInputTokens += usage.CacheReadInputTokens
	s.TotalUsage.CacheCreationInputTokens += usage.CacheCreationInputTokens
}
