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
	UsingFallback     bool   // true if currently using FallbackModel after a retriable error
	BudgetDowngraded  bool   // true if model was downgraded due to budget threshold

	// LastError captures the last error that caused the loop to exit.
	LastError error

	// PendingAdditionalContext collects context from hooks to inject
	// into the system prompt on the next LLM call.
	PendingAdditionalContext []string

	// AccessedFiles tracks file paths touched during this session.
	// Key: absolute file path, Value: set of operations (read, write, edit, glob, grep, exec)
	AccessedFiles map[string]map[string]bool

	// ActiveSkill holds the scope of the currently executing skill.
	// When set, tool permission checks are augmented by the skill's allowed-tools.
	// Cleared on end_turn or next user message.
	ActiveSkill *SkillScope
}

// SkillScope holds the runtime context for an active skill execution.
type SkillScope struct {
	SkillName    string
	AllowedTools []string
}

// RecordFileAccess records that a file was accessed with the given operation.
func (s *LoopState) RecordFileAccess(path string, op string) {
	if s.AccessedFiles == nil {
		s.AccessedFiles = make(map[string]map[string]bool)
	}
	if s.AccessedFiles[path] == nil {
		s.AccessedFiles[path] = make(map[string]bool)
	}
	s.AccessedFiles[path][op] = true
}

// addUsage accumulates token usage from an LLM response.
func (s *LoopState) addUsage(usage types.BetaUsage) {
	s.TotalUsage.InputTokens += usage.InputTokens
	s.TotalUsage.OutputTokens += usage.OutputTokens
	s.TotalUsage.CacheReadInputTokens += usage.CacheReadInputTokens
	s.TotalUsage.CacheCreationInputTokens += usage.CacheCreationInputTokens
}
