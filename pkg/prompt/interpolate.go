package prompt

import "strings"

// PromptVars holds all variables for template interpolation in prompt files.
type PromptVars struct {
	// Main system prompt vars
	OutputStyleConfig *string // nil = no output style
	SecurityPolicy    string
	IssuesExplainer   string
	PackageURL        string
	ReadmeURL         string
	Version           string
	FeedbackChannel   string
	BuildTime         string

	// Tool names (for substitution in prompt templates)
	BashToolName           string
	ReadToolName           string
	WriteToolName          string
	EditToolName           string
	GlobToolName           string
	GrepToolName           string
	TaskToolName           string
	AskUserQuestionTool    string
	TodoToolName           string
	ExploreAgentType       string

	// Doing tasks
	ToolUsageHints []string

	// Git status
	CurrentBranch string
	MainBranch    string
	GitStatus     string
	RecentCommits string

	// Scratchpad
	ScratchpadDir string

	// Sessions path
	SessionsPath string
}

// DefaultPromptVars returns PromptVars with standard defaults.
func DefaultPromptVars() PromptVars {
	return PromptVars{
		SecurityPolicy:  loadSystemPrompt("system-prompt-censoring-assistance-with-malicious-activities.md"),
		IssuesExplainer: "report the issue at https://github.com/anthropics/claude-code/issues",
		PackageURL:      "@anthropic-ai/claude-code",
		ReadmeURL:       "https://code.claude.com/docs/en/overview",
		Version:         "2.1.37",
		FeedbackChannel: "https://github.com/anthropics/claude-code/issues",

		BashToolName:        "Bash",
		ReadToolName:        "Read",
		WriteToolName:       "Write",
		EditToolName:        "Edit",
		GlobToolName:        "Glob",
		GrepToolName:        "Grep",
		TaskToolName:        "Task",
		AskUserQuestionTool: "AskUserQuestion",
		TodoToolName:        "TodoWrite",
		ExploreAgentType:    "Explore",
	}
}

// interpolate performs simple variable substitution on a prompt template.
// It replaces ${VAR} patterns and handles the JS-style conditional/object patterns
// by converting them to their Go equivalents.
func interpolate(tmpl string, vars *PromptVars) string {
	if vars == nil {
		return tmpl
	}

	result := tmpl

	// Handle the main system prompt's conditional output style
	// ${OUTPUT_STYLE_CONFIG!==null?'...':"..."}
	if vars.OutputStyleConfig != nil {
		result = strings.ReplaceAll(result,
			`${OUTPUT_STYLE_CONFIG!==null?'according to your "Output Style" below, which describes how you should respond to user queries.':"with software engineering tasks."}`,
			`according to your "Output Style" below, which describes how you should respond to user queries.`)
	} else {
		result = strings.ReplaceAll(result,
			`${OUTPUT_STYLE_CONFIG!==null?'according to your "Output Style" below, which describes how you should respond to user queries.':"with software engineering tasks."}`,
			"with software engineering tasks.")
	}

	// Security policy
	result = strings.ReplaceAll(result, "${SECURITY_POLICY}\n", vars.SecurityPolicy+"\n")
	result = strings.ReplaceAll(result, "${SECURITY_POLICY}", vars.SecurityPolicy)

	// Feedback/issues explainer block — replace the complex JS object expression
	// Match the complex ${...ISSUES_EXPLAINER...} pattern on line 8 of main prompt
	if idx := strings.Index(result, "${{ISSUES_EXPLAINER:"); idx >= 0 {
		// Find the end of the expression
		depth := 0
		end := idx
		for i := idx; i < len(result); i++ {
			if result[i] == '{' {
				depth++
			} else if result[i] == '}' {
				depth--
				if depth == 0 {
					end = i
					break
				}
			}
		}
		result = result[:idx] + vars.IssuesExplainer + result[end+1:]
	}

	// Doing tasks — tool usage hints
	// Handle the ${" ..."} self-evaluating string expression
	result = strings.ReplaceAll(result,
		`${"- NEVER propose changes to code you haven't read. If a user asks about or wants you to modify a file, read it first. Understand existing code before suggesting modifications."}`,
		"- NEVER propose changes to code you haven't read. If a user asks about or wants you to modify a file, read it first. Understand existing code before suggesting modifications.")

	// Handle TOOL_USAGE_HINTS_ARRAY
	if idx := strings.Index(result, "${TOOL_USAGE_HINTS_ARRAY.length>0?"); idx >= 0 {
		end := findClosingBrace(result, idx)
		var replacement string
		if len(vars.ToolUsageHints) > 0 {
			replacement = "\n" + strings.Join(vars.ToolUsageHints, "\n")
		}
		result = result[:idx] + replacement + result[end+1:]
	}

	// Tool usage policy section vars
	result = strings.ReplaceAll(result, "${WEBFETCH_ENABLED_SECTION}", "")
	result = strings.ReplaceAll(result, "${MCP_TOOLS_SECTION}", "")

	// Tool name substitutions
	result = strings.ReplaceAll(result, "${BASH_TOOL_NAME}", vars.BashToolName)
	result = strings.ReplaceAll(result, "${READ_TOOL_NAME}", vars.ReadToolName)
	result = strings.ReplaceAll(result, "${WRITE_TOOL_NAME}", vars.WriteToolName)
	result = strings.ReplaceAll(result, "${EDIT_TOOL_NAME}", vars.EditToolName)
	result = strings.ReplaceAll(result, "${GLOB_TOOL_NAME}", vars.GlobToolName)
	result = strings.ReplaceAll(result, "${GREP_TOOL_NAME}", vars.GrepToolName)
	result = strings.ReplaceAll(result, "${TASK_TOOL_NAME}", vars.TaskToolName)
	result = strings.ReplaceAll(result, "${ASK_USER_QUESTION_TOOL}", vars.AskUserQuestionTool)
	result = strings.ReplaceAll(result, "${TODO_TOOL_OBJECT.name}", vars.TodoToolName)
	result = strings.ReplaceAll(result, "${EXPLORE_AGENT.agentType}", vars.ExploreAgentType)

	// Tool permission mode — AskUserQuestion conditional
	if strings.Contains(result, "${AVAILABLE_TOOLS_SET.has(ASK_USER_QUESTION_TOOL)") {
		// Always include the AskUserQuestion fallback (since we always have it)
		askSection := " If you do not understand why the user has denied a tool call, use the " + vars.AskUserQuestionTool + " to ask them."
		start := strings.Index(result, "${AVAILABLE_TOOLS_SET.has(ASK_USER_QUESTION_TOOL)")
		end := findClosingBrace(result, start)
		result = result[:start] + askSection + result[end+1:]
	}

	// Git status vars
	result = strings.ReplaceAll(result, "${CURRENT_BRANCH}", vars.CurrentBranch)
	result = strings.ReplaceAll(result, "${MAIN_BRANCH}", vars.MainBranch)
	if vars.GitStatus != "" {
		result = strings.ReplaceAll(result, `${GIT_STATUS||"(clean)"}`, vars.GitStatus)
	} else {
		result = strings.ReplaceAll(result, `${GIT_STATUS||"(clean)"}`, "(clean)")
	}
	result = strings.ReplaceAll(result, "${RECENT_COMMITS}", vars.RecentCommits)

	// Scratchpad dir
	result = strings.ReplaceAll(result, "${SCRATCHPAD_DIR_FN()}", vars.ScratchpadDir)

	// Sessions path
	result = strings.ReplaceAll(result, "${GET_SESSIONS_PATH_FN(GET_CWD_FN())}", vars.SessionsPath)

	// MCP CLI — tool list (complex JS map expression — simplify to empty for now)
	if idx := strings.Index(result, "${AVAILABLE_TOOLS_LIST.map"); idx >= 0 {
		end := findClosingBrace(result, idx)
		result = result[:idx] + result[end+1:]
	}

	return result
}

// findClosingBrace finds the closing } for a ${...} expression starting at pos.
func findClosingBrace(s string, pos int) int {
	depth := 0
	inSingle := false
	inDouble := false
	inBacktick := false

	for i := pos; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch == '\\' && i+1 < len(s):
			i++ // skip escaped char
		case ch == '\'' && !inDouble && !inBacktick:
			inSingle = !inSingle
		case ch == '"' && !inSingle && !inBacktick:
			inDouble = !inDouble
		case ch == '`' && !inSingle && !inDouble:
			inBacktick = !inBacktick
		case ch == '{' && !inSingle && !inDouble && !inBacktick:
			depth++
		case ch == '}' && !inSingle && !inDouble && !inBacktick:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(s) - 1 // fallback: end of string
}

// simpleReplace performs a key=value replacement on a template string.
func simpleReplace(tmpl string, vars map[string]string) string {
	result := tmpl
	for k, v := range vars {
		result = strings.ReplaceAll(result, "${"+k+"}", v)
	}
	return result
}
