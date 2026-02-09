package tools

import (
	"context"
	"fmt"
)

// SkillInfo holds the data the SkillTool needs to execute a skill.
type SkillInfo struct {
	Name         string
	Description  string
	Body         string
	AllowedTools []string
	Arguments    []string
	Context      string // "inline" or "fork"
}

// SkillProvider resolves skill names to their definitions.
type SkillProvider interface {
	GetSkillInfo(name string) (SkillInfo, bool)
}

// SkillTool invokes skills by name, returning their body as tool_result content.
type SkillTool struct {
	Skills  SkillProvider
	Spawner SubagentSpawner // for fork execution (Phase 8)

	// ArgSubstituter is an optional function for argument substitution.
	// Signature: func(body string, argDefs []string, argsStr string) (string, error)
	ArgSubstituter func(body string, argDefs []string, argsStr string) (string, error)
}

func (s *SkillTool) Name() string { return "Skill" }

func (s *SkillTool) Description() string {
	return `Execute a skill within the main conversation

When users ask you to perform tasks, check if any of the available skills match. Skills provide specialized capabilities and domain knowledge.

When users reference a "slash command" or "/<something>" (e.g., "/commit", "/review-pr"), they are referring to a skill. Use this tool to invoke it.

How to invoke:
- Use this tool with the skill name and optional arguments
- Examples:
  - skill: "pdf" - invoke the pdf skill
  - skill: "commit", args: "-m 'Fix bug'" - invoke with arguments
  - skill: "review-pr", args: "123" - invoke with arguments

Important:
- Available skills are listed in system-reminder messages in the conversation
- When a skill matches the user's request, invoke the relevant Skill tool BEFORE generating any other response about the task
- NEVER mention a skill without actually calling this tool
- Do not invoke a skill that is already running
- Do not use this tool for built-in CLI commands (like /help, /clear, etc.)`
}

func (s *SkillTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"skill": map[string]any{
				"type":        "string",
				"description": "The skill name. E.g., \"commit\", \"review-pr\", or \"pdf\"",
			},
			"args": map[string]any{
				"type":        "string",
				"description": "Optional arguments for the skill",
			},
		},
		"required": []string{"skill"},
	}
}

func (s *SkillTool) SideEffect() SideEffectType { return SideEffectNone }

func (s *SkillTool) Execute(ctx context.Context, input map[string]any) (ToolOutput, error) {
	skillName, ok := input["skill"].(string)
	if !ok || skillName == "" {
		return ToolOutput{Content: "Error: skill name is required", IsError: true}, nil
	}

	args, _ := input["args"].(string)

	if s.Skills == nil {
		return ToolOutput{
			Content: "Error: no skill provider configured",
			IsError: true,
		}, nil
	}

	skill, found := s.Skills.GetSkillInfo(skillName)
	if !found {
		return ToolOutput{
			Content: fmt.Sprintf("Error: skill %q not found. Check available skills in the system prompt.", skillName),
			IsError: true,
		}, nil
	}

	body := skill.Body

	// Argument substitution (wired in Phase 5)
	if s.ArgSubstituter != nil && len(skill.Arguments) > 0 && args != "" {
		substituted, err := s.ArgSubstituter(body, skill.Arguments, args)
		if err != nil {
			return ToolOutput{
				Content: fmt.Sprintf("Error substituting arguments: %v", err),
				IsError: true,
			}, nil
		}
		body = substituted
	}

	// Fork execution (wired in Phase 8)
	if skill.Context == "fork" && s.Spawner != nil {
		result, err := s.Spawner.Spawn(ctx, AgentInput{
			Description:  fmt.Sprintf("Skill: %s", skill.Name),
			Prompt:       body,
			SubagentType: "general-purpose",
		})
		if err != nil {
			return ToolOutput{
				Content: fmt.Sprintf("Error spawning skill agent: %v", err),
				IsError: true,
			}, nil
		}
		return ToolOutput{Content: result.Output}, nil
	}

	return ToolOutput{Content: body}, nil
}
