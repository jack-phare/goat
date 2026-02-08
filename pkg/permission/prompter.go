package permission

import (
	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/types"
)

// UserPrompter handles interactive permission prompting.
type UserPrompter interface {
	// PromptForPermission shows the user a permission request and returns their decision.
	PromptForPermission(toolName string, input map[string]any, suggestions []types.PermissionUpdate) (agent.PermissionResult, error)
}

// StubPrompter always denies permission requests (for headless/testing).
type StubPrompter struct{}

func (s *StubPrompter) PromptForPermission(toolName string, _ map[string]any, _ []types.PermissionUpdate) (agent.PermissionResult, error) {
	return agent.PermissionResult{
		Behavior: "deny",
		Message:  "permission denied (no interactive prompter available)",
	}, nil
}
