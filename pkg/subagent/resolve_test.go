package subagent

import (
	"testing"

	"github.com/jg-phare/goat/pkg/types"
)

func TestResolve_MergePriority(t *testing.T) {
	builtIn := map[string]Definition{
		"explore": FromTypesDefinition("explore", types.AgentDefinition{Description: "built-in explore"}, SourceBuiltIn, 0),
	}
	cli := map[string]Definition{
		"explore": FromTypesDefinition("explore", types.AgentDefinition{Description: "cli explore"}, SourceCLIFlag, 5),
	}
	fileBased := map[string]Definition{
		"explore": {AgentDefinition: types.AgentDefinition{Name: "explore", Description: "project explore"}, Source: SourceProject, Priority: 30},
	}

	result := Resolve(builtIn, cli, fileBased)
	if len(result) != 1 {
		t.Fatalf("expected 1 def, got %d", len(result))
	}
	if result["explore"].Description != "project explore" {
		t.Errorf("Description = %q, want 'project explore'", result["explore"].Description)
	}
}

func TestResolve_NoOverlap(t *testing.T) {
	builtIn := map[string]Definition{
		"explore": FromTypesDefinition("explore", types.AgentDefinition{Description: "explore"}, SourceBuiltIn, 0),
	}
	cli := map[string]Definition{
		"custom": FromTypesDefinition("custom", types.AgentDefinition{Description: "custom"}, SourceCLIFlag, 5),
	}

	result := Resolve(builtIn, cli, nil)
	if len(result) != 2 {
		t.Fatalf("expected 2 defs, got %d", len(result))
	}
}

func TestResolve_CLIOverridesBuiltIn(t *testing.T) {
	builtIn := map[string]Definition{
		"agent": FromTypesDefinition("agent", types.AgentDefinition{Description: "built-in"}, SourceBuiltIn, 0),
	}
	cli := map[string]Definition{
		"agent": FromTypesDefinition("agent", types.AgentDefinition{Description: "cli"}, SourceCLIFlag, 5),
	}

	result := Resolve(builtIn, cli, nil)
	if result["agent"].Description != "cli" {
		t.Errorf("Description = %q, want 'cli'", result["agent"].Description)
	}
}

func TestParseCLIAgents_Valid(t *testing.T) {
	jsonStr := `{"my-agent":{"description":"CLI agent","prompt":"Do things","model":"sonnet"}}`
	defs, err := ParseCLIAgents(jsonStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(defs) != 1 {
		t.Fatalf("expected 1 def, got %d", len(defs))
	}
	def := defs["my-agent"]
	if def.Description != "CLI agent" {
		t.Errorf("Description = %q", def.Description)
	}
	if def.Source != SourceCLIFlag {
		t.Errorf("Source = %v", def.Source)
	}
	if def.Priority != 5 {
		t.Errorf("Priority = %d", def.Priority)
	}
}

func TestParseCLIAgents_Empty(t *testing.T) {
	defs, err := ParseCLIAgents("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if defs != nil {
		t.Errorf("expected nil for empty string, got %v", defs)
	}
}

func TestParseCLIAgents_Invalid(t *testing.T) {
	_, err := ParseCLIAgents("{invalid json")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
