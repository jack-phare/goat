package prompt

import (
	"strings"
	"testing"

	"github.com/jg-phare/goat/pkg/agent"
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
	mustContain(t, result, "Doing tasks")
	mustContain(t, result, "Executing actions with care")
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

	// Verify ordering: main prompt before doing tasks before CLAUDE.md before output style before append
	idxMain := strings.Index(result, "interactive CLI tool")
	idxDoing := strings.Index(result, "Doing tasks")
	idxCare := strings.Index(result, "Executing actions with care")
	idxClaudeMD := strings.Index(result, "CLAUDE.md")
	idxStyle := strings.Index(result, "# Output Style")
	idxAppend := strings.LastIndex(result, "FINAL")

	if idxMain >= idxDoing {
		t.Error("main prompt should come before doing tasks")
	}
	if idxDoing >= idxCare {
		t.Error("doing tasks should come before executing actions with care")
	}
	if idxCare >= idxClaudeMD {
		t.Error("executing actions should come before CLAUDE.md")
	}
	if idxClaudeMD >= idxStyle {
		t.Error("CLAUDE.md should come before output style")
	}
	if idxStyle >= idxAppend {
		t.Error("output style should come before append")
	}
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
