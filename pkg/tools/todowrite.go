package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

var validTodoStatuses = map[string]bool{
	"pending":     true,
	"in_progress": true,
	"completed":   true,
}

// TodoItem represents a single todo entry.
type TodoItem struct {
	Content    string
	Status     string
	ActiveForm string
}

// TodoWriteTool manages a structured todo list in memory.
type TodoWriteTool struct {
	mu    sync.Mutex
	Todos []TodoItem
}

func (t *TodoWriteTool) Name() string { return "TodoWrite" }

func (t *TodoWriteTool) Description() string {
	return `Use this tool to create a structured task list for your current coding session. This helps you track progress, organize complex tasks, and demonstrate thoroughness to the user.
It also helps the user understand the progress of the task and overall progress of their requests.

## When to Use This Tool

Use this tool proactively in these scenarios:

- Complex multi-step tasks - When a task requires 3 or more distinct steps or actions
- Non-trivial and complex tasks - Tasks that require careful planning or multiple operations
- Plan mode - When using plan mode, create a task list to track the work
- User explicitly requests todo list - When the user directly asks you to use the todo list
- User provides multiple tasks - When users provide a list of things to be done (numbered or comma-separated)
- After receiving new instructions - Immediately capture user requirements as tasks
- When you start working on a task - Mark it as in_progress BEFORE beginning work
- After completing a task - Mark it as completed and add any new follow-up tasks discovered during implementation

## When NOT to Use This Tool

Skip using this tool when:
- There is only a single, straightforward task
- The task is trivial and tracking it provides no organizational benefit
- The task can be completed in less than 3 trivial steps
- The task is purely conversational or informational

NOTE that you should not use this tool if there is only one trivial task to do. In this case you are better off just doing the task directly.

## Task Fields

- **subject**: A brief, actionable title in imperative form (e.g., "Fix authentication bug in login flow")
- **description**: Detailed description of what needs to be done, including context and acceptance criteria
- **activeForm**: Present continuous form shown in spinner when task is in_progress (e.g., "Fixing authentication bug"). This is displayed to the user while you work on the task.

**IMPORTANT**: Always provide activeForm when creating tasks. The subject should be imperative ("Run tests") while activeForm should be present continuous ("Running tests"). All tasks are created with status ` + "`pending`" + `.

## Tips

- Create tasks with clear, specific subjects that describe the outcome
- Include enough detail in the description for another agent to understand and complete the task
- After creating tasks, use TaskUpdate to set up dependencies (blocks/blockedBy) if needed
- Check TaskList first to avoid creating duplicate tasks`
}

func (t *TodoWriteTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"todos": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"content": map[string]any{
							"type":        "string",
							"description": "The todo item content",
						},
						"status": map[string]any{
							"type":        "string",
							"enum":        []string{"pending", "in_progress", "completed"},
							"description": "Status of the todo item",
						},
						"activeForm": map[string]any{
							"type":        "string",
							"description": "Present continuous form shown when in_progress",
						},
					},
					"required": []string{"content", "status"},
				},
				"description": "The full todo list (replaces existing)",
			},
		},
		"required": []string{"todos"},
	}
}

func (t *TodoWriteTool) SideEffect() SideEffectType { return SideEffectNone }

func (t *TodoWriteTool) Execute(_ context.Context, input map[string]any) (ToolOutput, error) {
	rawTodos, ok := input["todos"].([]any)
	if !ok {
		return ToolOutput{Content: "Error: todos is required and must be an array", IsError: true}, nil
	}

	items := make([]TodoItem, 0, len(rawTodos))
	for i, raw := range rawTodos {
		obj, ok := raw.(map[string]any)
		if !ok {
			return ToolOutput{
				Content: fmt.Sprintf("Error: todos[%d] must be an object", i),
				IsError: true,
			}, nil
		}

		content, _ := obj["content"].(string)
		if content == "" {
			return ToolOutput{
				Content: fmt.Sprintf("Error: todos[%d].content is required", i),
				IsError: true,
			}, nil
		}

		status, _ := obj["status"].(string)
		if !validTodoStatuses[status] {
			return ToolOutput{
				Content: fmt.Sprintf("Error: todos[%d].status must be one of: pending, in_progress, completed", i),
				IsError: true,
			}, nil
		}

		activeForm, _ := obj["activeForm"].(string)
		items = append(items, TodoItem{
			Content:    content,
			Status:     status,
			ActiveForm: activeForm,
		})
	}

	t.mu.Lock()
	t.Todos = items
	t.mu.Unlock()

	return ToolOutput{Content: t.formatList(items)}, nil
}

func (t *TodoWriteTool) formatList(items []TodoItem) string {
	if len(items) == 0 {
		return "Todo list cleared."
	}

	var b strings.Builder
	b.WriteString("Todo list updated:\n")
	for i, item := range items {
		marker := "[ ]"
		switch item.Status {
		case "in_progress":
			marker = "[~]"
		case "completed":
			marker = "[x]"
		}
		fmt.Fprintf(&b, "%d. %s %s (%s)", i+1, marker, item.Content, item.Status)
		if i < len(items)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
