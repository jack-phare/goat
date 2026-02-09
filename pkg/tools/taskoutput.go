package tools

import (
	"context"
	"fmt"
	"time"
)

// TaskOutputTool retrieves output from a background task.
type TaskOutputTool struct {
	TaskManager *TaskManager
}

func (t *TaskOutputTool) Name() string { return "TaskOutput" }

func (t *TaskOutputTool) Description() string {
	return `- Retrieves output from a running or completed task (background shell, agent, or remote session)
- Takes a task_id parameter identifying the task
- Returns the task output along with status information
- Use block=true (default) to wait for task completion
- Use block=false for non-blocking check of current status
- Task IDs can be found using the /tasks command
- Works with all task types: background shells, async agents, and remote sessions`
}

func (t *TaskOutputTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task_id": map[string]any{
				"type":        "string",
				"description": "The task ID to get output from",
			},
			"block": map[string]any{
				"type":        "boolean",
				"description": "Whether to wait for completion (default true)",
			},
			"timeout": map[string]any{
				"type":        "number",
				"description": "Max wait time in ms (default 30000)",
			},
		},
		"required": []string{"task_id"},
	}
}

func (t *TaskOutputTool) SideEffect() SideEffectType { return SideEffectNone }

func (t *TaskOutputTool) Execute(_ context.Context, input map[string]any) (ToolOutput, error) {
	if t.TaskManager == nil {
		return ToolOutput{Content: "Error: task manager not configured", IsError: true}, nil
	}

	taskID, ok := input["task_id"].(string)
	if !ok || taskID == "" {
		return ToolOutput{Content: "Error: task_id is required", IsError: true}, nil
	}

	block := true
	if b, ok := input["block"].(bool); ok {
		block = b
	}

	timeout := 30 * time.Second
	if t, ok := input["timeout"].(float64); ok && t > 0 {
		timeout = time.Duration(t) * time.Millisecond
	}

	output, err := t.TaskManager.GetOutput(taskID, block, timeout)
	if err != nil {
		return ToolOutput{
			Content: fmt.Sprintf("Error: %s\nPartial output:\n%s", err, output),
			IsError: true,
		}, nil
	}

	task, _ := t.TaskManager.Get(taskID)
	status := task.getStatus()

	return ToolOutput{
		Content: fmt.Sprintf("Task %s (status: %s):\n%s", taskID, status, output),
	}, nil
}
