package prompt

import (
	"fmt"
	"strings"

	"github.com/jg-phare/goat/pkg/agent"
)

// Assembler builds system prompts from embedded prompt parts.
// It implements agent.SystemPromptAssembler.
type Assembler struct{}

// Assemble composes the full system prompt from embedded parts based on config.
func (a *Assembler) Assemble(config *agent.AgentConfig) string {
	// Check for custom override (Raw field)
	if config.SystemPrompt.Raw != "" {
		return config.SystemPrompt.Raw
	}

	vars := buildVars(config)
	var parts []string

	// 1. Main system prompt (always)
	parts = append(parts, interpolate(loadSystemPrompt("system-prompt-main-system-prompt.md"), vars))

	// 2. Censoring policy (always)
	parts = append(parts, loadSystemPrompt("system-prompt-censoring-assistance-with-malicious-activities.md"))

	// 3. Doing tasks (always)
	parts = append(parts, interpolate(loadSystemPrompt("system-prompt-doing-tasks.md"), vars))

	// 4. Executing actions with care (always)
	parts = append(parts, loadSystemPrompt("system-prompt-executing-actions-with-care.md"))

	// 5. Tool usage policy (always)
	parts = append(parts, interpolate(loadSystemPrompt("system-prompt-tool-usage-policy.md"), vars))

	// 6. Tool permission mode (always)
	parts = append(parts, interpolate(loadSystemPrompt("system-prompt-tool-permission-mode.md"), vars))

	// 7. Tone and style (always)
	parts = append(parts, interpolate(loadSystemPrompt("system-prompt-tone-and-style.md"), vars))

	// 8. Task management (conditional: TodoWrite enabled)
	if toolEnabled(config, "TodoWrite") {
		parts = append(parts, interpolate(loadSystemPrompt("system-prompt-task-management.md"), vars))
	}

	// 9. Parallel tool call note (always)
	parts = append(parts, loadSystemPrompt("system-prompt-parallel-tool-call-note-part-of-tool-usage-policy.md"))

	// 10. Accessing past sessions (conditional: SessionsEnabled)
	if config.SessionsEnabled {
		parts = append(parts, interpolate(loadSystemPrompt("system-prompt-accessing-past-sessions.md"), vars))
	}

	// 11. Agent memory instructions (conditional: MemoryEnabled)
	if config.MemoryEnabled {
		parts = append(parts, loadSystemPrompt("system-prompt-agent-memory-instructions.md"))
	}

	// 12. Learning mode (conditional: LearningMode)
	if config.LearningMode {
		parts = append(parts, interpolate(loadSystemPrompt("system-prompt-learning-mode.md"), vars))
	}

	// 13. Git status (conditional: git info available)
	if config.GitBranch != "" {
		parts = append(parts, interpolate(loadSystemPrompt("system-prompt-git-status.md"), vars))
	}

	// 14. MCP CLI (conditional: MCP servers configured)
	if len(config.MCPServers) > 0 {
		parts = append(parts, interpolate(loadSystemPrompt("system-prompt-mcp-cli.md"), vars))
	}

	// 15. Scratchpad directory (conditional: ScratchpadDir set)
	if config.ScratchpadDir != "" {
		parts = append(parts, interpolate(loadSystemPrompt("system-prompt-scratchpad-directory.md"), vars))
	}

	// 16. Hooks configuration (conditional: hooks configured)
	// Note: We include the hooks doc if HookRunner is not a NoOp — but since we
	// can't inspect that, we always include it. It's informational.
	// Deferred: only include when hooks are actually configured.

	// 17. Teammate communication (conditional: swarm mode — deferred)

	// 18. Chrome browser MCP tools (conditional: chrome MCP — deferred)

	// 19. CLAUDE.md instructions (conditional: ClaudeMDContent set)
	if config.ClaudeMDContent != "" {
		parts = append(parts, formatClaudeMDSection(config.ClaudeMDContent))
	}

	// 20. Output style (conditional: OutputStyle set)
	if config.OutputStyle != "" {
		parts = append(parts, formatOutputStyleSection(config.OutputStyle))
	}

	// 20b. Available skills (conditional: skills loaded)
	if config.Skills != nil && len(config.Skills.SkillNames()) > 0 {
		skillsList := config.Skills.FormatSkillsList()
		reminder := GetReminder(ReminderAvailableSkills, map[string]string{
			"FORMATTED_SKILLS_LIST": skillsList,
		})
		if reminder != "" {
			parts = append(parts, reminder)
		}
	}

	// 21. File read limits (always)
	parts = append(parts, loadSystemPrompt("system-prompt-file-read-limits.md"))

	// 21b. Large file handling (always)
	parts = append(parts, loadSystemPrompt("system-prompt-large-file-handling.md"))

	// 21c. Verify assumptions (always)
	parts = append(parts, loadSystemPrompt("system-prompt-use-tools-to-verify.md"))

	// 21d. Environment details (always)
	parts = append(parts, formatEnvironmentDetails(config))

	// 22. Tool documentation (conditional: specific tools enabled)
	for _, td := range toolDocs {
		if toolEnabled(config, td.toolName) {
			content := loadToolPrompt(td.promptFile)
			if content != "" {
				parts = append(parts, content)
			}
		}
	}

	// 23. User append (from SystemPromptConfig.Append)
	if config.SystemPrompt.Append != "" {
		parts = append(parts, config.SystemPrompt.Append)
	}

	// Filter empty parts and join
	var nonEmpty []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			nonEmpty = append(nonEmpty, trimmed)
		}
	}

	return strings.Join(nonEmpty, "\n\n")
}

// buildVars constructs the PromptVars from an AgentConfig.
func buildVars(config *agent.AgentConfig) *PromptVars {
	vars := DefaultPromptVars()

	if config.OutputStyle != "" {
		vars.OutputStyleConfig = &config.OutputStyle
	}

	if config.PromptVersion != "" {
		vars.Version = config.PromptVersion
	}

	// Git vars
	vars.CurrentBranch = config.GitBranch
	vars.MainBranch = config.GitMainBranch
	vars.GitStatus = config.GitStatus
	vars.RecentCommits = config.GitRecentCommits

	// Scratchpad
	vars.ScratchpadDir = config.ScratchpadDir

	// Sessions path
	if config.CWD != "" {
		vars.SessionsPath = fmt.Sprintf("~/.claude/projects/%s", config.CWD)
	}

	return &vars
}

// toolEnabled checks if a tool is available and not disabled in the registry.
func toolEnabled(config *agent.AgentConfig, name string) bool {
	if config.ToolRegistry == nil {
		return false
	}
	_, exists := config.ToolRegistry.Get(name)
	return exists && !config.ToolRegistry.IsDisabled(name)
}

// toolDocEntry maps a tool name to its prompt documentation file.
type toolDocEntry struct {
	toolName   string
	promptFile string
}

// toolDocs lists tools that have dedicated documentation to inject into the system prompt.
var toolDocs = []toolDocEntry{
	{"Glob", "tool-description-glob.md"},
	{"Grep", "tool-description-grep.md"},
	{"FileEdit", "tool-description-edit.md"},
	{"Edit", "tool-description-edit.md"},
	{"Read", "tool-description-readfile.md"},
	{"FileRead", "tool-description-readfile.md"},
	{"Write", "tool-description-write.md"},
	{"FileWrite", "tool-description-write.md"},
	{"Bash", "tool-description-bash.md"},
	{"NotebookEdit", "tool-description-notebookedit.md"},
	{"WebFetch", "tool-description-webfetch.md"},
	{"WebSearch", "tool-description-websearch.md"},
	{"Skill", "tool-description-skill.md"},
}

// formatClaudeMDSection wraps CLAUDE.md content in a section header.
func formatClaudeMDSection(content string) string {
	return "# CLAUDE.md\n\nCLAUDE.md is a file that the user places in the project root to provide instructions to Claude. Below are the combined contents of all CLAUDE.md files found in the project directory hierarchy.\n\n" + content
}

// formatOutputStyleSection wraps output style content in a section header.
func formatOutputStyleSection(style string) string {
	return "# Output Style\n\n" + style
}
