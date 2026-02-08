package tools

import (
	"context"
	"fmt"
)

// AgentInput contains the parameters for spawning a subagent.
type AgentInput struct {
	Description     string
	Prompt          string
	SubagentType    string
	Model           *string
	Resume          *string
	RunInBackground *bool
	MaxTurns        *int
	Name            *string // display name for the agent
	Mode            *string // permission mode override
}

// AgentResult contains the result from a subagent.
type AgentResult struct {
	AgentID string
	Output  string // final output (empty if background)
}

// SubagentSpawner creates and runs subagent instances.
type SubagentSpawner interface {
	Spawn(ctx context.Context, input AgentInput) (AgentResult, error)
}

// StubSubagentSpawner returns a not-configured message.
type StubSubagentSpawner struct{}

func (s *StubSubagentSpawner) Spawn(_ context.Context, _ AgentInput) (AgentResult, error) {
	return AgentResult{}, fmt.Errorf("subagent spawning not yet configured")
}

// AgentTool spawns subagent instances via a configurable spawner.
type AgentTool struct {
	Spawner SubagentSpawner
}

func (a *AgentTool) Name() string { return "Agent" }

func (a *AgentTool) Description() string {
	return "Launches a specialized agent to handle complex, multi-step tasks autonomously."
}

func (a *AgentTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"description": map[string]any{
				"type":        "string",
				"description": "A short (3-5 word) description of the task",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "The task for the agent to perform",
			},
			"subagent_type": map[string]any{
				"type":        "string",
				"description": "The type of specialized agent to use",
			},
			"model": map[string]any{
				"type":        "string",
				"description": "Optional model to use for this agent",
			},
			"resume": map[string]any{
				"type":        "string",
				"description": "Optional agent ID to resume from",
			},
			"run_in_background": map[string]any{
				"type":        "boolean",
				"description": "Set to true to run this agent in the background",
			},
			"max_turns": map[string]any{
				"type":        "integer",
				"description": "Maximum number of agentic turns before stopping",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Optional display name for the agent",
			},
			"mode": map[string]any{
				"type":        "string",
				"description": "Optional permission mode override",
			},
		},
		"required": []string{"description", "prompt", "subagent_type"},
	}
}

func (a *AgentTool) SideEffect() SideEffectType { return SideEffectSpawns }

func (a *AgentTool) Execute(ctx context.Context, input map[string]any) (ToolOutput, error) {
	description, ok := input["description"].(string)
	if !ok || description == "" {
		return ToolOutput{Content: "Error: description is required", IsError: true}, nil
	}

	prompt, ok := input["prompt"].(string)
	if !ok || prompt == "" {
		return ToolOutput{Content: "Error: prompt is required", IsError: true}, nil
	}

	subagentType, ok := input["subagent_type"].(string)
	if !ok || subagentType == "" {
		return ToolOutput{Content: "Error: subagent_type is required", IsError: true}, nil
	}

	spawner := a.Spawner
	if spawner == nil {
		spawner = &StubSubagentSpawner{}
	}

	agentInput := AgentInput{
		Description:  description,
		Prompt:       prompt,
		SubagentType: subagentType,
	}

	if m, ok := input["model"].(string); ok {
		agentInput.Model = &m
	}
	if r, ok := input["resume"].(string); ok {
		agentInput.Resume = &r
	}
	if bg, ok := input["run_in_background"].(bool); ok {
		agentInput.RunInBackground = &bg
	}
	if mt, ok := input["max_turns"].(float64); ok {
		turns := int(mt)
		agentInput.MaxTurns = &turns
	}
	if n, ok := input["name"].(string); ok {
		agentInput.Name = &n
	}
	if mode, ok := input["mode"].(string); ok {
		agentInput.Mode = &mode
	}

	result, err := spawner.Spawn(ctx, agentInput)
	if err != nil {
		return ToolOutput{
			Content: fmt.Sprintf("Error: %s", err),
			IsError: true,
		}, nil
	}

	if result.Output == "" && result.AgentID != "" {
		return ToolOutput{
			Content: fmt.Sprintf("Agent started in background. Agent ID: %s", result.AgentID),
		}, nil
	}

	content := result.Output
	if result.AgentID != "" {
		content += fmt.Sprintf("\n\nagentId: %s", result.AgentID)
	}

	return ToolOutput{Content: content}, nil
}
