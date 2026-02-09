package teams

import (
	"context"
	"fmt"

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

// CompleteTask fires the TaskCompleted hook and, if not prevented, marks the
// task as completed in the SharedTaskList. Returns the hook feedback message
// if completion was prevented, or empty string on success.
func (tm *TeamManager) CompleteTask(ctx context.Context, taskID, agentName string) (string, error) {
	team := tm.GetTeam()
	if team == nil {
		return "", fmt.Errorf("no active team")
	}

	// Look up the task subject for hook input
	tasks, err := team.Tasks.List()
	if err != nil {
		return "", fmt.Errorf("list tasks: %w", err)
	}
	var subject string
	for _, t := range tasks {
		if t.ID == taskID {
			subject = t.Subject
			break
		}
	}

	// Fire the hook before completing
	results, err := tm.fireTaskCompleted(ctx, taskID, subject, agentName)
	if err != nil {
		return "", fmt.Errorf("fire TaskCompleted hook: %w", err)
	}

	if prevent, msg := ShouldPreventCompletion(results); prevent {
		return msg, nil
	}

	if err := team.Tasks.Complete(taskID); err != nil {
		return "", err
	}
	return "", nil
}

// CheckIdle fires the TeammateIdle hook and returns whether the teammate
// should keep working. If keepWorking is true, the returned message explains why.
func (tm *TeamManager) CheckIdle(ctx context.Context, agentName string) (keepWorking bool, feedback string, err error) {
	results, err := tm.fireTeammateIdle(ctx, agentName)
	if err != nil {
		return false, "", fmt.Errorf("fire TeammateIdle hook: %w", err)
	}

	keepWorking, feedback = ShouldKeepWorking(results)
	return keepWorking, feedback, nil
}
