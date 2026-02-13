package main

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jg-phare/goat/pkg/prompt"
	"github.com/jg-phare/goat/pkg/tools"
)

// projectRoot returns the path to the goat project root.
func projectRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine caller file path")
	}
	// file is cmd/eval/skills_integration_test.go, project root is ../../
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// TestSkillsIntegration_LoadAndInvoke validates the full pipeline:
// load real skills from eval/skills/ -> register -> adapter -> invoke SkillTool.
func TestSkillsIntegration_LoadAndInvoke(t *testing.T) {
	root := projectRoot(t)
	skillsDir := filepath.Join(root, "eval", "skills")

	// Step 1: Load skills using the same path the eval binary uses.
	loader := prompt.NewSkillLoader(skillsDir, "")
	skills, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	// Step 2: Verify all 3 skills were discovered.
	expectedSkills := []string{"go-expert", "project-context", "testing-patterns"}
	if len(skills) != len(expectedSkills) {
		names := make([]string, 0, len(skills))
		for name := range skills {
			names = append(names, name)
		}
		t.Fatalf("expected %d skills, got %d: %v", len(expectedSkills), len(skills), names)
	}
	for _, name := range expectedSkills {
		if _, ok := skills[name]; !ok {
			t.Errorf("expected skill %q not found", name)
		}
	}

	// Step 3: Register skills in registry (same as eval binary).
	registry := prompt.NewSkillRegistry()
	for _, entry := range skills {
		registry.Register(entry)
	}

	// Verify registry methods used by the assembler.
	names := registry.SkillNames()
	if len(names) != 3 {
		t.Errorf("SkillNames() returned %d names, want 3", len(names))
	}

	formatted := registry.FormatSkillsList()
	if formatted == "" {
		t.Fatal("FormatSkillsList() returned empty string")
	}
	for _, name := range expectedSkills {
		if !strings.Contains(formatted, name) {
			t.Errorf("FormatSkillsList() missing skill %q:\n%s", name, formatted)
		}
	}
	t.Logf("FormatSkillsList output:\n%s", formatted)

	// Step 4: Wire the adapter (same as eval binary).
	adapter := &tools.SkillProviderAdapter{Inner: registry}

	// Step 5: Create SkillTool with real arg substituter (same as eval binary).
	skillTool := &tools.SkillTool{
		Skills:         adapter,
		ArgSubstituter: prompt.SubstituteArgs,
	}

	// Step 6: Invoke each skill and verify the body is returned.
	for _, name := range expectedSkills {
		t.Run("invoke_"+name, func(t *testing.T) {
			out, err := skillTool.Execute(context.Background(), map[string]any{
				"skill": name,
			})
			if err != nil {
				t.Fatalf("Execute(%q) error: %v", name, err)
			}
			if out.IsError {
				t.Fatalf("Execute(%q) returned error: %s", name, out.Content)
			}
			if out.Content == "" {
				t.Fatalf("Execute(%q) returned empty body", name)
			}
			// Verify the body contains expected content.
			t.Logf("Skill %q body length: %d chars", name, len(out.Content))
		})
	}

	// Step 7: Verify go-expert skill contains expected Go-specific content.
	t.Run("go-expert_content", func(t *testing.T) {
		out, _ := skillTool.Execute(context.Background(), map[string]any{
			"skill": "go-expert",
		})
		checks := []string{
			"Error Handling",
			"fmt.Errorf",
			"Concurrency",
			"errgroup",
		}
		for _, check := range checks {
			if !strings.Contains(out.Content, check) {
				t.Errorf("go-expert body missing expected content %q", check)
			}
		}
	})

	// Step 8: Verify unknown skill returns error.
	t.Run("invoke_unknown", func(t *testing.T) {
		out, err := skillTool.Execute(context.Background(), map[string]any{
			"skill": "nonexistent-skill",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !out.IsError {
			t.Error("expected error output for unknown skill")
		}
	})

	// Step 9: Verify SkillTool registers correctly in a tool registry.
	t.Run("tool_registry", func(t *testing.T) {
		toolRegistry := tools.NewRegistry(tools.WithAllowed("Read", "Glob", "Grep"))
		toolRegistry.Register(skillTool)

		// Verify the Skill tool appears in the registry.
		got, ok := toolRegistry.Get("Skill")
		if !ok {
			t.Fatal("SkillTool not found in tool registry")
		}
		if got.Name() != "Skill" {
			t.Errorf("tool Name() = %q, want %q", got.Name(), "Skill")
		}
	})
}
