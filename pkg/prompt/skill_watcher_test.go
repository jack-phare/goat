package prompt

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jg-phare/goat/pkg/types"
)

func TestSkillWatcher_FileCreation(t *testing.T) {
	tmp := t.TempDir()
	skillsDir := filepath.Join(tmp, "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	registry := NewSkillRegistry()
	watcher := NewSkillWatcher(registry, []string{skillsDir})
	watcher.debounce = 100 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer watcher.Stop()

	// Give watcher time to start
	time.Sleep(200 * time.Millisecond)

	// Create a new skill
	skillDir := filepath.Join(skillsDir, "new-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `---
description: A new hot-reloaded skill
---
Body of new skill.
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Wait for debounce + processing
	time.Sleep(800 * time.Millisecond)

	_, ok := registry.Get("new-skill")
	if !ok {
		t.Error("expected skill 'new-skill' to be loaded after creation")
	}
}

func TestSkillWatcher_FileModification(t *testing.T) {
	tmp := t.TempDir()
	skillsDir := filepath.Join(tmp, "skills")
	skillDir := filepath.Join(skillsDir, "mod-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create initial skill
	content := `---
description: Original description
---
Original body.
`
	skillFile := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	registry := NewSkillRegistry()
	registry.Register(types.SkillEntry{
		SkillDefinition: types.SkillDefinition{
			Name:        "mod-skill",
			Description: "Original description",
			Body:        "Original body.",
		},
	})

	watcher := NewSkillWatcher(registry, []string{skillsDir})
	watcher.debounce = 100 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer watcher.Stop()

	time.Sleep(200 * time.Millisecond)

	// Modify the skill
	updated := `---
description: Updated description
---
Updated body.
`
	if err := os.WriteFile(skillFile, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}

	time.Sleep(800 * time.Millisecond)

	entry, ok := registry.Get("mod-skill")
	if !ok {
		t.Fatal("expected skill 'mod-skill' to still exist")
	}
	if entry.Description != "Updated description" {
		t.Errorf("Description = %q, want %q", entry.Description, "Updated description")
	}
}

func TestSkillWatcher_FileDeletion(t *testing.T) {
	tmp := t.TempDir()
	skillsDir := filepath.Join(tmp, "skills")
	skillDir := filepath.Join(skillsDir, "del-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}

	skillFile := filepath.Join(skillDir, "SKILL.md")
	content := `---
description: Will be deleted
---
Body.
`
	if err := os.WriteFile(skillFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	registry := NewSkillRegistry()
	registry.Register(types.SkillEntry{
		SkillDefinition: types.SkillDefinition{
			Name:        "del-skill",
			Description: "Will be deleted",
		},
	})

	watcher := NewSkillWatcher(registry, []string{skillsDir})
	watcher.debounce = 100 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer watcher.Stop()

	time.Sleep(200 * time.Millisecond)

	// Delete the skill file
	if err := os.Remove(skillFile); err != nil {
		t.Fatal(err)
	}

	time.Sleep(800 * time.Millisecond)

	_, ok := registry.Get("del-skill")
	if ok {
		t.Error("expected skill 'del-skill' to be removed after deletion")
	}
}

func TestSkillWatcher_Debounce(t *testing.T) {
	tmp := t.TempDir()
	skillsDir := filepath.Join(tmp, "skills")
	skillDir := filepath.Join(skillsDir, "debounce-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}

	registry := NewSkillRegistry()
	watcher := NewSkillWatcher(registry, []string{skillsDir})
	watcher.debounce = 300 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer watcher.Stop()

	time.Sleep(200 * time.Millisecond)

	// Write multiple times in quick succession
	skillFile := filepath.Join(skillDir, "SKILL.md")
	for i := 0; i < 3; i++ {
		content := `---
description: Version ` + string(rune('0'+i)) + `
---
Body.
`
		if err := os.WriteFile(skillFile, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Wait for debounce to fire
	time.Sleep(600 * time.Millisecond)

	// Skill should exist (we don't check which version because debouncing is non-deterministic)
	_, ok := registry.Get("debounce-skill")
	if !ok {
		t.Error("expected skill 'debounce-skill' to exist after debounced reload")
	}
}

func TestSkillWatcher_StopCancel(t *testing.T) {
	tmp := t.TempDir()
	registry := NewSkillRegistry()
	watcher := NewSkillWatcher(registry, []string{tmp})

	ctx := context.Background()
	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Stop should not panic
	watcher.Stop()
	watcher.Stop() // double stop should be safe
}

func TestSkillWatcher_NonexistentDir(t *testing.T) {
	registry := NewSkillRegistry()
	watcher := NewSkillWatcher(registry, []string{"/nonexistent/dir"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Should not error — just logs a warning
	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("Start: %v (expected no error for missing dir)", err)
	}
	watcher.Stop()
}
