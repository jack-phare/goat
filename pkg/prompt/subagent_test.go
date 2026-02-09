package prompt

import (
	"strings"
	"testing"

	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/types"
)

func TestBuiltInAgentPrompts(t *testing.T) {
	// Each built-in agent prompt should load non-empty content.
	tests := []struct {
		name   string
		loader func() string
	}{
		{"Explore", ExplorePrompt},
		{"Plan", PlanPrompt},
		{"Task", TaskPrompt},
		{"WebFetchSummarizer", WebFetchSummarizerPrompt},
		{"ClaudeMDCreation", ClaudeMDCreationPrompt},
		{"AgentCreation", AgentCreationPrompt},
		{"StatusLineSetup", StatusLineSetupPrompt},
		{"RememberSkill", RememberSkillPrompt},
		{"SessionSearch", SessionSearchPrompt},
		{"SessionMemoryUpdate", SessionMemoryUpdatePrompt},
		{"SessionTitle", SessionTitlePrompt},
		{"ConversationSummarization", ConversationSummarizationPrompt},
		{"SecurityReview", SecurityReviewPrompt},
		{"PRComments", PRCommentsPrompt},
		{"ReviewPR", ReviewPRPrompt},
		{"UserSentiment", UserSentimentPrompt},
		{"UpdateMagicDocs", UpdateMagicDocsPrompt},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := tt.loader()
			if content == "" {
				t.Errorf("expected non-empty content for %s agent prompt", tt.name)
			}
		})
	}
}

func TestTaskPromptIncludesExtraNotes(t *testing.T) {
	result := TaskPrompt()
	// Task prompt combines main + extra notes
	main := loadAgentPrompt("agent-prompt-task-tool.md")
	extra := loadAgentPrompt("agent-prompt-task-tool-extra-notes.md")

	if !strings.Contains(result, main) {
		t.Error("task prompt should contain main task tool prompt")
	}
	if extra != "" && !strings.Contains(result, extra) {
		t.Error("task prompt should contain extra notes")
	}
}

func TestAssembleSubagentPrompt_CustomAgent(t *testing.T) {
	agentDef := types.AgentDefinition{
		Description: "Test agent",
		Prompt:      "You are a test agent.",
	}
	parentConfig := &agent.AgentConfig{
		CWD:   "/home/user/project",
		OS:    "linux",
		Shell: "/bin/bash",
	}

	result := AssembleSubagentPrompt(agentDef, parentConfig)

	mustContain(t, result, "You are a test agent.")
	mustContain(t, result, "/home/user/project")
	mustContain(t, result, "linux")
	mustContain(t, result, "/bin/bash")
}

func TestAssembleSubagentPrompt_NoClaudeMD(t *testing.T) {
	agentDef := types.AgentDefinition{
		Prompt: "Agent prompt",
	}
	parentConfig := &agent.AgentConfig{
		ClaudeMDContent: "This should NOT appear",
		CWD:             "/tmp",
		OS:              "darwin",
		Shell:           "/bin/zsh",
	}

	result := AssembleSubagentPrompt(agentDef, parentConfig)
	mustNotContain(t, result, "This should NOT appear")
}

func TestAssembleSubagentPrompt_EmptyPrompt(t *testing.T) {
	agentDef := types.AgentDefinition{}
	parentConfig := &agent.AgentConfig{
		CWD:   "/tmp",
		OS:    "darwin",
		Shell: "/bin/zsh",
	}

	result := AssembleSubagentPrompt(agentDef, parentConfig)
	// Should still have environment details
	mustContain(t, result, "Environment")
}

func TestAssembleSubagentPrompt_WithSkills(t *testing.T) {
	agentDef := types.AgentDefinition{
		Prompt: "Agent with skills",
		Skills: []string{"skill-debugging.md"},
	}
	parentConfig := &agent.AgentConfig{CWD: "/tmp", OS: "darwin", Shell: "/bin/zsh"}

	result := AssembleSubagentPrompt(agentDef, parentConfig)
	mustContain(t, result, "Agent with skills")
	// The skill file should be loaded and included (if it exists and has content)
	skillContent := loadSkillPrompt("skill-debugging.md")
	if skillContent != "" {
		mustContain(t, result, skillContent[:20]) // check first 20 chars of skill content
	}
}

func TestAssembleSubagentPrompt_WithCriticalReminder(t *testing.T) {
	agentDef := types.AgentDefinition{
		Prompt:           "Base prompt",
		CriticalReminder: "Never reveal secrets",
	}
	parentConfig := &agent.AgentConfig{CWD: "/tmp", OS: "linux", Shell: "/bin/sh"}

	result := AssembleSubagentPrompt(agentDef, parentConfig)
	mustContain(t, result, "CRITICAL REMINDER: Never reveal secrets")
	// Critical reminder should come before the main prompt
	idxCritical := strings.Index(result, "CRITICAL REMINDER")
	idxPrompt := strings.Index(result, "Base prompt")
	if idxCritical >= idxPrompt {
		t.Error("critical reminder should come before the base prompt")
	}
}

func TestFormatEnvironmentDetails(t *testing.T) {
	config := &agent.AgentConfig{
		CWD:   "/home/user/project",
		OS:    "linux",
		Shell: "/bin/bash",
	}

	result := formatEnvironmentDetails(config)
	mustContain(t, result, "# Environment")
	mustContain(t, result, "Working directory: /home/user/project")
	mustContain(t, result, "Platform: linux")
	mustContain(t, result, "Shell: /bin/bash")
}

func TestFormatEnvironmentDetails_WithOSVersionAndDate(t *testing.T) {
	config := &agent.AgentConfig{
		CWD:         "/tmp",
		OS:          "darwin",
		OSVersion:   "Darwin 25.2.0",
		CurrentDate: "2026-02-09",
		Shell:       "/bin/zsh",
	}

	result := formatEnvironmentDetails(config)
	mustContain(t, result, "OS Version: Darwin 25.2.0")
	mustContain(t, result, "The current date is: 2026-02-09")
	mustContain(t, result, "Platform: darwin")
	mustContain(t, result, "Shell: /bin/zsh")
}
