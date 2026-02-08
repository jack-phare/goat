package subagent

import "testing"

func TestBuiltInAgents_Count(t *testing.T) {
	agents := BuiltInAgents()
	if len(agents) != 6 {
		t.Errorf("expected 6 built-in agents, got %d", len(agents))
	}
}

func TestBuiltInAgents_HasExpectedKeys(t *testing.T) {
	agents := BuiltInAgents()
	expected := []string{"general-purpose", "Explore", "Plan", "Bash", "statusline-setup", "claude-code-guide"}
	for _, name := range expected {
		if _, ok := agents[name]; !ok {
			t.Errorf("missing built-in agent %q", name)
		}
	}
}

func TestBuiltInAgents_AllHaveDescriptions(t *testing.T) {
	agents := BuiltInAgents()
	for name, def := range agents {
		if def.Description == "" {
			t.Errorf("agent %q has empty description", name)
		}
	}
}

func TestBuiltInAgents_AllHavePrompts(t *testing.T) {
	agents := BuiltInAgents()
	for name, def := range agents {
		if def.Prompt == "" {
			t.Errorf("agent %q has empty prompt", name)
		}
	}
}

func TestBuiltInAgents_AllAreBuiltIn(t *testing.T) {
	agents := BuiltInAgents()
	for name, def := range agents {
		if def.Source != SourceBuiltIn {
			t.Errorf("agent %q source = %v, want SourceBuiltIn", name, def.Source)
		}
	}
}

func TestBuiltInAgents_ExploreModel(t *testing.T) {
	agents := BuiltInAgents()
	explore := agents["Explore"]
	if explore.Model != "haiku" {
		t.Errorf("Explore model = %q, want 'haiku'", explore.Model)
	}
}
