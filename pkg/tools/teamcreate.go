package tools

import (
	"context"
	"fmt"
	"os"
)

// TeamCoordinator is the interface for team management operations.
// Implemented by teams.TeamManager.
type TeamCoordinator interface {
	CreateTeam(ctx context.Context, name string) (TeamInfo, error)
	SpawnTeammate(ctx context.Context, name, agentType, prompt string) (TeamMemberInfo, error)
	RequestShutdown(ctx context.Context, name string) error
	SendMessage(ctx context.Context, msg TeamMessage) error
	Broadcast(ctx context.Context, from, content string, recipients []string) error
	Cleanup(ctx context.Context) error
	GetTeamName() string
	GetMemberNames() []string
}

// TeamInfo is returned after team creation.
type TeamInfo struct {
	Name       string
	ConfigPath string
}

// TeamMemberInfo is returned after spawning a teammate.
type TeamMemberInfo struct {
	Name    string
	AgentID string
}

// TeamMessage represents a message to send.
type TeamMessage struct {
	From      string
	To        string
	Content   string
	Summary   string
	Type      string
	RequestID string
	Approve   bool
}

// StubTeamCoordinator returns not-configured errors.
type StubTeamCoordinator struct{}

func (s *StubTeamCoordinator) CreateTeam(_ context.Context, _ string) (TeamInfo, error) {
	return TeamInfo{}, fmt.Errorf("agent teams not configured")
}
func (s *StubTeamCoordinator) SpawnTeammate(_ context.Context, _, _, _ string) (TeamMemberInfo, error) {
	return TeamMemberInfo{}, fmt.Errorf("agent teams not configured")
}
func (s *StubTeamCoordinator) RequestShutdown(_ context.Context, _ string) error {
	return fmt.Errorf("agent teams not configured")
}
func (s *StubTeamCoordinator) SendMessage(_ context.Context, _ TeamMessage) error {
	return fmt.Errorf("agent teams not configured")
}
func (s *StubTeamCoordinator) Broadcast(_ context.Context, _, _ string, _ []string) error {
	return fmt.Errorf("agent teams not configured")
}
func (s *StubTeamCoordinator) Cleanup(_ context.Context) error {
	return fmt.Errorf("agent teams not configured")
}
func (s *StubTeamCoordinator) GetTeamName() string    { return "" }
func (s *StubTeamCoordinator) GetMemberNames() []string { return nil }

// TeamCreateTool creates a new agent team.
type TeamCreateTool struct {
	Coordinator TeamCoordinator
}

func (t *TeamCreateTool) Name() string { return "TeamCreate" }

func (t *TeamCreateTool) Description() string {
	return "Creates a new agent team for coordinating multiple Claude Code sessions."
}

func (t *TeamCreateTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"team_name": map[string]any{
				"type":        "string",
				"description": "Name for the team",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Description of the team's purpose",
			},
		},
		"required": []string{"team_name"},
	}
}

func (t *TeamCreateTool) SideEffect() SideEffectType { return SideEffectSpawns }

func (t *TeamCreateTool) Execute(ctx context.Context, input map[string]any) (ToolOutput, error) {
	if !isTeamsEnabled() {
		return ToolOutput{
			Content: "Error: Agent teams are not enabled. Set CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1 to enable.",
			IsError: true,
		}, nil
	}

	teamName, ok := input["team_name"].(string)
	if !ok || teamName == "" {
		return ToolOutput{Content: "Error: team_name is required", IsError: true}, nil
	}

	coordinator := t.Coordinator
	if coordinator == nil {
		coordinator = &StubTeamCoordinator{}
	}

	info, err := coordinator.CreateTeam(ctx, teamName)
	if err != nil {
		return ToolOutput{
			Content: fmt.Sprintf("Error: %s", err),
			IsError: true,
		}, nil
	}

	return ToolOutput{
		Content: fmt.Sprintf("Team '%s' created. Config: %s", info.Name, info.ConfigPath),
	}, nil
}

func isTeamsEnabled() bool {
	return os.Getenv("CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS") == "1"
}
