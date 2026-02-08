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

func (n *NoOpCompactor) ShouldCompact(_ TokenBudget) bool {
	return false
}

func (n *NoOpCompactor) Compact(_ context.Context, req CompactRequest) ([]llm.ChatMessage, error) {
	return req.Messages, nil
}

// NoOpSessionStore does nothing and returns empty values.
type NoOpSessionStore struct{}

func (n *NoOpSessionStore) Create(_ SessionMetadata) error                          { return nil }
func (n *NoOpSessionStore) Load(_ string) (*SessionState, error)                    { return &SessionState{}, nil }
func (n *NoOpSessionStore) LoadLatest(_ string) (*SessionState, error)              { return nil, nil }
func (n *NoOpSessionStore) Delete(_ string) error                                   { return nil }
func (n *NoOpSessionStore) List() ([]SessionMetadata, error)                        { return nil, nil }
func (n *NoOpSessionStore) Fork(_, _ string) (*SessionState, error)                 { return &SessionState{}, nil }
func (n *NoOpSessionStore) AppendMessage(_ string, _ MessageEntry) error            { return nil }
func (n *NoOpSessionStore) AppendSDKMessage(_ string, _ types.SDKMessage) error     { return nil }
func (n *NoOpSessionStore) LoadMessages(_ string) ([]MessageEntry, error)           { return nil, nil }
func (n *NoOpSessionStore) LoadMessagesUpTo(_, _ string) ([]MessageEntry, error)    { return nil, nil }
func (n *NoOpSessionStore) UpdateMetadata(_ string, _ func(*SessionMetadata)) error { return nil }
func (n *NoOpSessionStore) CreateCheckpoint(_, _ string, _ []string) error          { return nil }
func (n *NoOpSessionStore) RewindFiles(_, _ string, _ bool) (*RewindFilesResult, error) {
	return &RewindFilesResult{}, nil
}
func (n *NoOpSessionStore) Close() error { return nil }
