package subagent

import (
	"os"
	"path/filepath"
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
	defs, err := loader.LoadAll()
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
	defs, err := loader.LoadAll()
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
	defs, err := loader.LoadAll()
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
	defs, err := loader.LoadAll()
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
	defs, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(defs) != 1 {
		t.Errorf("expected 1 def (only .md), got %d", len(defs))
	}
}

func TestLoader_LoadAll_ErrorsOnInvalidFiles(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".claude", "agents")
	writeAgentFile(t, agentDir, "valid.md", `---
description: Valid agent
---
Prompt.
`)
	// Invalid frontmatter — should cause an error
	writeAgentFile(t, agentDir, "invalid.md", "No frontmatter here")

	loader := NewLoader(dir, "")
	_, err := loader.LoadAll()
	if err == nil {
		t.Fatal("expected error for invalid agent file, got nil")
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
	defs, err := loader.LoadAll()
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
