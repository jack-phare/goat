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

// AgentMetrics contains execution metrics from a subagent run.
type AgentMetrics struct {
	DurationSecs float64
	TurnCount    int
	CostUSD      float64
	InputTokens  int
	OutputTokens int
}

// AgentResult contains the result from a subagent.
type AgentResult struct {
	AgentID    string
	Output     string        // final output (empty if background)
	OutputFile string        // path to output file (background agents only)
	Error      string        // error message from subagent (empty on success)
	Metrics    *AgentMetrics // execution metrics (nil for background agents)
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
	return `Launch a new agent to handle complex, multi-step tasks autonomously.

The Agent tool launches specialized agents (subprocesses) that autonomously handle complex tasks. Each agent type has specific capabilities and tools available to it.

When NOT to use the Agent tool:
- If you want to read a specific file path, use the Read or Glob tool instead of the Agent tool, to find the match more quickly
- If you are searching for a specific class definition like "class Foo", use the Glob tool instead, to find the match more quickly
- If you are searching for code within a specific file or set of 2-3 files, use the Read tool instead of the Agent tool, to find the match more quickly
- Other tasks that are not related to the agent descriptions above

Usage notes:
- Always include a short description (3-5 words) summarizing what the agent will do
- Launch multiple agents concurrently whenever possible, to maximize performance; to do that, use a single message with multiple tool uses
- When the agent is done, it will return a single message back to you. The result returned by the agent is not visible to the user. To show the user the result, you should send a text message back to the user with a concise summary of the result.
- You can optionally run agents in the background using the run_in_background parameter. When an agent runs in the background, the tool result will include an output_file path. To check on the agent's progress or retrieve its results, use the Read tool to read the output file, or use Bash with ` + "`tail`" + ` to see recent output. You can continue working while background agents run.
- Agents can be resumed using the resume parameter by passing the agent ID from a previous invocation. When resumed, the agent continues with its full previous context preserved. When NOT resuming, each invocation starts fresh and you should provide a detailed task description with all necessary context.
- When the agent is done, it will return a single message back to you along with its agent ID. You can use this ID to resume the agent later if needed for follow-up work.
- Provide clear, detailed prompts so the agent can work autonomously and return exactly the information you need.
- The agent's outputs should generally be trusted
- Clearly tell the agent whether you expect it to write code or just to do research (search, file reads, web fetches, etc.), since it is not aware of the user's intent
- If the user specifies that they want you to run agents "in parallel", you MUST send a single message with multiple Agent tool use content blocks.`
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
		msg := fmt.Sprintf("Agent started in background. Agent ID: %s", result.AgentID)
		if result.OutputFile != "" {
			msg += fmt.Sprintf("\nOutput file: %s", result.OutputFile)
		}
		return ToolOutput{Content: msg}, nil
	}

	content := result.Output
	if result.AgentID != "" {
		content += fmt.Sprintf("\n\nagentId: %s", result.AgentID)
	}

	if result.Metrics != nil {
		m := result.Metrics
		content += fmt.Sprintf("\n---\nDuration: %.1fs | Turns: %d | Cost: $%.4f | Tokens: %d in / %d out",
			m.DurationSecs, m.TurnCount, m.CostUSD, m.InputTokens, m.OutputTokens)
	}

	if result.Error != "" {
		content += fmt.Sprintf("\n\nError: %s", result.Error)
		return ToolOutput{Content: content, IsError: true}, nil
	}

	return ToolOutput{Content: content}, nil
}
