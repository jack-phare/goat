package prompt

import (
	"strings"
	"testing"

	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/types"
)

func TestIntegration_FullAssembly(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		Model:           "claude-sonnet-4-5-20250929",
		CWD:             "/home/user/project",
		OS:              "linux",
		Shell:           "/bin/bash",
		PromptVersion:   "2.1.37",
		SessionsEnabled: true,
		MemoryEnabled:   true,
		GitBranch:       "feature/prompt-assembly",
		GitMainBranch:   "main",
		GitStatus:       "M pkg/prompt/assembler.go",
		GitRecentCommits: "abc1234 Add system prompt assembly\ndef5678 Initial commit",
		ClaudeMDContent: "Always run tests before committing.\nPrefer Go idiomatic patterns.",
		OutputStyle:     "Be concise and use bullet points.",
		SystemPrompt:    types.SystemPromptConfig{Append: "Remember: you are Goat, not Claude Code."},
		MCPServers: map[string]types.McpServerConfig{
			"slack": {Type: "stdio", Command: "mcp-slack"},
		},
		ScratchpadDir: "/tmp/scratch-session-123",
	}

	result := a.Assemble(config)

	// Verify all expected sections are present
	expectedSections := []string{
		"interactive CLI tool",          // Main prompt
		"authorized security testing",   // Censoring policy
		"Doing tasks",                   // Doing tasks
		"Executing actions with care",   // Actions with care
		"# Using your tools",            // Using your tools (dynamic section, GOAT-07c)
		"Tone and style",                // Tone and style
		"parallel tool",                 // Parallel tool call note
		"Accessing Past Sessions",       // Sessions
		"Agent Memory",                  // Memory
		"feature/prompt-assembly",       // Git branch
		"M pkg/prompt/assembler.go",     // Git status
		"MCP",                           // MCP
		"Scratchpad Directory",          // Scratchpad
		"# CLAUDE.md",                   // CLAUDE.md section
		"Always run tests",             // CLAUDE.md content
		"# Output Style",               // Output style section
		"Be concise",                   // Output style content
		"Remember: you are Goat",       // Appended text
	}

	for _, section := range expectedSections {
		if !strings.Contains(result, section) {
			t.Errorf("expected assembled prompt to contain %q", section)
		}
	}

	// Verify no unresolved template variables for known vars
	knownVars := []string{
		"${BASH_TOOL_NAME}",
		"${READ_TOOL_NAME}",
		"${WRITE_TOOL_NAME}",
		"${EDIT_TOOL_NAME}",
		"${SECURITY_POLICY}",
		"${OUTPUT_STYLE_CONFIG",
	}
	for _, v := range knownVars {
		if strings.Contains(result, v) {
			t.Errorf("unresolved template variable found: %s", v)
		}
	}

	// Verify token estimate is within expected range
	tokens := EstimateTokens(result)
	t.Logf("Full assembly: %d chars, ~%d tokens", len(result), tokens)
	if tokens < 2000 {
		t.Errorf("token estimate %d seems too low for full assembly", tokens)
	}
	if tokens > 50000 {
		t.Errorf("token estimate %d seems too high for full assembly", tokens)
	}

	// Verify output style triggers conditional in main prompt
	mustContain(t, result, `according to your "Output Style" below`)
}

func TestIntegration_MinimalAssembly(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		PromptVersion: "2.1.37",
	}

	result := a.Assemble(config)
	tokens := EstimateTokens(result)
	t.Logf("Minimal assembly: %d chars, ~%d tokens", len(result), tokens)

	// Even minimal should have core sections
	mustContain(t, result, "interactive CLI tool")
	mustContain(t, result, "with software engineering tasks.")
	mustContain(t, result, "Doing tasks")
}

func TestIntegration_SubagentAssembly(t *testing.T) {
	parentConfig := &agent.AgentConfig{
		CWD:             "/home/user/project",
		OS:              "linux",
		Shell:           "/bin/bash",
		ClaudeMDContent: "secret project instructions",
	}

	// Use built-in explore prompt
	agentDef := types.AgentDefinition{
		Prompt: ExplorePrompt(),
	}

	result := AssembleSubagentPrompt(agentDef, parentConfig)

	// Should have agent prompt + environment
	if result == "" {
		t.Fatal("subagent prompt should not be empty")
	}
	mustContain(t, result, "/home/user/project")
	mustContain(t, result, "linux")

	// Should NOT have parent CLAUDE.md
	mustNotContain(t, result, "secret project instructions")
}

func TestIntegration_LoadClaudeMDAndAssemble(t *testing.T) {
	// Test the full flow: LoadClaudeMD → set on config → Assemble
	dir := t.TempDir()
	writeFile(t, dir+"/CLAUDE.md", "# Project Rules\n\nAlways format with gofmt.")

	content := LoadClaudeMD(dir)
	if content == "" {
		t.Fatal("LoadClaudeMD should find the file")
	}

	a := &Assembler{}
	config := &agent.AgentConfig{
		CWD:             dir,
		ClaudeMDContent: content,
		PromptVersion:   "2.1.37",
	}

	result := a.Assemble(config)
	mustContain(t, result, "Project Rules")
	mustContain(t, result, "Always format with gofmt")
}
