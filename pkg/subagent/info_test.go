package subagent

import "testing"

func TestListAgentInfo_BuiltIns(t *testing.T) {
	mgr := newTestManager(&mockLLMClient{})
	infos := mgr.ListAgentInfo()
	if len(infos) == 0 {
		t.Fatal("expected agent infos for built-ins")
	}

	// All should have names and descriptions
	for _, info := range infos {
		if info.Name == "" {
			t.Error("expected non-empty name")
		}
		if info.Description == "" {
			t.Errorf("agent %q has empty description", info.Name)
		}
	}
}

func TestListAgentInfo_ActiveFlag(t *testing.T) {
	mgr := newTestManager(&mockLLMClient{})

	// Manually add an active agent
	mgr.mu.Lock()
	mgr.active["a1"] = &RunningAgent{
		ID:    "a1",
		Type:  "general-purpose",
		State: StateRunning,
	}
	mgr.mu.Unlock()

	infos := mgr.ListAgentInfo()
	found := false
	for _, info := range infos {
		if info.Name == "general-purpose" {
			found = true
			if !info.IsActive {
				t.Error("general-purpose should be IsActive=true")
			}
		}
	}
	if !found {
		t.Error("expected general-purpose in infos")
	}
}

func TestListAgentInfo_CompletedNotActive(t *testing.T) {
	mgr := newTestManager(&mockLLMClient{})

	mgr.mu.Lock()
	mgr.active["a1"] = &RunningAgent{
		ID:    "a1",
		Type:  "Explore",
		State: StateCompleted,
	}
	mgr.mu.Unlock()

	infos := mgr.ListAgentInfo()
	for _, info := range infos {
		if info.Name == "Explore" && info.IsActive {
			t.Error("completed agent should not be IsActive")
		}
	}
}

func TestListAgentInfo_Source(t *testing.T) {
	mgr := newTestManager(&mockLLMClient{})
	infos := mgr.ListAgentInfo()

	for _, info := range infos {
		if info.Source != SourceBuiltIn {
			t.Errorf("agent %q source = %v, want SourceBuiltIn", info.Name, info.Source)
		}
	}
}
