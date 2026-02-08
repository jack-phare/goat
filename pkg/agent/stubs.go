package agent

import (
	"context"

	"github.com/jg-phare/goat/pkg/llm"
	"github.com/jg-phare/goat/pkg/types"
)

// StaticPromptAssembler returns a fixed system prompt string.
type StaticPromptAssembler struct {
	Prompt string
}

func (s *StaticPromptAssembler) Assemble(_ *AgentConfig) string {
	return s.Prompt
}

// AllowAllChecker always permits tool execution.
type AllowAllChecker struct{}

func (a *AllowAllChecker) Check(_ context.Context, _ string, _ map[string]any) (PermissionResult, error) {
	return PermissionResult{Behavior: "allow"}, nil
}

// NoOpHookRunner does nothing and returns empty results.
type NoOpHookRunner struct{}

func (n *NoOpHookRunner) Fire(_ context.Context, _ types.HookEvent, _ any) ([]HookResult, error) {
	return nil, nil
}

// NoOpCompactor never compacts.
type NoOpCompactor struct{}

func (n *NoOpCompactor) ShouldCompact(_ []llm.ChatMessage, _ string) bool {
	return false
}

func (n *NoOpCompactor) Compact(_ context.Context, msgs []llm.ChatMessage) ([]llm.ChatMessage, error) {
	return msgs, nil
}
