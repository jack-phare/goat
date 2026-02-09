package subagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeAgentFile(t *testing.T, dir, name, content string) {
	t.Helper()
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
}

func TestLoader_LoadAll_ProjectDir(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".claude", "agents")
	writeAgentFile(t, agentDir, "my-agent.md", `---
description: My project agent
tools: Read
---
Project agent prompt.
`)

	loader := NewLoader(dir, "")
	defs, _, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(defs) != 1 {
		t.Fatalf("expected 1 def, got %d", len(defs))
	}
	def, ok := defs["my-agent"]
	if !ok {
		t.Fatal("expected 'my-agent' definition")
	}
	if def.Source != SourceProject {
		t.Errorf("Source = %v, want SourceProject", def.Source)
	}
	if def.Priority != 30 {
		t.Errorf("Priority = %d, want 30", def.Priority)
	}
}

func TestLoader_LoadAll_UserDir(t *testing.T) {
	dir := t.TempDir()
	userDir := filepath.Join(dir, "user-agents")
	writeAgentFile(t, userDir, "user-agent.md", `---
description: User agent
---
User agent prompt.
`)

	loader := NewLoader(dir, userDir)
	defs, _, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	def, ok := defs["user-agent"]
	if !ok {
		t.Fatal("expected 'user-agent' definition")
	}
	if def.Source != SourceUser {
		t.Errorf("Source = %v, want SourceUser", def.Source)
	}
}

func TestLoader_LoadAll_PriorityOverride(t *testing.T) {
	dir := t.TempDir()

	// User agent
	userDir := filepath.Join(dir, "user-agents")
	writeAgentFile(t, userDir, "shared.md", `---
description: User version
---
User prompt.
`)

	// Project agent with same name
	projectDir := filepath.Join(dir, ".claude", "agents")
	writeAgentFile(t, projectDir, "shared.md", `---
description: Project version
---
Project prompt.
`)

	loader := NewLoader(dir, userDir)
	defs, _, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	def, ok := defs["shared"]
	if !ok {
		t.Fatal("expected 'shared' definition")
	}
	// Project should win (priority 30 > 20)
	if def.Description != "Project version" {
		t.Errorf("Description = %q, want 'Project version' (project should override user)", def.Description)
	}
}

func TestLoader_LoadAll_MissingDirs(t *testing.T) {
	loader := NewLoader("/nonexistent/path", "/also/nonexistent")
	defs, _, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(defs) != 0 {
		t.Errorf("expected 0 defs from nonexistent dirs, got %d", len(defs))
	}
}

func TestLoader_LoadAll_SkipsNonMD(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".claude", "agents")
	writeAgentFile(t, agentDir, "valid.md", `---
description: Valid agent
---
Prompt.
`)
	// Non-.md files should be skipped
	os.WriteFile(filepath.Join(agentDir, "notes.txt"), []byte("not an agent"), 0o644)
	os.WriteFile(filepath.Join(agentDir, "config.yaml"), []byte("name: test"), 0o644)

	loader := NewLoader(dir, "")
	defs, _, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(defs) != 1 {
		t.Errorf("expected 1 def (only .md), got %d", len(defs))
	}
}

func TestLoader_LoadAll_WarnsOnInvalidFiles(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".claude", "agents")
	writeAgentFile(t, agentDir, "valid.md", `---
description: Valid agent
---
Prompt.
`)
	// Invalid frontmatter — should produce a warning, not a hard error
	writeAgentFile(t, agentDir, "invalid.md", "No frontmatter here")

	loader := NewLoader(dir, "")
	defs, warnings, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if len(defs) != 1 {
		t.Errorf("expected 1 valid def, got %d", len(defs))
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	if warnings[0].File == "" {
		t.Error("warning should include filename")
	}
}

func TestLoader_SkipsMalformedFiles(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".claude", "agents")

	// 3 files, 1 bad
	writeAgentFile(t, agentDir, "good1.md", "---\ndescription: Good 1\n---\nPrompt.")
	writeAgentFile(t, agentDir, "bad.md", "No frontmatter here")
	writeAgentFile(t, agentDir, "good2.md", "---\ndescription: Good 2\n---\nPrompt.")

	loader := NewLoader(dir, "")
	defs, warnings, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(defs) != 2 {
		t.Errorf("expected 2 defs, got %d", len(defs))
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
}

func TestLoader_WarningForMalformedFile(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".claude", "agents")
	writeAgentFile(t, agentDir, "broken.md", "No frontmatter")

	loader := NewLoader(dir, "")
	_, warnings, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	w := warnings[0]
	if !strings.Contains(w.File, "broken.md") {
		t.Errorf("warning file = %q, want to contain 'broken.md'", w.File)
	}
	if w.Error == nil {
		t.Error("warning error should not be nil")
	}
}

func TestLoader_LoadAll_UnknownFieldWarnings(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".claude", "agents")
	writeAgentFile(t, agentDir, "typo-agent.md", `---
description: Agent with typo
tols: Read
---
Prompt.
`)

	loader := NewLoader(dir, "")
	defs, warnings, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// File should still load successfully (typo is a warning, not an error)
	if _, ok := defs["typo-agent"]; !ok {
		t.Error("expected 'typo-agent' to be loaded despite unknown field warning")
	}
	// Should have a warning about the unknown field
	if len(warnings) == 0 {
		t.Fatal("expected at least 1 warning for unknown field 'tols'")
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w.Error.Error(), "tols") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about 'tols', got: %v", warnings)
	}
}

func TestLoader_LoadAll_PluginDirs(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "plugins")
	writeAgentFile(t, pluginDir, "plugin-agent.md", `---
description: Plugin agent
---
Plugin prompt.
`)

	loader := NewLoader(dir, "", pluginDir)
	defs, _, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	def, ok := defs["plugin-agent"]
	if !ok {
		t.Fatal("expected 'plugin-agent' definition")
	}
	if def.Source != SourcePlugin {
		t.Errorf("Source = %v, want SourcePlugin", def.Source)
	}
	if def.Priority != 10 {
		t.Errorf("Priority = %d, want 10", def.Priority)
	}
}
