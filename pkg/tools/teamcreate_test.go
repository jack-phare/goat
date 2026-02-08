package tools

import (
	"context"
	"fmt"
	"testing"
)

type mockTeamCoordinator struct {
	createTeamFn     func(ctx context.Context, name string) (TeamInfo, error)
	spawnFn          func(ctx context.Context, name, agentType, prompt string) (TeamMemberInfo, error)
	requestShutFn    func(ctx context.Context, name string) error
	sendMessageFn    func(ctx context.Context, msg TeamMessage) error
	broadcastFn      func(ctx context.Context, from, content string, recipients []string) error
	cleanupFn        func(ctx context.Context) error
	teamName         string
	memberNames      []string
}

func (m *mockTeamCoordinator) CreateTeam(ctx context.Context, name string) (TeamInfo, error) {
	if m.createTeamFn != nil {
		return m.createTeamFn(ctx, name)
	}
	return TeamInfo{Name: name, ConfigPath: "/tmp/config.json"}, nil
}

func (m *mockTeamCoordinator) SpawnTeammate(ctx context.Context, name, agentType, prompt string) (TeamMemberInfo, error) {
	if m.spawnFn != nil {
		return m.spawnFn(ctx, name, agentType, prompt)
	}
	return TeamMemberInfo{Name: name, AgentID: "agent-123"}, nil
}

func (m *mockTeamCoordinator) RequestShutdown(ctx context.Context, name string) error {
	if m.requestShutFn != nil {
		return m.requestShutFn(ctx, name)
	}
	return nil
}

func (m *mockTeamCoordinator) SendMessage(ctx context.Context, msg TeamMessage) error {
	if m.sendMessageFn != nil {
		return m.sendMessageFn(ctx, msg)
	}
	return nil
}

func (m *mockTeamCoordinator) Broadcast(ctx context.Context, from, content string, recipients []string) error {
	if m.broadcastFn != nil {
		return m.broadcastFn(ctx, from, content, recipients)
	}
	return nil
}

func (m *mockTeamCoordinator) Cleanup(ctx context.Context) error {
	if m.cleanupFn != nil {
		return m.cleanupFn(ctx)
	}
	return nil
}

func (m *mockTeamCoordinator) GetTeamName() string      { return m.teamName }
func (m *mockTeamCoordinator) GetMemberNames() []string  { return m.memberNames }

func TestTeamCreateToolSuccess(t *testing.T) {
	t.Setenv("CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS", "1")

	tool := &TeamCreateTool{
		Coordinator: &mockTeamCoordinator{},
	}

	output, err := tool.Execute(context.Background(), map[string]any{
		"team_name": "my-team",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if output.IsError {
		t.Fatalf("unexpected error: %s", output.Content)
	}
	if output.Content == "" {
		t.Error("expected non-empty output")
	}
}

func TestTeamCreateToolFeatureGate(t *testing.T) {
	// Don't set the env var
	tool := &TeamCreateTool{}

	output, err := tool.Execute(context.Background(), map[string]any{
		"team_name": "my-team",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !output.IsError {
		t.Fatal("expected error for disabled feature")
	}
}

func TestTeamCreateToolMissingName(t *testing.T) {
	t.Setenv("CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS", "1")

	tool := &TeamCreateTool{Coordinator: &mockTeamCoordinator{}}
	output, _ := tool.Execute(context.Background(), map[string]any{})
	if !output.IsError {
		t.Fatal("expected error for missing name")
	}
}

func TestTeamCreateToolCoordinatorError(t *testing.T) {
	t.Setenv("CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS", "1")

	tool := &TeamCreateTool{
		Coordinator: &mockTeamCoordinator{
			createTeamFn: func(_ context.Context, _ string) (TeamInfo, error) {
				return TeamInfo{}, fmt.Errorf("team already active")
			},
		},
	}

	output, _ := tool.Execute(context.Background(), map[string]any{
		"team_name": "test",
	})
	if !output.IsError {
		t.Fatal("expected error from coordinator")
	}
}

func TestTeamCreateToolNilCoordinator(t *testing.T) {
	t.Setenv("CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS", "1")

	tool := &TeamCreateTool{}
	output, _ := tool.Execute(context.Background(), map[string]any{
		"team_name": "test",
	})
	if !output.IsError {
		t.Fatal("expected error from stub coordinator")
	}
}

func TestTeamCreateToolName(t *testing.T) {
	tool := &TeamCreateTool{}
	if tool.Name() != "TeamCreate" {
		t.Errorf("expected TeamCreate, got %s", tool.Name())
	}
}

func TestTeamCreateToolSideEffect(t *testing.T) {
	tool := &TeamCreateTool{}
	if tool.SideEffect() != SideEffectSpawns {
		t.Error("expected SideEffectSpawns")
	}
}
