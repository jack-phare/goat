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
	return `Remove team and task directories when the swarm work is complete.

This operation:
- Removes the team directory
- Removes the task directory
- Clears team context from the current session

IMPORTANT: TeamDelete will fail if the team still has active members. Gracefully terminate teammates first, then call TeamDelete after all teammates have shut down.

Use this when all teammates have finished their work and you want to clean up the team resources. The team name is automatically determined from the current session's team context.`
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
