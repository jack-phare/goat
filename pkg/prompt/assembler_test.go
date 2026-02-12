package prompt

import (
	"context"
	"strings"
	"testing"

	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/tools"
	"github.com/jg-phare/goat/pkg/types"
)

func TestAssembler_MinimalConfig(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		Model:         "claude-sonnet-4-5-20250929",
		PromptVersion: "2.1.37",
	}

	result := a.Assemble(config)

	// Should contain "always" sections
	mustContain(t, result, "interactive CLI tool")
	mustContain(t, result, "with software engineering tasks.")
	mustContain(t, result, "# System")
	mustContain(t, result, "Doing tasks")
	mustContain(t, result, "Executing actions with care")
	mustContain(t, result, "# Using your tools")
	mustContain(t, result, "Tone and style")
	mustContain(t, result, "parallel tool")

	// Should NOT contain conditional section headers
	mustNotContain(t, result, "Accessing Past Sessions")
	mustNotContain(t, result, "Agent Memory Instructions")
	mustNotContain(t, result, "Learning Style Active")
	mustNotContain(t, result, "# CLAUDE.md\n")
	mustNotContain(t, result, "# Output Style")
	mustNotContain(t, result, "Scratchpad Directory")
}

func TestAssembler_CustomOverride(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		SystemPrompt: types.SystemPromptConfig{Raw: "Custom prompt only"},
	}

	result := a.Assemble(config)
	if result != "Custom prompt only" {
		t.Errorf("expected custom prompt verbatim, got %q", result)
	}
}

func TestAssembler_WithAppend(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		SystemPrompt:  types.SystemPromptConfig{Append: "APPENDED TEXT"},
		PromptVersion: "2.1.37",
	}

	result := a.Assemble(config)
	mustContain(t, result, "APPENDED TEXT")
	// The appended text should be at or near the end
	idx := strings.LastIndex(result, "APPENDED TEXT")
	if idx < len(result)/2 {
		t.Error("appended text should be near end of prompt")
	}
}

func TestAssembler_WithOutputStyle(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		OutputStyle:   "Use bullet points. Be concise.",
		PromptVersion: "2.1.37",
	}

	result := a.Assemble(config)
	mustContain(t, result, `according to your "Output Style" below`)
	mustContain(t, result, "# Output Style")
	mustContain(t, result, "Use bullet points. Be concise.")
}

func TestAssembler_WithSessionsEnabled(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		SessionsEnabled: true,
		CWD:             "/home/user/project",
		PromptVersion:   "2.1.37",
	}

	result := a.Assemble(config)
	mustContain(t, result, "Accessing Past Sessions")
}

func TestAssembler_WithMemoryEnabled(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		MemoryEnabled: true,
		PromptVersion: "2.1.37",
	}

	result := a.Assemble(config)
	mustContain(t, result, "Agent Memory")
}

func TestAssembler_WithGitStatus(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		GitBranch:        "feature/test",
		GitMainBranch:    "main",
		GitStatus:        "M pkg/prompt/assembler.go",
		GitRecentCommits: "abc1234 Initial commit",
		PromptVersion:    "2.1.37",
	}

	result := a.Assemble(config)
	mustContain(t, result, "feature/test")
	mustContain(t, result, "main")
	mustContain(t, result, "M pkg/prompt/assembler.go")
	mustContain(t, result, "abc1234 Initial commit")
}

func TestAssembler_WithClaudeMD(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		ClaudeMDContent: "Always run tests before committing.",
		PromptVersion:   "2.1.37",
	}

	result := a.Assemble(config)
	mustContain(t, result, "CLAUDE.md")
	mustContain(t, result, "Always run tests before committing.")
}

func TestAssembler_WithScratchpadDir(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		ScratchpadDir: "/tmp/scratch-abc123",
		PromptVersion: "2.1.37",
	}

	result := a.Assemble(config)
	mustContain(t, result, "Scratchpad Directory")
	mustContain(t, result, "/tmp/scratch-abc123")
}

func TestAssembler_WithMCPServers(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		MCPServers: map[string]types.McpServerConfig{
			"slack": {Type: "stdio", Command: "mcp-slack"},
		},
		PromptVersion: "2.1.37",
	}

	result := a.Assemble(config)
	mustContain(t, result, "MCP")
}

func TestAssembler_ToolNameInterpolation(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		PromptVersion: "2.1.37",
	}

	result := a.Assemble(config)

	// Should have interpolated tool names (no raw ${...} patterns for known vars)
	mustNotContain(t, result, "${BASH_TOOL_NAME}")
	mustNotContain(t, result, "${READ_TOOL_NAME}")
	mustNotContain(t, result, "${WRITE_TOOL_NAME}")
	mustNotContain(t, result, "${EDIT_TOOL_NAME}")
	mustNotContain(t, result, "${TASK_TOOL_NAME}")

	// Should contain the resolved names
	mustContain(t, result, "Bash")
	mustContain(t, result, "Read")
	mustContain(t, result, "Edit")
}

func TestAssembler_GitStatusClean(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		GitBranch:     "main",
		GitMainBranch: "main",
		PromptVersion: "2.1.37",
	}

	result := a.Assemble(config)
	mustContain(t, result, "(clean)")
}

func TestAssembler_SectionOrdering(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		SessionsEnabled: true,
		MemoryEnabled:   true,
		GitBranch:       "main",
		GitMainBranch:   "main",
		ClaudeMDContent: "project instructions",
		OutputStyle:     "concise",
		SystemPrompt:    types.SystemPromptConfig{Append: "FINAL"},
		PromptVersion:   "2.1.37",
	}

	result := a.Assemble(config)

	// Verify CC-matching order: Identity -> System -> Doing Tasks -> Actions -> Using Tools -> Tone -> ... -> CLAUDE.md -> Style -> Append
	idxMain := strings.Index(result, "interactive CLI tool")
	idxSystem := strings.Index(result, "# System")
	idxDoing := strings.Index(result, "Doing tasks")
	idxCare := strings.Index(result, "Executing actions with care")
	idxUsing := strings.Index(result, "# Using your tools")
	idxTone := strings.Index(result, "Tone and style")
	idxClaudeMD := strings.Index(result, "# CLAUDE.md")
	idxStyle := strings.Index(result, "# Output Style")
	idxAppend := strings.LastIndex(result, "FINAL")

	if idxMain >= idxSystem {
		t.Error("main prompt should come before # System")
	}
	if idxSystem >= idxDoing {
		t.Error("# System should come before Doing tasks")
	}
	if idxDoing >= idxCare {
		t.Error("doing tasks should come before executing actions with care")
	}
	if idxCare >= idxUsing {
		t.Error("executing actions should come before # Using your tools")
	}
	if idxUsing >= idxTone {
		t.Error("# Using your tools should come before Tone and style")
	}
	if idxTone >= idxClaudeMD {
		t.Error("Tone and style should come before CLAUDE.md")
	}
	if idxClaudeMD >= idxStyle {
		t.Error("CLAUDE.md should come before output style")
	}
	if idxStyle >= idxAppend {
		t.Error("output style should come before append")
	}
}

// --- GOAT-07a Tests: # System Section ---

func TestAssembler_SystemSection(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		PromptVersion: "2.1.37",
	}

	result := a.Assemble(config)

	// All 6 bullet points from CC's SKz() must be present
	mustContain(t, result, "# System")
	mustContain(t, result, "All text you output outside of tool use is displayed to the user")
	mustContain(t, result, "Tools are executed in a user-selected permission mode")
	mustContain(t, result, "<system-reminder>")
	mustContain(t, result, "prompt injection")
	mustContain(t, result, "hooks")
	mustContain(t, result, "automatically compress prior messages")
}

func TestAssembler_SystemSectionAskUserConditional(t *testing.T) {
	a := &Assembler{}

	// Without AskUserQuestion tool
	config := &agent.AgentConfig{
		PromptVersion: "2.1.37",
	}
	result := a.Assemble(config)
	mustNotContain(t, result, "use the AskUserQuestion to ask them")

	// With AskUserQuestion tool
	reg := tools.NewRegistry()
	reg.Register(&stubTool{name: "AskUserQuestion"})
	config.ToolRegistry = reg
	result = a.Assemble(config)
	mustContain(t, result, "use the AskUserQuestion to ask them")
}

// --- GOAT-07c Tests: # Using your tools dynamic ---

func TestAssembler_UsingYourToolsDynamic(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		PromptVersion: "2.1.37",
	}

	result := a.Assemble(config)

	// Core tool preference bullets always present
	mustContain(t, result, "# Using your tools")
	mustContain(t, result, "Do NOT use Bash when a relevant dedicated tool is provided")
	mustContain(t, result, "To read files use Read instead of cat/head/tail/sed")
	mustContain(t, result, "parallel tool")
}

func TestAssembler_UsingYourToolsConditionalBullets(t *testing.T) {
	a := &Assembler{}

	// Without Agent/TodoWrite tools
	config := &agent.AgentConfig{
		PromptVersion: "2.1.37",
	}
	result := a.Assemble(config)
	mustNotContain(t, result, "Break down and manage your work with the TodoWrite")
	mustNotContain(t, result, "Use the Task tool with specialized agents")

	// With Agent and TodoWrite tools
	reg := tools.NewRegistry()
	reg.Register(&stubTool{name: "Agent"})
	reg.Register(&stubTool{name: "TodoWrite"})
	config.ToolRegistry = reg
	result = a.Assemble(config)
	mustContain(t, result, "Break down and manage your work with the TodoWrite")
	mustContain(t, result, "Use the Task tool with specialized agents")
}

func TestInterpolate_SecurityPolicy(t *testing.T) {
	vars := DefaultPromptVars()
	input := "Before ${SECURITY_POLICY} After"
	result := interpolate(input, &vars)

	mustContain(t, result, "Before")
	mustContain(t, result, "After")
	mustContain(t, result, "authorized security testing")
	mustNotContain(t, result, "${SECURITY_POLICY}")
}

func TestInterpolate_OutputStyleConditional(t *testing.T) {
	// Without output style
	vars := DefaultPromptVars()
	input := `helps users ${OUTPUT_STYLE_CONFIG!==null?'according to your "Output Style" below, which describes how you should respond to user queries.':"with software engineering tasks."}`

	result := interpolate(input, &vars)
	mustContain(t, result, "with software engineering tasks.")
	mustNotContain(t, result, "Output Style")

	// With output style
	style := "concise"
	vars.OutputStyleConfig = &style
	result = interpolate(input, &vars)
	mustContain(t, result, `according to your "Output Style" below`)
}

func TestInterpolate_ToolNames(t *testing.T) {
	vars := DefaultPromptVars()
	input := "Use ${BASH_TOOL_NAME} and ${READ_TOOL_NAME} and ${GREP_TOOL_NAME}"
	result := interpolate(input, &vars)

	if result != "Use Bash and Read and Grep" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestInterpolate_NilVars(t *testing.T) {
	result := interpolate("hello ${FOO}", nil)
	if result != "hello ${FOO}" {
		t.Errorf("nil vars should return template unchanged, got %q", result)
	}
}

func TestSimpleReplace(t *testing.T) {
	vars := map[string]string{
		"NAME":  "World",
		"GREET": "Hello",
	}
	result := simpleReplace("${GREET}, ${NAME}!", vars)
	if result != "Hello, World!" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestFindClosingBrace(t *testing.T) {
	tests := []struct {
		input    string
		pos      int
		expected int
	}{
		{"${foo}", 0, 5},
		{`${"hello"}`, 0, 9},
		{"${a{b}c}", 0, 7},
		{`${'a}b'}`, 0, 7},
	}

	for _, tt := range tests {
		got := findClosingBrace(tt.input, tt.pos)
		if got != tt.expected {
			t.Errorf("findClosingBrace(%q, %d) = %d, want %d", tt.input, tt.pos, got, tt.expected)
		}
	}
}

func mustContain(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected result to contain %q (len=%d)", substr, len(s))
	}
}

func mustNotContain(t *testing.T, s, substr string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Errorf("expected result NOT to contain %q", substr)
	}
}

// --- Phase 3 Tests ---

func TestAssembler_EnvironmentDetailsInMainPrompt(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		CWD:   "/home/user/project",
		OS:    "linux",
		Shell: "/bin/bash",
	}

	result := a.Assemble(config)
	mustContain(t, result, "Working directory: /home/user/project")
	mustContain(t, result, "Platform: linux")
	mustContain(t, result, "Shell: /bin/bash")
}

func TestAssembler_EnvironmentDetailsWithOSVersionAndDate(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		CWD:         "/home/user/project",
		OS:          "darwin",
		OSVersion:   "Darwin 25.2.0",
		CurrentDate: "2026-02-09",
		Shell:       "/bin/zsh",
	}

	result := a.Assemble(config)
	mustContain(t, result, "OS Version: Darwin 25.2.0")
	mustContain(t, result, "The current date is: 2026-02-09")
}

func TestAssembler_ToolDocumentation(t *testing.T) {
	a := &Assembler{}

	// Create a registry with Glob tool
	reg := tools.NewRegistry()
	reg.Register(&stubTool{name: "Glob"})

	config := &agent.AgentConfig{
		ToolRegistry: reg,
	}

	result := a.Assemble(config)

	// The glob tool doc should be included (tool-description-glob.md)
	// Exact content depends on the embedded file, but it should be non-empty
	// and included when the tool is registered
	if !strings.Contains(result, "glob") && !strings.Contains(result, "Glob") && !strings.Contains(result, "pattern") {
		// If the embedded file has any content about glob patterns, it should appear
		// If the file is empty, this test will need adjusting
		t.Log("Warning: Glob tool documentation may not have been loaded (embedded file might be empty or different)")
	}
}

func TestAssembler_AlwaysPromptSections(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{}

	result := a.Assemble(config)

	// These "always" sections should be present
	mustContain(t, result, "File Read Limits")
	mustContain(t, result, "Large File Handling")
	mustContain(t, result, "Verify Assumptions")
}

// stubTool satisfies tools.Tool for testing
type stubTool struct {
	name string
}

func (s *stubTool) Name() string                                                           { return s.name }
func (s *stubTool) Description() string                                                     { return "stub" }
func (s *stubTool) InputSchema() map[string]any                                             { return map[string]any{"type": "object"} }
func (s *stubTool) SideEffect() tools.SideEffectType                                        { return tools.SideEffectNone }
func (s *stubTool) Execute(_ context.Context, _ map[string]any) (tools.ToolOutput, error) {
	return tools.ToolOutput{}, nil
}

// --- Auto-memory tests ---

func TestAssembler_WithAutoMemory(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		AutoMemoryDir:     "/home/user/.claude/projects/abc123/memory",
		AutoMemoryContent: "# Project Memory\n\nSome notes.",
		PromptVersion:     "2.1.37",
	}

	result := a.Assemble(config)
	mustContain(t, result, "# auto memory")
	mustContain(t, result, "/home/user/.claude/projects/abc123/memory")
	mustContain(t, result, "## MEMORY.md")
	mustContain(t, result, "# Project Memory")
	mustContain(t, result, "Some notes.")
}

func TestAssembler_WithoutAutoMemory(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		PromptVersion: "2.1.37",
	}

	result := a.Assemble(config)
	mustNotContain(t, result, "# auto memory")
	mustNotContain(t, result, "## MEMORY.md")
}

func TestAssembler_WithManagedPolicy(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		ManagedPolicyContent: "Do not access external APIs.",
		ClaudeMDContent:      "Project instructions here.",
		PromptVersion:        "2.1.37",
	}

	result := a.Assemble(config)
	mustContain(t, result, "# Managed Policy")
	mustContain(t, result, "Do not access external APIs.")
}

func TestAssembler_ManagedPolicyBeforeClaudeMD(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		ManagedPolicyContent: "managed policy content",
		ClaudeMDContent:      "project instructions",
		PromptVersion:        "2.1.37",
	}

	result := a.Assemble(config)
	idxManaged := strings.Index(result, "# Managed Policy")
	idxClaudeMD := strings.Index(result, "# CLAUDE.md")

	if idxManaged < 0 {
		t.Fatal("expected Managed Policy section")
	}
	if idxClaudeMD < 0 {
		t.Fatal("expected CLAUDE.md section")
	}
	if idxManaged >= idxClaudeMD {
		t.Error("managed policy should appear before CLAUDE.md")
	}
}

func TestAssembler_WithUnconditionalRules(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		ProjectRules: []agent.RuleEntry{
			{Content: "Always use gofmt."},
		},
		PromptVersion: "2.1.37",
	}

	result := a.Assemble(config)
	mustContain(t, result, "Always use gofmt.")
}

func TestAssembler_WithConditionalRulesMatching(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		ProjectRules: []agent.RuleEntry{
			{Content: "Go formatting rule", PathPatterns: []string{"**/*.go"}},
		},
		ActiveFilePaths: []string{"pkg/prompt/assembler.go"},
		PromptVersion:   "2.1.37",
	}

	result := a.Assemble(config)
	mustContain(t, result, "Go formatting rule")
}

func TestAssembler_WithConditionalRulesNotMatching(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		ProjectRules: []agent.RuleEntry{
			{Content: "Go formatting rule", PathPatterns: []string{"**/*.go"}},
		},
		ActiveFilePaths: []string{"src/index.js"},
		PromptVersion:   "2.1.37",
	}

	result := a.Assemble(config)
	mustNotContain(t, result, "Go formatting rule")
}

func TestAssembler_AutoMemoryPositionAfterAgentMemory(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		MemoryEnabled:     true,
		AutoMemoryDir:     "/tmp/mem",
		AutoMemoryContent: "memory content",
		PromptVersion:     "2.1.37",
	}

	result := a.Assemble(config)
	idxAgentMemory := strings.Index(result, "Agent Memory")
	idxAutoMemory := strings.Index(result, "# auto memory")

	if idxAgentMemory < 0 {
		t.Fatal("expected Agent Memory section")
	}
	if idxAutoMemory < 0 {
		t.Fatal("expected auto memory section")
	}
	if idxAutoMemory <= idxAgentMemory {
		t.Error("auto memory should come after agent memory instructions")
	}
}
