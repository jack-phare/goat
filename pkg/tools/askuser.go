package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// QuestionOption represents a choice for a question.
type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// QuestionSpec represents a single question to ask the user.
type QuestionSpec struct {
	Question    string           `json:"question"`
	Header      string           `json:"header"`
	Options     []QuestionOption `json:"options"`
	MultiSelect bool             `json:"multiSelect"`
}

// UserInputHandler provides user interaction for the AskUserQuestion tool.
type UserInputHandler interface {
	AskQuestions(ctx context.Context, questions []QuestionSpec) (map[string]string, error)
}

// AskUserQuestionTool blocks for user input, delegating to a callback interface.
type AskUserQuestionTool struct {
	Handler UserInputHandler
}

func (a *AskUserQuestionTool) Name() string { return "AskUserQuestion" }

func (a *AskUserQuestionTool) Description() string {
	return `Use this tool when you need to ask the user questions during execution. This allows you to:
1. Gather user preferences or requirements
2. Clarify ambiguous instructions
3. Get decisions on implementation choices as you work
4. Offer choices to the user about what direction to take.

Usage notes:
- Users will always be able to select "Other" to provide custom text input
- Use multiSelect: true to allow multiple answers to be selected for a question
- If you recommend a specific option, make that the first option in the list and add "(Recommended)" at the end of the label

Plan mode note: In plan mode, use this tool to clarify requirements or choose between approaches BEFORE finalizing your plan. Do NOT use this tool to ask "Is my plan ready?" or "Should I proceed?" - use ExitPlanMode for plan approval.`
}

func (a *AskUserQuestionTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"questions": map[string]any{
				"type":     "array",
				"minItems": 1,
				"maxItems": 4,
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"question": map[string]any{
							"type":        "string",
							"description": "The question to ask",
						},
						"header": map[string]any{
							"type":        "string",
							"description": "Short label (max 12 chars)",
							"maxLength":   12,
						},
						"options": map[string]any{
							"type":     "array",
							"minItems": 2,
							"maxItems": 4,
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"label":       map[string]any{"type": "string"},
									"description": map[string]any{"type": "string"},
								},
								"required": []string{"label", "description"},
							},
						},
						"multiSelect": map[string]any{
							"type":    "boolean",
							"default": false,
						},
					},
					"required": []string{"question", "header", "options", "multiSelect"},
				},
				"description": "Questions to ask the user (1-4 questions)",
			},
		},
		"required": []string{"questions"},
	}
}

func (a *AskUserQuestionTool) SideEffect() SideEffectType { return SideEffectBlocking }

func (a *AskUserQuestionTool) Execute(ctx context.Context, input map[string]any) (ToolOutput, error) {
	if a.Handler == nil {
		return ToolOutput{Content: "Error: user input not available in this context", IsError: true}, nil
	}

	rawQuestions, ok := input["questions"].([]any)
	if !ok || len(rawQuestions) == 0 {
		return ToolOutput{Content: "Error: questions is required and must be a non-empty array", IsError: true}, nil
	}
	if len(rawQuestions) > 4 {
		return ToolOutput{Content: "Error: maximum 4 questions allowed", IsError: true}, nil
	}

	questions := make([]QuestionSpec, 0, len(rawQuestions))
	for i, raw := range rawQuestions {
		// Re-marshal and unmarshal to parse the question spec
		data, err := json.Marshal(raw)
		if err != nil {
			return ToolOutput{
				Content: fmt.Sprintf("Error: invalid question at index %d", i),
				IsError: true,
			}, nil
		}
		var q QuestionSpec
		if err := json.Unmarshal(data, &q); err != nil {
			return ToolOutput{
				Content: fmt.Sprintf("Error: invalid question at index %d", i),
				IsError: true,
			}, nil
		}

		if q.Question == "" {
			return ToolOutput{
				Content: fmt.Sprintf("Error: questions[%d].question is required", i),
				IsError: true,
			}, nil
		}
		if q.Header == "" {
			return ToolOutput{
				Content: fmt.Sprintf("Error: questions[%d].header is required", i),
				IsError: true,
			}, nil
		}
		if len([]rune(q.Header)) > 12 {
			return ToolOutput{
				Content: fmt.Sprintf("Error: questions[%d].header must be 12 chars or less", i),
				IsError: true,
			}, nil
		}
		if len(q.Options) < 2 || len(q.Options) > 4 {
			return ToolOutput{
				Content: fmt.Sprintf("Error: questions[%d] must have 2-4 options", i),
				IsError: true,
			}, nil
		}

		questions = append(questions, q)
	}

	answers, err := a.Handler.AskQuestions(ctx, questions)
	if err != nil {
		return ToolOutput{
			Content: fmt.Sprintf("Error getting user input: %s", err),
			IsError: true,
		}, nil
	}

	var b strings.Builder
	b.WriteString("User answers:\n")
	for k, v := range answers {
		fmt.Fprintf(&b, "- %s: %s\n", k, v)
	}

	return ToolOutput{Content: strings.TrimRight(b.String(), "\n")}, nil
}
