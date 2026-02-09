package tools

import (
	"context"
	"fmt"
)

// TaskStopTool stops a running background task.
type TaskStopTool struct {
	TaskManager *TaskManager
}

func (t *TaskStopTool) Name() string { return "TaskStop" }

func (t *TaskStopTool) Description() string {
	return `
- Stops a running background task by its ID
- Takes a task_id parameter identifying the task to stop
- Returns a success or failure status
- Use this tool when you need to terminate a long-running task`
}

func (t *TaskStopTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task_id": map[string]any{
				"type":        "string",
				"description": "The ID of the background task to stop",
			},
		},
		"required": []string{"task_id"},
	}
}

func (t *TaskStopTool) SideEffect() SideEffectType { return SideEffectMutating }

func (t *TaskStopTool) Execute(_ context.Context, input map[string]any) (ToolOutput, error) {
	if t.TaskManager == nil {
		return ToolOutput{Content: "Error: task manager not configured", IsError: true}, nil
	}

	taskID, ok := input["task_id"].(string)
	if !ok || taskID == "" {
		return ToolOutput{Content: "Error: task_id is required", IsError: true}, nil
	}

	if err := t.TaskManager.Stop(taskID); err != nil {
		return ToolOutput{
			Content: fmt.Sprintf("Error: %s", err),
			IsError: true,
		}, nil
	}

	return ToolOutput{Content: fmt.Sprintf("Task %s stopped successfully.", taskID)}, nil
}
