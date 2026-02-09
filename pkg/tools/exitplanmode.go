package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// AllowedPrompt represents a permitted action in the implementation phase.
type AllowedPrompt struct {
	Tool   string `json:"tool"`
	Prompt string `json:"prompt"`
}

// ExitPlanModeTool signals that plan mode should be exited.
type ExitPlanModeTool struct{}

func (e *ExitPlanModeTool) Name() string { return "ExitPlanMode" }

func (e *ExitPlanModeTool) Description() string {
	return `Use this tool when you are in plan mode and have finished writing your plan to the plan file and are ready for user approval.

## How This Tool Works
- You should have already written your plan to the plan file specified in the plan mode system message
- This tool does NOT take the plan content as a parameter - it will read the plan from the file you wrote
- This tool simply signals that you're done planning and ready for the user to review and approve
- The user will see the contents of your plan file when they review it

## When to Use This Tool
IMPORTANT: Only use this tool when the task requires planning the implementation steps of a task that requires writing code. For research tasks where you're gathering information, searching files, reading files or in general trying to understand the codebase - do NOT use this tool.

## Before Using This Tool
Ensure your plan is complete and unambiguous:
- If you have unresolved questions about requirements or approach, use AskUserQuestion first (in earlier phases)
- Once your plan is finalized, use THIS tool to request approval

**Important:** Do NOT use AskUserQuestion to ask "Is this plan okay?" or "Should I proceed?" - that's exactly what THIS tool does. ExitPlanMode inherently requests user approval of your plan.

## Examples

1. Initial task: "Search for and understand the implementation of vim mode in the codebase" - Do not use the exit plan mode tool because you are not planning the implementation steps of a task.
2. Initial task: "Help me implement yank mode for vim" - Use the exit plan mode tool after you have finished planning the implementation steps of the task.
3. Initial task: "Add a new feature to handle user authentication" - If unsure about auth method (OAuth, JWT, etc.), use AskUserQuestion first, then use exit plan mode tool after clarifying the approach.`
}

func (e *ExitPlanModeTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"allowedPrompts": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"tool": map[string]any{
							"type": "string",
							"enum": []string{"Bash"},
						},
						"prompt": map[string]any{
							"type":        "string",
							"description": "Semantic description of the action",
						},
					},
					"required": []string{"tool", "prompt"},
				},
				"description": "Prompt-based permissions needed to implement the plan",
			},
		},
	}
}

func (e *ExitPlanModeTool) SideEffect() SideEffectType { return SideEffectNone }

func (e *ExitPlanModeTool) Execute(_ context.Context, input map[string]any) (ToolOutput, error) {
	var prompts []AllowedPrompt

	if raw, ok := input["allowedPrompts"].([]any); ok {
		for i, item := range raw {
			data, err := json.Marshal(item)
			if err != nil {
				return ToolOutput{
					Content: fmt.Sprintf("Error: invalid allowedPrompts[%d]", i),
					IsError: true,
				}, nil
			}
			var p AllowedPrompt
			if err := json.Unmarshal(data, &p); err != nil {
				return ToolOutput{
					Content: fmt.Sprintf("Error: invalid allowedPrompts[%d]", i),
					IsError: true,
				}, nil
			}
			if p.Tool != "Bash" {
				return ToolOutput{
					Content: fmt.Sprintf("Error: allowedPrompts[%d].tool must be \"Bash\"", i),
					IsError: true,
				}, nil
			}
			if p.Prompt == "" {
				return ToolOutput{
					Content: fmt.Sprintf("Error: allowedPrompts[%d].prompt is required", i),
					IsError: true,
				}, nil
			}
			prompts = append(prompts, p)
		}
	}

	if len(prompts) == 0 {
		return ToolOutput{Content: "Exiting plan mode."}, nil
	}

	var b strings.Builder
	b.WriteString("Exiting plan mode. Allowed prompts:\n")
	for _, p := range prompts {
		fmt.Fprintf(&b, "- [%s] %s\n", p.Tool, p.Prompt)
	}

	return ToolOutput{Content: strings.TrimRight(b.String(), "\n")}, nil
}
