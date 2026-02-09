package prompt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jg-phare/goat/pkg/types"
)

// createTestSkill creates a SKILL.md file in dir/{name}/SKILL.md.
func createTestSkill(t *testing.T, dir, name, content string) {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", skillDir, err)
	}
	path := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestSkillLoader_ProjectDir(t *testing.T) {
	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, ".claude", "skills")
	createTestSkill(t, projectDir, "test-skill", `---
description: A project skill
when_to_use: When testing
---
Do the test.
`)

	loader := NewSkillLoader(tmp, "")
	skills, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}

	skill, ok := skills["test-skill"]
	if !ok {
		t.Fatal("expected skill 'test-skill'")
	}
	if skill.Description != "A project skill" {
		t.Errorf("Description = %q", skill.Description)
	}
	if skill.Source != types.SkillSourceProject {
		t.Errorf("Source = %v, want Project", skill.Source)
	}
	if skill.Priority != 30 {
		t.Errorf("Priority = %d, want 30", skill.Priority)
	}
}

func TestSkillLoader_UserDir(t *testing.T) {
	tmp := t.TempDir()
	userDir := filepath.Join(tmp, "user-claude")
	skillsDir := filepath.Join(userDir, "skills")
	createTestSkill(t, skillsDir, "user-skill", `---
description: A user skill
---
User body.
`)

	loader := NewSkillLoader("", userDir)
	skills, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	skill, ok := skills["user-skill"]
	if !ok {
		t.Fatal("expected skill 'user-skill'")
	}
	if skill.Source != types.SkillSourceUser {
		t.Errorf("Source = %v, want User", skill.Source)
	}
	if skill.Priority != 20 {
		t.Errorf("Priority = %d, want 20", skill.Priority)
	}
}

func TestSkillLoader_ProjectOverridesUser(t *testing.T) {
	tmp := t.TempDir()

	userDir := filepath.Join(tmp, "user-claude")
	userSkillsDir := filepath.Join(userDir, "skills")
	createTestSkill(t, userSkillsDir, "shared-skill", `---
description: User version
---
User body.
`)

	projectDir := filepath.Join(tmp, "project")
	projectSkillsDir := filepath.Join(projectDir, ".claude", "skills")
	createTestSkill(t, projectSkillsDir, "shared-skill", `---
description: Project version
---
Project body.
`)

	loader := NewSkillLoader(projectDir, userDir)
	skills, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	skill, ok := skills["shared-skill"]
	if !ok {
		t.Fatal("expected skill 'shared-skill'")
	}
	if skill.Description != "Project version" {
		t.Errorf("Description = %q, want %q", skill.Description, "Project version")
	}
	if skill.Source != types.SkillSourceProject {
		t.Errorf("Source = %v, want Project", skill.Source)
	}
}

func TestSkillLoader_MissingDirectorySilentlySkipped(t *testing.T) {
	loader := NewSkillLoader("/nonexistent/path", "/another/nonexistent")
	skills, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
}

func TestSkillLoader_MalformedFrontmatter(t *testing.T) {
	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, ".claude", "skills")
	createTestSkill(t, projectDir, "bad-skill", `---
name: [invalid
description: broken
---
Body.
`)

	loader := NewSkillLoader(tmp, "")
	_, err := loader.LoadAll()
	if err == nil {
		t.Fatal("expected error for malformed frontmatter")
	}
}

func TestSkillLoader_NameDerivedFromDirectory(t *testing.T) {
	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, ".claude", "skills")
	createTestSkill(t, projectDir, "auto-named-skill", `---
description: Skill with name derived from dir
---
Body.
`)

	loader := NewSkillLoader(tmp, "")
	skills, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	if _, ok := skills["auto-named-skill"]; !ok {
		t.Errorf("expected skill with derived name 'auto-named-skill', got keys %v", skillNames(skills))
	}
}

func TestSkillLoader_ValidateRequiredFields(t *testing.T) {
	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, ".claude", "skills")
	createTestSkill(t, projectDir, "no-desc", `---
name: no-desc
---
Body.
`)

	loader := NewSkillLoader(tmp, "")
	_, err := loader.LoadAll()
	if err == nil {
		t.Fatal("expected error for missing description")
	}
}

func TestSkillLoader_PluginDirs(t *testing.T) {
	tmp := t.TempDir()
	pluginDir := filepath.Join(tmp, "plugins")
	createTestSkill(t, pluginDir, "plugin-skill", `---
description: A plugin-provided skill
---
Plugin body.
`)

	loader := NewSkillLoader("", "", pluginDir)
	skills, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	skill, ok := skills["plugin-skill"]
	if !ok {
		t.Fatal("expected skill 'plugin-skill'")
	}
	if skill.Source != types.SkillSourcePlugin {
		t.Errorf("Source = %v, want Plugin", skill.Source)
	}
	if skill.Priority != 10 {
		t.Errorf("Priority = %d, want 10", skill.Priority)
	}
}

func TestSkillLoader_PluginOverriddenByProject(t *testing.T) {
	tmp := t.TempDir()

	pluginDir := filepath.Join(tmp, "plugins")
	createTestSkill(t, pluginDir, "shared", `---
description: Plugin version
---
Plugin body.
`)

	projectDir := filepath.Join(tmp, "project")
	projectSkillsDir := filepath.Join(projectDir, ".claude", "skills")
	createTestSkill(t, projectSkillsDir, "shared", `---
description: Project version
---
Project body.
`)

	loader := NewSkillLoader(projectDir, "", pluginDir)
	skills, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	skill := skills["shared"]
	if skill.Description != "Project version" {
		t.Errorf("Description = %q, want %q", skill.Description, "Project version")
	}
}

func TestSkillLoader_SkipNonDirEntries(t *testing.T) {
	tmp := t.TempDir()
	skillsDir := filepath.Join(tmp, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Create a file (not a directory) in the skills directory
	if err := os.WriteFile(filepath.Join(skillsDir, "not-a-dir.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	loader := NewSkillLoader(tmp, "")
	skills, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
}

// skillNames returns the keys of a skill map for debug output.
func skillNames(m map[string]types.SkillEntry) []string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	return names
}
