package teams

import (
	"context"
	"fmt"

	"github.com/jg-phare/goat/pkg/tools"
)

// TeamManagerAdapter wraps a *TeamManager to implement tools.TeamCoordinator.
type TeamManagerAdapter struct {
	TM *TeamManager

	// SpawnFunc is called to spawn a teammate process. If nil, SpawnTeammate
	// returns an error indicating spawn is not configured.
	SpawnFunc func(ctx context.Context, name, agentType, prompt string) (tools.TeamMemberInfo, error)
}

// CreateTeam creates a new team via the wrapped TeamManager.
func (a *TeamManagerAdapter) CreateTeam(ctx context.Context, name string) (tools.TeamInfo, error) {
	team, err := a.TM.CreateTeam(ctx, name)
	if err != nil {
		return tools.TeamInfo{}, err
	}
	return tools.TeamInfo{
		Name:       team.Name,
		ConfigPath: team.configPath,
	}, nil
}

// SpawnTeammate delegates to the configured SpawnFunc.
func (a *TeamManagerAdapter) SpawnTeammate(ctx context.Context, name, agentType, prompt string) (tools.TeamMemberInfo, error) {
	if a.SpawnFunc != nil {
		return a.SpawnFunc(ctx, name, agentType, prompt)
	}
	return tools.TeamMemberInfo{}, fmt.Errorf("teammate spawning not configured")
}

// RequestShutdown sends a shutdown request message to the named teammate.
func (a *TeamManagerAdapter) RequestShutdown(_ context.Context, name string) error {
	team := a.TM.GetTeam()
	if team == nil {
		return fmt.Errorf("no active team")
	}
	return team.Mailbox.Send(Message{
		From:    "lead",
		To:      name,
		Content: "Shutdown requested.",
		Type:    "shutdown_request",
	})
}

// SendMessage sends a message to a teammate via the team mailbox.
func (a *TeamManagerAdapter) SendMessage(_ context.Context, msg tools.TeamMessage) error {
	team := a.TM.GetTeam()
	if team == nil {
		return fmt.Errorf("no active team")
	}
	return team.Mailbox.Send(Message{
		From:    msg.From,
		To:      msg.To,
		Content: msg.Content,
		Summary: msg.Summary,
		Type:    msg.Type,
	})
}

// Broadcast sends a message to all specified recipients.
func (a *TeamManagerAdapter) Broadcast(_ context.Context, from, content string, recipients []string) error {
	team := a.TM.GetTeam()
	if team == nil {
		return fmt.Errorf("no active team")
	}
	return team.Mailbox.Broadcast(from, content, recipients)
}

// Cleanup delegates to the wrapped TeamManager.
func (a *TeamManagerAdapter) Cleanup(ctx context.Context) error {
	return a.TM.Cleanup(ctx)
}

// GetTeamName returns the active team's name.
func (a *TeamManagerAdapter) GetTeamName() string {
	team := a.TM.GetTeam()
	if team == nil {
		return ""
	}
	return team.Name
}

// GetMemberNames returns the names of all team members.
func (a *TeamManagerAdapter) GetMemberNames() []string {
	team := a.TM.GetTeam()
	if team == nil {
		return nil
	}
	return team.MemberNames()
}

// Verify TeamManagerAdapter implements tools.TeamCoordinator at compile time.
var _ tools.TeamCoordinator = (*TeamManagerAdapter)(nil)
