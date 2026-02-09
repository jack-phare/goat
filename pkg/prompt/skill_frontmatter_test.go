package prompt

import (
	"testing"

	"github.com/jg-phare/goat/pkg/types"
)

func TestParseSkillContent_FullFields(t *testing.T) {
	data := []byte(`---
name: deploy
description: Deploy the application to production
allowed-tools:
  - Bash
  - Read
when_to_use: Use when the user asks to deploy, push to prod, or release
argument-hint: "[environment] [--dry-run]"
arguments:
  - environment
  - flags
context: fork
---
# Deploy Skill

Deploy the application to $environment with $flags.
`)

	entry, err := ParseSkillContent(data, "/test/.claude/skills/deploy/SKILL.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if entry.Name != "deploy" {
		t.Errorf("Name = %q, want %q", entry.Name, "deploy")
	}
	if entry.Description != "Deploy the application to production" {
		t.Errorf("Description = %q, want %q", entry.Description, "Deploy the application to production")
	}
	if len(entry.AllowedTools) != 2 || entry.AllowedTools[0] != "Bash" || entry.AllowedTools[1] != "Read" {
		t.Errorf("AllowedTools = %v, want [Bash Read]", entry.AllowedTools)
	}
	if entry.WhenToUse != "Use when the user asks to deploy, push to prod, or release" {
		t.Errorf("WhenToUse = %q", entry.WhenToUse)
	}
	if entry.ArgumentHint != "[environment] [--dry-run]" {
		t.Errorf("ArgumentHint = %q", entry.ArgumentHint)
	}
	if len(entry.Arguments) != 2 || entry.Arguments[0] != "environment" || entry.Arguments[1] != "flags" {
		t.Errorf("Arguments = %v, want [environment flags]", entry.Arguments)
	}
	if entry.Context != "fork" {
		t.Errorf("Context = %q, want %q", entry.Context, "fork")
	}
	if entry.Body != "# Deploy Skill\n\nDeploy the application to $environment with $flags." {
		t.Errorf("Body = %q", entry.Body)
	}
}

func TestParseSkillContent_Minimal(t *testing.T) {
	data := []byte(`---
name: test-skill
description: A minimal test skill
---
Just do the thing.
`)

	entry, err := ParseSkillContent(data, "/test/SKILL.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if entry.Name != "test-skill" {
		t.Errorf("Name = %q, want %q", entry.Name, "test-skill")
	}
	if entry.Description != "A minimal test skill" {
		t.Errorf("Description = %q", entry.Description)
	}
	if entry.Body != "Just do the thing." {
		t.Errorf("Body = %q", entry.Body)
	}
	if entry.Context != "" {
		t.Errorf("Context = %q, want empty", entry.Context)
	}
}

func TestParseSkillContent_NoFrontmatter(t *testing.T) {
	data := []byte("Just a body with no frontmatter.\n")

	_, err := ParseSkillContent(data, "/test/SKILL.md")
	if err == nil {
		t.Fatal("expected error for no frontmatter")
	}
	if got := err.Error(); got != "no frontmatter found in /test/SKILL.md" {
		t.Errorf("error = %q", got)
	}
}

func TestParseSkillContent_MissingDescription(t *testing.T) {
	data := []byte(`---
name: broken-skill
---
Body text.
`)

	_, err := ParseSkillContent(data, "/test/SKILL.md")
	if err == nil {
		t.Fatal("expected error for missing description")
	}
	if got := err.Error(); got != "missing required field 'description' in /test/SKILL.md" {
		t.Errorf("error = %q", got)
	}
}

func TestParseSkillContent_NameDerivedFromDir(t *testing.T) {
	data := []byte(`---
description: A skill without explicit name
---
Body text.
`)

	entry, err := ParseSkillContent(data, "/project/.claude/skills/my-cool-skill/SKILL.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if entry.Name != "my-cool-skill" {
		t.Errorf("Name = %q, want %q", entry.Name, "my-cool-skill")
	}
}

func TestParseSkillContent_InvalidContext(t *testing.T) {
	data := []byte(`---
name: bad-context
description: A skill with invalid context
context: hybrid
---
Body text.
`)

	_, err := ParseSkillContent(data, "/test/SKILL.md")
	if err == nil {
		t.Fatal("expected error for invalid context")
	}
}

func TestParseSkillContent_EmptyArgument(t *testing.T) {
	data := []byte(`---
name: empty-arg
description: A skill with empty argument
arguments:
  - valid_arg
  - ""
---
Body text.
`)

	_, err := ParseSkillContent(data, "/test/SKILL.md")
	if err == nil {
		t.Fatal("expected error for empty argument")
	}
}

func TestParseSkillContent_BodyExtraction(t *testing.T) {
	data := []byte(`---
name: body-test
description: Testing body extraction
---
Line one.

Line two.

Line three.
`)

	entry, err := ParseSkillContent(data, "/test/SKILL.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "Line one.\n\nLine two.\n\nLine three."
	if entry.Body != expected {
		t.Errorf("Body = %q, want %q", entry.Body, expected)
	}
}

func TestParseSkillContent_InvalidYAML(t *testing.T) {
	data := []byte(`---
name: [invalid
description: broken yaml
---
Body.
`)

	_, err := ParseSkillContent(data, "/test/SKILL.md")
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestValidateSkill_NoWarnings(t *testing.T) {
	entry := types.SkillEntry{
		SkillDefinition: types.SkillDefinition{
			Name:        "good-skill",
			Description: "Does things",
			WhenToUse:   "When you need it",
		},
	}

	warnings := ValidateSkill(entry)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
}

func TestValidateSkill_MissingWhenToUse(t *testing.T) {
	entry := types.SkillEntry{
		SkillDefinition: types.SkillDefinition{
			Name:        "partial-skill",
			Description: "Does things",
		},
	}

	warnings := ValidateSkill(entry)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	if warnings[0].Field != "when_to_use" {
		t.Errorf("warning field = %q, want %q", warnings[0].Field, "when_to_use")
	}
}

func TestValidateSkill_MissingName(t *testing.T) {
	entry := types.SkillEntry{
		SkillDefinition: types.SkillDefinition{
			Description: "Does things",
			WhenToUse:   "When needed",
		},
	}

	warnings := ValidateSkill(entry)
	found := false
	for _, w := range warnings {
		if w.Field == "name" {
			found = true
		}
	}
	if !found {
		t.Error("expected warning for missing name")
	}
}

func TestDeriveSkillName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/project/.claude/skills/my-skill/SKILL.md", "my-skill"},
		{"/home/user/.claude/skills/deploy/SKILL.md", "deploy"},
		{"/foo/SKILL.md", "foo"},
	}

	for _, tt := range tests {
		got := deriveSkillName(tt.path)
		if got != tt.want {
			t.Errorf("deriveSkillName(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
