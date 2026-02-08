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
	return "Creates or replaces a structured todo list for tracking task progress."
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
