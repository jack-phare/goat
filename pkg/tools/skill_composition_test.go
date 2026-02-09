package tools

import (
	"context"
	"testing"
)

func TestSkillTool_CompositionBodyCanReferenceSkills(t *testing.T) {
	// A skill body that instructs the LLM to invoke another skill.
	// Since SkillTool is in the registry, the LLM can call it recursively.
	provider := &mockSkillProvider{
		skills: map[string]SkillInfo{
			"verification-specialist": {
				Name:        "verification-specialist",
				Description: "Verify code changes",
				Body:        "Check your available skills for any with \"verifier\" in the name. Use the Skill tool to invoke verifier-playwright.",
			},
		},
	}
	tool := &SkillTool{Skills: provider}

	out, err := tool.Execute(context.Background(), map[string]any{
		"skill": "verification-specialist",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.IsError {
		t.Errorf("unexpected error output: %s", out.Content)
	}
	// The body references the Skill tool, which the LLM can invoke
	if out.Content == "" {
		t.Error("expected non-empty content")
	}
}

func TestSkillTool_NotExcludedFromRegistry(t *testing.T) {
	// Verify that SkillTool registers with name "Skill" which is not "Agent"
	// (the only tool excluded from subagent registries)
	tool := &SkillTool{}
	if tool.Name() == "Agent" {
		t.Error("SkillTool should not have the same name as AgentTool")
	}
}
