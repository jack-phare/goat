package prompt

import (
	"fmt"
	"strings"

	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/types"
)

// AssembleSubagentPrompt builds a system prompt for a subagent.
// It uses the agent definition's prompt as the base and appends environment details.
// It does NOT include the parent's CLAUDE.md by default.
func AssembleSubagentPrompt(agentDef types.AgentDefinition, parentConfig *agent.AgentConfig) string {
	var parts []string

	// 1. Agent's custom prompt
	if agentDef.Prompt != "" {
		parts = append(parts, agentDef.Prompt)
	}

	// 2. Environment details
	parts = append(parts, formatEnvironmentDetails(parentConfig))

	return strings.Join(parts, "\n\n")
}

// formatEnvironmentDetails creates an environment section from config.
func formatEnvironmentDetails(config *agent.AgentConfig) string {
	var lines []string
	lines = append(lines, "# Environment")
	if config.CWD != "" {
		lines = append(lines, fmt.Sprintf("- Working directory: %s", config.CWD))
	}
	if config.OS != "" {
		lines = append(lines, fmt.Sprintf("- Platform: %s", config.OS))
	}
	if config.Shell != "" {
		lines = append(lines, fmt.Sprintf("- Shell: %s", config.Shell))
	}
	return strings.Join(lines, "\n")
}

// Built-in agent prompt accessors — each loads from the embedded agents directory.

func ExplorePrompt() string {
	return loadAgentPrompt("agent-prompt-explore.md")
}

func PlanPrompt() string {
	return loadAgentPrompt("agent-prompt-plan-mode-enhanced.md")
}

func TaskPrompt() string {
	main := loadAgentPrompt("agent-prompt-task-tool.md")
	extra := loadAgentPrompt("agent-prompt-task-tool-extra-notes.md")
	if extra != "" {
		return main + "\n\n" + extra
	}
	return main
}

func WebFetchSummarizerPrompt() string {
	return loadAgentPrompt("agent-prompt-webfetch-summarizer.md")
}

func ClaudeMDCreationPrompt() string {
	return loadAgentPrompt("agent-prompt-claudemd-creation.md")
}

func AgentCreationPrompt() string {
	return loadAgentPrompt("agent-prompt-agent-creation-architect.md")
}

func StatusLineSetupPrompt() string {
	return loadAgentPrompt("agent-prompt-status-line-setup.md")
}

func RememberSkillPrompt() string {
	return loadAgentPrompt("agent-prompt-remember-skill.md")
}

func SessionSearchPrompt() string {
	return loadAgentPrompt("agent-prompt-session-search-assistant.md")
}

func SessionMemoryUpdatePrompt() string {
	return loadAgentPrompt("agent-prompt-session-memory-update-instructions.md")
}

func SessionTitlePrompt() string {
	return loadAgentPrompt("agent-prompt-session-title-and-branch-generation.md")
}

func ConversationSummarizationPrompt() string {
	return loadAgentPrompt("agent-prompt-conversation-summarization.md")
}

func SecurityReviewPrompt() string {
	return loadAgentPrompt("agent-prompt-security-review-slash-command.md")
}

func PRCommentsPrompt() string {
	return loadAgentPrompt("agent-prompt-pr-comments-slash-command.md")
}

func ReviewPRPrompt() string {
	return loadAgentPrompt("agent-prompt-review-pr-slash-command.md")
}

func UserSentimentPrompt() string {
	return loadAgentPrompt("agent-prompt-user-sentiment-analysis.md")
}

func UpdateMagicDocsPrompt() string {
	return loadAgentPrompt("agent-prompt-update-magic-docs.md")
}
