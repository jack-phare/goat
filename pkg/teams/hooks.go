package teams

import (
	"context"

	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/hooks"
	"github.com/jg-phare/goat/pkg/types"
)

// fireTeammateIdle fires the TeammateIdle hook event.
// If any hook returns exit code 2, it means the teammate should keep working.
func (tm *TeamManager) fireTeammateIdle(ctx context.Context, teammateName string) ([]agent.HookResult, error) {
	if tm.hooks == nil {
		return nil, nil
	}

	team := tm.GetTeam()
	teamName := ""
	if team != nil {
		teamName = team.Name
	}

	input := hooks.TeammateIdleHookInput{
		BaseHookInput: hooks.BaseHookInput{},
		HookEventName: string(types.HookEventTeammateIdle),
		TeammateName:  teammateName,
		TeamName:      teamName,
	}

	return tm.hooks.Fire(ctx, types.HookEventTeammateIdle, input)
}

// fireTaskCompleted fires the TaskCompleted hook event.
// If any hook returns exit code 2, it prevents the task from being completed.
func (tm *TeamManager) fireTaskCompleted(ctx context.Context, taskID, taskSubject, teammateName string) ([]agent.HookResult, error) {
	if tm.hooks == nil {
		return nil, nil
	}

	team := tm.GetTeam()
	teamName := ""
	if team != nil {
		teamName = team.Name
	}

	input := hooks.TaskCompletedHookInput{
		BaseHookInput: hooks.BaseHookInput{},
		HookEventName: string(types.HookEventTaskCompleted),
		TaskID:        taskID,
		TaskSubject:   taskSubject,
		TeammateName:  teammateName,
		TeamName:      teamName,
	}

	return tm.hooks.Fire(ctx, types.HookEventTaskCompleted, input)
}

// ShouldKeepWorking returns true if any hook result indicates the teammate should continue
// (simulating exit code 2 behavior — the hook sets continue=false with a reason message).
func ShouldKeepWorking(results []agent.HookResult) (bool, string) {
	for _, r := range results {
		if r.Continue != nil && !*r.Continue && r.Message != "" {
			return true, r.Message
		}
	}
	return false, ""
}

// ShouldPreventCompletion returns true if any hook result indicates the task should not
// be marked as complete (simulating exit code 2 behavior).
func ShouldPreventCompletion(results []agent.HookResult) (bool, string) {
	for _, r := range results {
		if r.Continue != nil && !*r.Continue && r.Message != "" {
			return true, r.Message
		}
	}
	return false, ""
}
