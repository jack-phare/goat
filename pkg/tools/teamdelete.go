package tools

import (
	"context"
	"fmt"
)

// TeamDeleteTool cleans up the active team.
type TeamDeleteTool struct {
	Coordinator TeamCoordinator
}

func (t *TeamDeleteTool) Name() string { return "TeamDelete" }

func (t *TeamDeleteTool) Description() string {
	return "Deletes the active agent team and cleans up resources."
}

func (t *TeamDeleteTool) InputSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *TeamDeleteTool) SideEffect() SideEffectType { return SideEffectMutating }

func (t *TeamDeleteTool) Execute(ctx context.Context, _ map[string]any) (ToolOutput, error) {
	if !isTeamsEnabled() {
		return ToolOutput{
			Content: "Error: Agent teams are not enabled. Set CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1 to enable.",
			IsError: true,
		}, nil
	}

	coordinator := t.Coordinator
	if coordinator == nil {
		coordinator = &StubTeamCoordinator{}
	}

	teamName := coordinator.GetTeamName()
	if err := coordinator.Cleanup(ctx); err != nil {
		return ToolOutput{
			Content: fmt.Sprintf("Error: %s", err),
			IsError: true,
		}, nil
	}

	if teamName == "" {
		teamName = "unknown"
	}
	return ToolOutput{Content: fmt.Sprintf("Team '%s' deleted.", teamName)}, nil
}
