package prompt

import (
	"strings"
	"testing"

	"github.com/jg-phare/goat/pkg/types"
)

func TestSkillRegistry_RegisterAndGet(t *testing.T) {
	r := NewSkillRegistry()
	entry := types.SkillEntry{
		SkillDefinition: types.SkillDefinition{
			Name:        "test",
			Description: "A test skill",
		},
	}

	r.Register(entry)

	got, ok := r.Get("test")
	if !ok {
		t.Fatal("expected to find skill 'test'")
	}
	if got.Description != "A test skill" {
		t.Errorf("Description = %q", got.Description)
	}
}

func TestSkillRegistry_GetSkill(t *testing.T) {
	r := NewSkillRegistry()
	r.Register(types.SkillEntry{
		SkillDefinition: types.SkillDefinition{Name: "x", Description: "X"},
	})

	got, ok := r.GetSkill("x")
	if !ok {
		t.Fatal("expected to find skill")
	}
	if got.Name != "x" {
		t.Errorf("Name = %q", got.Name)
	}
}

func TestSkillRegistry_GetNotFound(t *testing.T) {
	r := NewSkillRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("expected not found")
	}
}

func TestSkillRegistry_Unregister(t *testing.T) {
	r := NewSkillRegistry()
	r.Register(types.SkillEntry{
		SkillDefinition: types.SkillDefinition{Name: "temp", Description: "Temporary"},
	})

	r.Unregister("temp")
	_, ok := r.Get("temp")
	if ok {
		t.Error("expected skill to be unregistered")
	}
}

func TestSkillRegistry_ListSorted(t *testing.T) {
	r := NewSkillRegistry()
	r.Register(types.SkillEntry{SkillDefinition: types.SkillDefinition{Name: "charlie", Description: "C"}})
	r.Register(types.SkillEntry{SkillDefinition: types.SkillDefinition{Name: "alpha", Description: "A"}})
	r.Register(types.SkillEntry{SkillDefinition: types.SkillDefinition{Name: "bravo", Description: "B"}})

	list := r.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 skills, got %d", len(list))
	}
	if list[0].Name != "alpha" || list[1].Name != "bravo" || list[2].Name != "charlie" {
		t.Errorf("list order: %s, %s, %s", list[0].Name, list[1].Name, list[2].Name)
	}
}

func TestSkillRegistry_Names(t *testing.T) {
	r := NewSkillRegistry()
	r.Register(types.SkillEntry{SkillDefinition: types.SkillDefinition{Name: "b", Description: "B"}})
	r.Register(types.SkillEntry{SkillDefinition: types.SkillDefinition{Name: "a", Description: "A"}})

	names := r.Names()
	if len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Errorf("Names = %v", names)
	}
}

func TestSkillRegistry_SlashCommands(t *testing.T) {
	r := NewSkillRegistry()
	r.Register(types.SkillEntry{SkillDefinition: types.SkillDefinition{Name: "commit", Description: "Git commit"}})
	r.Register(types.SkillEntry{SkillDefinition: types.SkillDefinition{Name: "deploy", Description: "Deploy"}})

	cmds := r.SlashCommands()
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}
}

func TestSkillRegistry_FormatSkillsList(t *testing.T) {
	r := NewSkillRegistry()
	r.Register(types.SkillEntry{
		SkillDefinition: types.SkillDefinition{
			Name:        "deploy",
			Description: "Deploy the app",
			WhenToUse:   "When user asks to deploy",
		},
	})
	r.Register(types.SkillEntry{
		SkillDefinition: types.SkillDefinition{
			Name:        "commit",
			Description: "Create a git commit",
		},
	})

	formatted := r.FormatSkillsList()

	if !strings.Contains(formatted, "- commit: Create a git commit") {
		t.Errorf("missing commit entry in:\n%s", formatted)
	}
	if !strings.Contains(formatted, "- deploy: Deploy the app. When user asks to deploy") {
		t.Errorf("missing deploy entry with when_to_use in:\n%s", formatted)
	}

	// Verify alphabetical order
	commitIdx := strings.Index(formatted, "commit")
	deployIdx := strings.Index(formatted, "deploy")
	if commitIdx > deployIdx {
		t.Error("expected alphabetical order (commit before deploy)")
	}
}

func TestSkillRegistry_FormatSkillsListEmpty(t *testing.T) {
	r := NewSkillRegistry()
	if got := r.FormatSkillsList(); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestResolveSkills_PriorityOverride(t *testing.T) {
	embedded := map[string]types.SkillEntry{
		"shared": {
			SkillDefinition: types.SkillDefinition{Name: "shared", Description: "Embedded"},
			Priority:        0,
		},
	}
	fileBased := map[string]types.SkillEntry{
		"shared": {
			SkillDefinition: types.SkillDefinition{Name: "shared", Description: "File"},
			Priority:        30,
		},
		"extra": {
			SkillDefinition: types.SkillDefinition{Name: "extra", Description: "Extra"},
			Priority:        30,
		},
	}

	result := ResolveSkills(embedded, fileBased)

	if len(result) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(result))
	}
	if result["shared"].Description != "File" {
		t.Errorf("shared Description = %q, want %q", result["shared"].Description, "File")
	}
}

func TestResolveSkills_LowerPriorityDoesNotOverwrite(t *testing.T) {
	high := map[string]types.SkillEntry{
		"x": {
			SkillDefinition: types.SkillDefinition{Name: "x", Description: "High"},
			Priority:        30,
		},
	}
	low := map[string]types.SkillEntry{
		"x": {
			SkillDefinition: types.SkillDefinition{Name: "x", Description: "Low"},
			Priority:        10,
		},
	}

	// High first, then low — low should NOT overwrite
	result := ResolveSkills(high, low)
	if result["x"].Description != "High" {
		t.Errorf("Description = %q, want %q", result["x"].Description, "High")
	}
}

func TestLoadEmbeddedSkills(t *testing.T) {
	skills := LoadEmbeddedSkills()
	if len(skills) != 4 {
		t.Fatalf("expected 4 embedded skills, got %d", len(skills))
	}

	for _, name := range []string{"debugging", "update-claude-code-config", "verification-specialist", "skillify"} {
		skill, ok := skills[name]
		if !ok {
			t.Errorf("missing embedded skill %q", name)
			continue
		}
		if skill.Source != types.SkillSourceEmbedded {
			t.Errorf("%s Source = %v, want Embedded", name, skill.Source)
		}
		if skill.Priority != 0 {
			t.Errorf("%s Priority = %d, want 0", name, skill.Priority)
		}
		if skill.Description == "" {
			t.Errorf("%s has empty description", name)
		}
	}
}

func TestLoadEmbeddedSkills_HaveBodies(t *testing.T) {
	skills := LoadEmbeddedSkills()
	for name, skill := range skills {
		if skill.Body == "" {
			t.Errorf("embedded skill %q has empty body", name)
		}
	}
}
