package tools

import (
	"context"
	"fmt"
	"testing"
)

func TestTeamDeleteToolSuccess(t *testing.T) {
	t.Setenv("CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS", "1")

	tool := &TeamDeleteTool{
		Coordinator: &mockTeamCoordinator{
			teamName: "my-team",
		},
	}

	output, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if output.IsError {
		t.Fatalf("unexpected error: %s", output.Content)
	}
	if output.Content != "Team 'my-team' deleted." {
		t.Errorf("unexpected output: %s", output.Content)
	}
}

func TestTeamDeleteToolFeatureGate(t *testing.T) {
	tool := &TeamDeleteTool{}
	output, _ := tool.Execute(context.Background(), nil)
	if !output.IsError {
		t.Fatal("expected error for disabled feature")
	}
}

func TestTeamDeleteToolCoordinatorError(t *testing.T) {
	t.Setenv("CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS", "1")

	tool := &TeamDeleteTool{
		Coordinator: &mockTeamCoordinator{
			cleanupFn: func(_ context.Context) error {
				return fmt.Errorf("team has active members")
			},
		},
	}

	output, _ := tool.Execute(context.Background(), nil)
	if !output.IsError {
		t.Fatal("expected error from coordinator")
	}
}

func TestTeamDeleteToolNilCoordinator(t *testing.T) {
	t.Setenv("CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS", "1")

	tool := &TeamDeleteTool{}
	output, _ := tool.Execute(context.Background(), nil)
	if !output.IsError {
		t.Fatal("expected error from stub coordinator")
	}
}

func TestTeamDeleteToolName(t *testing.T) {
	tool := &TeamDeleteTool{}
	if tool.Name() != "TeamDelete" {
		t.Errorf("expected TeamDelete, got %s", tool.Name())
	}
}

func TestTeamDeleteToolSideEffect(t *testing.T) {
	tool := &TeamDeleteTool{}
	if tool.SideEffect() != SideEffectMutating {
		t.Error("expected SideEffectMutating")
	}
}
