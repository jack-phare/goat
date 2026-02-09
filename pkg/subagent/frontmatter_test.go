package subagent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jg-phare/goat/pkg/types"
)

func TestSplitFrontmatter_Valid(t *testing.T) {
	input := []byte("---\nname: test\n---\nBody content here")
	yamlPart, body, err := splitFrontmatter(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(yamlPart) != "name: test" {
		t.Errorf("yaml = %q, want 'name: test'", string(yamlPart))
	}
	if body != "Body content here" {
		t.Errorf("body = %q, want 'Body content here'", body)
	}
}

func TestSplitFrontmatter_NoFrontmatter(t *testing.T) {
	input := []byte("Just a body with no frontmatter")
	yamlPart, body, err := splitFrontmatter(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if yamlPart != nil {
		t.Errorf("expected nil yaml, got %q", string(yamlPart))
	}
	if body != "Just a body with no frontmatter" {
		t.Errorf("body = %q", body)
	}
}

func TestSplitFrontmatter_NoClosingDelimiter(t *testing.T) {
	input := []byte("---\nname: test\nNo closing delimiter")
	yamlPart, body, err := splitFrontmatter(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if yamlPart != nil {
		t.Errorf("expected nil yaml when no closing delimiter, got %q", string(yamlPart))
	}
	if body != string(input) {
		t.Errorf("body should be entire content")
	}
}

func TestSplitFrontmatter_EmptyBody(t *testing.T) {
	input := []byte("---\nname: test\n---\n")
	yamlPart, body, err := splitFrontmatter(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(yamlPart) != "name: test" {
		t.Errorf("yaml = %q", string(yamlPart))
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
}

func TestParseContent_Valid(t *testing.T) {
	content := []byte(`---
name: my-agent
description: A test agent
model: haiku
tools: Read, Glob, Grep
---
You are a test agent.
`)
	def, err := ParseContent(content, "test.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def.Name != "my-agent" {
		t.Errorf("Name = %q, want 'my-agent'", def.Name)
	}
	if def.Description != "A test agent" {
		t.Errorf("Description = %q", def.Description)
	}
	if def.Model != "haiku" {
		t.Errorf("Model = %q", def.Model)
	}
	if len(def.Tools) != 3 {
		t.Fatalf("Tools count = %d, want 3", len(def.Tools))
	}
	if def.Tools[0] != "Read" || def.Tools[1] != "Glob" || def.Tools[2] != "Grep" {
		t.Errorf("Tools = %v", def.Tools)
	}
	if def.Prompt != "You are a test agent." {
		t.Errorf("Prompt = %q", def.Prompt)
	}
}

func TestParseContent_ToolsAsList(t *testing.T) {
	content := []byte(`---
name: list-agent
description: Agent with list tools
tools:
  - Read
  - Glob
  - Grep
---
Body.
`)
	def, err := ParseContent(content, "test.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(def.Tools) != 3 {
		t.Fatalf("Tools count = %d, want 3", len(def.Tools))
	}
	if def.Tools[0] != "Read" || def.Tools[1] != "Glob" || def.Tools[2] != "Grep" {
		t.Errorf("Tools = %v", def.Tools)
	}
}

func TestParseContent_MissingDescription(t *testing.T) {
	content := []byte(`---
name: no-desc
---
Body.
`)
	_, err := ParseContent(content, "test.md")
	if err == nil {
		t.Fatal("expected error for missing description")
	}
}

func TestParseContent_NoFrontmatter(t *testing.T) {
	content := []byte("Just a body")
	_, err := ParseContent(content, "test.md")
	if err == nil {
		t.Fatal("expected error for no frontmatter")
	}
}

func TestParseContent_DeriveNameFromFilePath(t *testing.T) {
	content := []byte(`---
description: Auto-named agent
---
Body.
`)
	def, err := ParseContent(content, "/path/to/my-cool-agent.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def.Name != "my-cool-agent" {
		t.Errorf("Name = %q, want 'my-cool-agent'", def.Name)
	}
}

func TestParseContent_AllFields(t *testing.T) {
	content := []byte(`---
name: full-agent
description: Full agent definition
model: opus
tools: Read, Glob
disallowedTools: Bash
maxTurns: 5
permissionMode: bypassPermissions
memory: auto
skills:
  - commit
criticalSystemReminder_EXPERIMENTAL: Be careful!
---
Full agent prompt.
`)
	def, err := ParseContent(content, "test.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def.Name != "full-agent" {
		t.Errorf("Name = %q", def.Name)
	}
	if def.Model != "opus" {
		t.Errorf("Model = %q", def.Model)
	}
	if len(def.DisallowedTools) != 1 || def.DisallowedTools[0] != "Bash" {
		t.Errorf("DisallowedTools = %v", def.DisallowedTools)
	}
	if def.MaxTurns == nil || *def.MaxTurns != 5 {
		t.Error("MaxTurns should be 5")
	}
	if def.PermissionMode != "bypassPermissions" {
		t.Errorf("PermissionMode = %q", def.PermissionMode)
	}
	if def.Memory != "auto" {
		t.Errorf("Memory = %q", def.Memory)
	}
	if len(def.Skills) != 1 || def.Skills[0] != "commit" {
		t.Errorf("Skills = %v", def.Skills)
	}
	if def.CriticalReminder != "Be careful!" {
		t.Errorf("CriticalReminder = %q", def.CriticalReminder)
	}
	if def.Prompt != "Full agent prompt." {
		t.Errorf("Prompt = %q", def.Prompt)
	}
}

func TestParseContent_InvalidYAML(t *testing.T) {
	content := []byte("---\n: invalid: yaml: [broken\n---\nBody.")
	_, err := ParseContent(content, "test.md")
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestParseFile_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-agent.md")
	content := `---
description: Test agent from file
tools: Read
---
File-based prompt.
`
	os.WriteFile(path, []byte(content), 0o644)

	def, err := ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def.Name != "test-agent" {
		t.Errorf("Name = %q, want 'test-agent'", def.Name)
	}
	if def.Description != "Test agent from file" {
		t.Errorf("Description = %q", def.Description)
	}
	if def.FilePath != path {
		t.Errorf("FilePath = %q", def.FilePath)
	}
}

func TestParseFile_NotFound(t *testing.T) {
	_, err := ParseFile("/nonexistent/path/agent.md")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestFlexStringList_EmptyString(t *testing.T) {
	content := []byte(`---
description: Empty tools
tools: ""
---
Body.
`)
	def, err := ParseContent(content, "test.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(def.Tools) != 0 {
		t.Errorf("Tools = %v, want empty", def.Tools)
	}
}

func TestFromTypesDefinition(t *testing.T) {
	ad := types.AgentDefinition{
		Description: "Test agent",
		Prompt:      "You are a test agent.",
		Model:       "sonnet",
	}
	def := FromTypesDefinition("test-agent", ad, SourceBuiltIn, 0)
	if def.Name != "test-agent" {
		t.Errorf("Name = %q", def.Name)
	}
	if def.Source != SourceBuiltIn {
		t.Errorf("Source = %v", def.Source)
	}
	if def.Priority != 0 {
		t.Errorf("Priority = %d", def.Priority)
	}

	// If AgentDefinition already has a Name, it should be preserved
	ad.Name = "original"
	def2 := FromTypesDefinition("override", ad, SourceCLIFlag, 1)
	if def2.Name != "original" {
		t.Errorf("Name = %q, want 'original' (should preserve existing)", def2.Name)
	}
}

// --- Phase 2 Tests: Frontmatter Validation ---

func TestFrontmatter_InvalidPermissionMode(t *testing.T) {
	content := []byte(`---
description: Test
permissionMode: foo
---
Body.
`)
	_, err := ParseContent(content, "test.md")
	if err == nil {
		t.Fatal("expected error for invalid permissionMode")
	}
	if !strings.Contains(err.Error(), "invalid permissionMode") {
		t.Errorf("error = %q, want 'invalid permissionMode'", err.Error())
	}
}

func TestFrontmatter_ValidPermissionModes(t *testing.T) {
	modes := []string{"", "default", "acceptEdits", "bypassPermissions", "plan", "delegate", "dontAsk"}
	for _, mode := range modes {
		var yaml string
		if mode == "" {
			yaml = "description: Test"
		} else {
			yaml = fmt.Sprintf("description: Test\npermissionMode: %s", mode)
		}
		content := []byte("---\n" + yaml + "\n---\nBody.")
		_, err := ParseContent(content, "test.md")
		if err != nil {
			t.Errorf("mode %q: unexpected error: %v", mode, err)
		}
	}
}

func TestFrontmatter_MaxTurnsZero(t *testing.T) {
	content := []byte(`---
description: Test
maxTurns: 0
---
Body.
`)
	_, err := ParseContent(content, "test.md")
	if err == nil {
		t.Fatal("expected error for maxTurns: 0")
	}
	if !strings.Contains(err.Error(), "maxTurns must be positive") {
		t.Errorf("error = %q, want 'maxTurns must be positive'", err.Error())
	}
}

func TestFrontmatter_MaxTurnsNegative(t *testing.T) {
	content := []byte(`---
description: Test
maxTurns: -1
---
Body.
`)
	_, err := ParseContent(content, "test.md")
	if err == nil {
		t.Fatal("expected error for maxTurns: -1")
	}
	if !strings.Contains(err.Error(), "maxTurns must be positive") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestFrontmatter_ValidMaxTurns(t *testing.T) {
	content := []byte(`---
description: Test
maxTurns: 5
---
Body.
`)
	def, err := ParseContent(content, "test.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def.MaxTurns == nil || *def.MaxTurns != 5 {
		t.Error("expected maxTurns = 5")
	}
}

func TestFrontmatter_ModelValidation(t *testing.T) {
	// Valid: known aliases
	for _, model := range []string{"haiku", "sonnet", "opus"} {
		content := []byte(fmt.Sprintf("---\ndescription: Test\nmodel: %s\n---\nBody.", model))
		_, err := ParseContent(content, "test.md")
		if err != nil {
			t.Errorf("model %q: unexpected error: %v", model, err)
		}
	}

	// Valid: full model IDs (contain - or /)
	for _, model := range []string{"claude-sonnet-4-5-20250929", "anthropic/claude-3"} {
		content := []byte(fmt.Sprintf("---\ndescription: Test\nmodel: %s\n---\nBody.", model))
		_, err := ParseContent(content, "test.md")
		if err != nil {
			t.Errorf("model %q: unexpected error: %v", model, err)
		}
	}

	// Invalid: garbage string with no dash or slash
	content := []byte("---\ndescription: Test\nmodel: foobar\n---\nBody.")
	_, err := ParseContent(content, "test.md")
	if err == nil {
		t.Fatal("expected error for model: foobar")
	}
	if !strings.Contains(err.Error(), "invalid model") {
		t.Errorf("error = %q, want 'invalid model'", err.Error())
	}
}

// --- Phase 6 Tests: Unknown YAML Field Warnings ---

func TestFrontmatter_UnknownField_TypoSuggestion(t *testing.T) {
	content := []byte(`---
description: Test
tols: Read
---
Body.
`)
	_, warnings, err := ParseContentWithWarnings(content, "test.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings count = %d, want 1; warnings = %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], `unknown field "tols"`) {
		t.Errorf("warning = %q, want 'unknown field \"tols\"'", warnings[0])
	}
	if !strings.Contains(warnings[0], `did you mean "tools"`) {
		t.Errorf("warning = %q, want 'did you mean \"tools\"'", warnings[0])
	}
}

func TestFrontmatter_UnknownField_Multiple(t *testing.T) {
	content := []byte(`---
description: Test
foo: bar
baz: qux
---
Body.
`)
	_, warnings, err := ParseContentWithWarnings(content, "test.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 2 {
		t.Fatalf("warnings count = %d, want 2; warnings = %v", len(warnings), warnings)
	}
	// Check both unknown fields are reported (order may vary due to map iteration)
	combined := strings.Join(warnings, " | ")
	if !strings.Contains(combined, `"foo"`) || !strings.Contains(combined, `"baz"`) {
		t.Errorf("warnings = %v, want both 'foo' and 'baz'", warnings)
	}
}

func TestFrontmatter_AllKnownFields_NoWarnings(t *testing.T) {
	content := []byte(`---
name: full-agent
description: Full agent definition
model: opus
tools: Read, Glob
disallowedTools: Bash
maxTurns: 5
permissionMode: bypassPermissions
memory: auto
skills:
  - commit
criticalSystemReminder_EXPERIMENTAL: Be careful!
---
Full agent prompt.
`)
	_, warnings, err := ParseContentWithWarnings(content, "test.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings for known fields, got %v", warnings)
	}
}

func TestFrontmatter_UnknownField_NoSuggestion(t *testing.T) {
	content := []byte(`---
description: Test
zzzzRandomField: value
---
Body.
`)
	_, warnings, err := ParseContentWithWarnings(content, "test.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings count = %d, want 1", len(warnings))
	}
	if !strings.Contains(warnings[0], `unknown field "zzzzRandomField"`) {
		t.Errorf("warning = %q", warnings[0])
	}
	// Should NOT contain "did you mean" for an unrecognizable field
	if strings.Contains(warnings[0], "did you mean") {
		t.Errorf("should not suggest for unrecognizable field, got %q", warnings[0])
	}
}

func TestFrontmatter_ColorField_Known(t *testing.T) {
	content := []byte(`---
description: Test
color: blue
---
Body.
`)
	_, warnings, err := ParseContentWithWarnings(content, "test.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("'color' should be a known field, got warnings: %v", warnings)
	}
}

// --- Phase 7 Tests: Color Field ---

func TestFrontmatter_ColorField_Hex(t *testing.T) {
	content := []byte(`---
description: Test
color: "#FF5733"
---
Body.
`)
	def, err := ParseContent(content, "test.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def.Color != "#FF5733" {
		t.Errorf("Color = %q, want '#FF5733'", def.Color)
	}
}

func TestFrontmatter_ColorField_Named(t *testing.T) {
	content := []byte(`---
description: Test
color: blue
---
Body.
`)
	def, err := ParseContent(content, "test.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def.Color != "blue" {
		t.Errorf("Color = %q, want 'blue'", def.Color)
	}
}

func TestFrontmatter_ColorField_Empty(t *testing.T) {
	content := []byte(`---
description: Test
---
Body.
`)
	def, err := ParseContent(content, "test.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def.Color != "" {
		t.Errorf("Color = %q, want empty", def.Color)
	}
}

func TestAgentSource_String(t *testing.T) {
	tests := []struct {
		source AgentSource
		want   string
	}{
		{SourceBuiltIn, "built-in"},
		{SourceCLIFlag, "cli"},
		{SourceProject, "project"},
		{SourceUser, "user"},
		{SourcePlugin, "plugin"},
		{AgentSource(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.source.String(); got != tt.want {
			t.Errorf("%d.String() = %q, want %q", tt.source, got, tt.want)
		}
	}
}
