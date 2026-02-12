package prompt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRules_BasicDirectory(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, ".claude", "rules")
	os.MkdirAll(rulesDir, 0o755)

	writeFile(t, filepath.Join(rulesDir, "style.md"), "Use consistent formatting.")
	writeFile(t, filepath.Join(rulesDir, "testing.md"), "Always write tests.")

	rules, err := LoadRules(rulesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
}

func TestLoadRules_RecursiveSubdirs(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, ".claude", "rules")
	subDir := filepath.Join(rulesDir, "go")
	os.MkdirAll(subDir, 0o755)

	writeFile(t, filepath.Join(rulesDir, "general.md"), "Be concise.")
	writeFile(t, filepath.Join(subDir, "formatting.md"), "Use gofmt.")

	rules, err := LoadRules(rulesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules from subdirs, got %d", len(rules))
	}
}

func TestLoadRules_NonExistentDir(t *testing.T) {
	rules, err := LoadRules("/nonexistent/path/rules")
	if err != nil {
		t.Fatalf("expected no error for non-existent dir, got: %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(rules))
	}
}

func TestLoadRules_SkipsNonMdFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "rule.md"), "A rule.")
	writeFile(t, filepath.Join(dir, "notes.txt"), "Not a rule.")
	writeFile(t, filepath.Join(dir, "config.yaml"), "not: a rule")

	rules, err := LoadRules(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule (only .md), got %d", len(rules))
	}
}

func TestParseFrontmatter_WithPaths(t *testing.T) {
	content := `---
paths:
  - "**/*.go"
  - "pkg/**"
---
Use gofmt for all Go files.`

	rule, err := ParseRuleFrontmatter(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rule.PathPatterns) != 2 {
		t.Fatalf("expected 2 path patterns, got %d", len(rule.PathPatterns))
	}
	if rule.PathPatterns[0] != "**/*.go" {
		t.Errorf("pattern[0] = %q, want %q", rule.PathPatterns[0], "**/*.go")
	}
	if rule.PathPatterns[1] != "pkg/**" {
		t.Errorf("pattern[1] = %q, want %q", rule.PathPatterns[1], "pkg/**")
	}
	if rule.Content != "Use gofmt for all Go files." {
		t.Errorf("content = %q", rule.Content)
	}
	if !rule.IsConditional() {
		t.Error("rule with paths should be conditional")
	}
}

func TestParseFrontmatter_NoPaths(t *testing.T) {
	content := "Always be polite."

	rule, err := ParseRuleFrontmatter(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rule.PathPatterns) != 0 {
		t.Errorf("expected 0 path patterns, got %d", len(rule.PathPatterns))
	}
	if rule.Content != "Always be polite." {
		t.Errorf("content = %q", rule.Content)
	}
	if rule.IsConditional() {
		t.Error("rule without paths should not be conditional")
	}
}

func TestParseFrontmatter_EmptyFrontmatter(t *testing.T) {
	content := `---
---
Body content.`

	rule, err := ParseRuleFrontmatter(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rule.Content != "Body content." {
		t.Errorf("content = %q", rule.Content)
	}
	if rule.IsConditional() {
		t.Error("empty frontmatter should not be conditional")
	}
}

func TestParseFrontmatter_NoClosingDelimiter(t *testing.T) {
	content := "---\npaths:\n  - '*.go'\nNo closing delimiter"

	rule, err := ParseRuleFrontmatter(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should treat entire content as body since no closing ---
	if rule.Content == "" {
		t.Error("expected non-empty content")
	}
}

func TestMatchRules_UnconditionalAlwaysIncluded(t *testing.T) {
	rules := []Rule{
		{Content: "Always included"},
		{Content: "Conditional", PathPatterns: []string{"**/*.go"}},
	}

	matched := MatchRules(rules, nil)
	if len(matched) != 1 {
		t.Fatalf("expected 1 matched rule (unconditional), got %d", len(matched))
	}
	if matched[0].Content != "Always included" {
		t.Errorf("matched wrong rule: %q", matched[0].Content)
	}
}

func TestMatchRules_ConditionalMatchesFiles(t *testing.T) {
	rules := []Rule{
		{Content: "Go rule", PathPatterns: []string{"**/*.go"}},
		{Content: "JS rule", PathPatterns: []string{"**/*.js"}},
	}

	matched := MatchRules(rules, []string{"pkg/foo/bar.go"})
	if len(matched) != 1 {
		t.Fatalf("expected 1 matched rule, got %d", len(matched))
	}
	if matched[0].Content != "Go rule" {
		t.Errorf("expected Go rule, got %q", matched[0].Content)
	}
}

func TestMatchRules_ConditionalNoMatch(t *testing.T) {
	rules := []Rule{
		{Content: "Go rule", PathPatterns: []string{"**/*.go"}},
	}

	matched := MatchRules(rules, []string{"src/index.js"})
	if len(matched) != 0 {
		t.Errorf("expected 0 matched rules, got %d", len(matched))
	}
}

func TestMatchRules_MultiplePatterns(t *testing.T) {
	rules := []Rule{
		{Content: "Backend rule", PathPatterns: []string{"pkg/**", "internal/**"}},
	}

	// Should match on second pattern
	matched := MatchRules(rules, []string{"internal/handler.go"})
	if len(matched) != 1 {
		t.Fatalf("expected 1 matched rule, got %d", len(matched))
	}
}

func TestMatchRules_EmptyFilePaths(t *testing.T) {
	rules := []Rule{
		{Content: "Always"},
		{Content: "Conditional", PathPatterns: []string{"**/*.go"}},
	}

	matched := MatchRules(rules, []string{})
	if len(matched) != 1 {
		t.Fatalf("expected 1 matched rule (unconditional only), got %d", len(matched))
	}
}
