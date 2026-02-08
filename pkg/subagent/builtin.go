package subagent

import (
	"github.com/jg-phare/goat/pkg/prompt"
	"github.com/jg-phare/goat/pkg/types"
)

// BuiltInAgents returns the default set of built-in agent definitions.
func BuiltInAgents() map[string]Definition {
	defs := make(map[string]Definition)

	// general-purpose: inherits all parent tools
	defs["general-purpose"] = FromTypesDefinition("general-purpose", types.AgentDefinition{
		Description: "General-purpose agent for researching complex questions, searching for code, and executing multi-step tasks.",
		Prompt:      prompt.TaskPrompt(),
	}, SourceBuiltIn, 0)

	// Explore: fast search agent with limited tools
	defs["Explore"] = FromTypesDefinition("Explore", types.AgentDefinition{
		Description:     "Fast agent specialized for exploring codebases.",
		Prompt:          prompt.ExplorePrompt(),
		Model:           "haiku",
		DisallowedTools: []string{"Write", "Edit", "NotebookEdit", "Agent", "ExitPlanMode"},
	}, SourceBuiltIn, 0)

	// Plan: architecture agent, no write tools
	defs["Plan"] = FromTypesDefinition("Plan", types.AgentDefinition{
		Description:     "Software architect agent for designing implementation plans.",
		Prompt:          prompt.PlanPrompt(),
		DisallowedTools: []string{"Write", "Edit", "NotebookEdit", "Agent", "ExitPlanMode"},
	}, SourceBuiltIn, 0)

	// Bash: command execution specialist
	defs["Bash"] = FromTypesDefinition("Bash", types.AgentDefinition{
		Description: "Command execution specialist for running bash commands.",
		Prompt:      "You are a command execution specialist. Execute the requested bash commands and return the results.",
		Tools:       []string{"Bash"},
	}, SourceBuiltIn, 0)

	// statusline-setup: status line configuration
	defs["statusline-setup"] = FromTypesDefinition("statusline-setup", types.AgentDefinition{
		Description: "Configure the user's Claude Code status line setting.",
		Prompt:      prompt.StatusLineSetupPrompt(),
		Model:       "sonnet",
		Tools:       []string{"Read", "Edit"},
	}, SourceBuiltIn, 0)

	// claude-code-guide: documentation specialist
	defs["claude-code-guide"] = FromTypesDefinition("claude-code-guide", types.AgentDefinition{
		Description: "Answer questions about Claude Code features, hooks, slash commands, MCP servers, settings, and IDE integrations.",
		Prompt:      "You are a Claude Code documentation specialist. Answer questions about Claude Code features and usage.",
		Model:       "haiku",
		Tools:       []string{"Glob", "Grep", "Read", "WebFetch", "WebSearch"},
	}, SourceBuiltIn, 0)

	return defs
}
