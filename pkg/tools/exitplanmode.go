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
	return "Signals that plan mode is complete and the agent is ready to implement."
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
