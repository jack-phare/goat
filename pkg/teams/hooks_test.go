package teams

import (
	"context"
	"testing"

	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/hooks"
	"github.com/jg-phare/goat/pkg/types"
)

func TestFireTeammateIdle(t *testing.T) {
	var firedInput any
	runner := hooks.NewRunner(hooks.RunnerConfig{
		Hooks: map[types.HookEvent][]hooks.CallbackMatcher{
			types.HookEventTeammateIdle: {
				{
					Hooks: []hooks.HookCallback{
						func(input any, _ string, _ context.Context) (hooks.HookJSONOutput, error) {
							firedInput = input
							return hooks.HookJSONOutput{
								Sync: &hooks.SyncHookJSONOutput{},
							}, nil
						},
					},
				},
			},
		},
	})

	tm := NewTeamManager(runner, t.TempDir())
	tm.CreateTeam(context.Background(), "hook-test")

	results, err := tm.fireTeammateIdle(context.Background(), "worker-1")
	if err != nil {
		t.Fatalf("fireTeammateIdle: %v", err)
	}
	if firedInput == nil {
		t.Fatal("hook was not fired")
	}

	// Verify input structure
	input, ok := firedInput.(hooks.TeammateIdleHookInput)
	if !ok {
		t.Fatalf("unexpected input type: %T", firedInput)
	}
	if input.TeammateName != "worker-1" {
		t.Errorf("expected worker-1, got %s", input.TeammateName)
	}
	if input.TeamName != "hook-test" {
		t.Errorf("expected hook-test, got %s", input.TeamName)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestFireTeammateIdleNilHooks(t *testing.T) {
	tm := NewTeamManager(nil, t.TempDir())
	results, err := tm.fireTeammateIdle(context.Background(), "worker")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Error("expected nil results with nil hooks")
	}
}

func TestFireTaskCompleted(t *testing.T) {
	var firedInput any
	runner := hooks.NewRunner(hooks.RunnerConfig{
		Hooks: map[types.HookEvent][]hooks.CallbackMatcher{
			types.HookEventTaskCompleted: {
				{
					Hooks: []hooks.HookCallback{
						func(input any, _ string, _ context.Context) (hooks.HookJSONOutput, error) {
							firedInput = input
							return hooks.HookJSONOutput{
								Sync: &hooks.SyncHookJSONOutput{},
							}, nil
						},
					},
				},
			},
		},
	})

	tm := NewTeamManager(runner, t.TempDir())
	tm.CreateTeam(context.Background(), "task-hook")

	results, err := tm.fireTaskCompleted(context.Background(), "task-1", "Build feature", "alice")
	if err != nil {
		t.Fatalf("fireTaskCompleted: %v", err)
	}
	if firedInput == nil {
		t.Fatal("hook was not fired")
	}

	input, ok := firedInput.(hooks.TaskCompletedHookInput)
	if !ok {
		t.Fatalf("unexpected input type: %T", firedInput)
	}
	if input.TaskID != "task-1" {
		t.Errorf("expected task-1, got %s", input.TaskID)
	}
	if input.TaskSubject != "Build feature" {
		t.Errorf("expected Build feature, got %s", input.TaskSubject)
	}
	if input.TeammateName != "alice" {
		t.Errorf("expected alice, got %s", input.TeammateName)
	}
	if input.TeamName != "task-hook" {
		t.Errorf("expected task-hook, got %s", input.TeamName)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestFireTaskCompletedNilHooks(t *testing.T) {
	tm := NewTeamManager(nil, t.TempDir())
	results, err := tm.fireTaskCompleted(context.Background(), "t-1", "Test", "worker")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Error("expected nil results with nil hooks")
	}
}

func TestShouldKeepWorking(t *testing.T) {
	f := false

	// No results
	keep, _ := ShouldKeepWorking(nil)
	if keep {
		t.Error("expected false for nil results")
	}

	// Result with continue=false and message → keep working
	results := []agent.HookResult{
		{Continue: &f, Message: "More work to do"},
	}
	keep, msg := ShouldKeepWorking(results)
	if !keep {
		t.Error("expected true when hook says continue=false with message")
	}
	if msg != "More work to do" {
		t.Errorf("unexpected message: %s", msg)
	}

	// Result with continue=false but no message → don't keep
	results = []agent.HookResult{
		{Continue: &f},
	}
	keep, _ = ShouldKeepWorking(results)
	if keep {
		t.Error("expected false when no message")
	}
}

func TestShouldPreventCompletion(t *testing.T) {
	f := false

	// Result with continue=false and message → prevent completion
	results := []agent.HookResult{
		{Continue: &f, Message: "Quality check failed"},
	}
	prevent, msg := ShouldPreventCompletion(results)
	if !prevent {
		t.Error("expected true for preventing completion")
	}
	if msg != "Quality check failed" {
		t.Errorf("unexpected message: %s", msg)
	}

	// Normal result → allow completion
	tr := true
	results = []agent.HookResult{
		{Continue: &tr},
	}
	prevent, _ = ShouldPreventCompletion(results)
	if prevent {
		t.Error("expected false for normal result")
	}
}
