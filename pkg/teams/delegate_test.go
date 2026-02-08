package teams

import (
	"testing"
)

func TestDelegateModeEnable(t *testing.T) {
	d := &DelegateModeState{}

	tools := d.Enable()
	if !d.IsActive() {
		t.Error("expected active after Enable")
	}
	if len(tools) != len(DelegateModeTools) {
		t.Errorf("expected %d tools, got %d", len(DelegateModeTools), len(tools))
	}
}

func TestDelegateModeDisable(t *testing.T) {
	d := &DelegateModeState{}

	d.Enable()
	d.Disable()

	if d.IsActive() {
		t.Error("expected inactive after Disable")
	}
}

func TestDelegateModeIsActiveDefault(t *testing.T) {
	d := &DelegateModeState{}
	if d.IsActive() {
		t.Error("expected inactive by default")
	}
}

func TestDelegateModeFilterToolsActive(t *testing.T) {
	d := &DelegateModeState{}
	d.Enable()

	allTools := []string{
		"TeamCreate", "SendMessage", "TeamDelete",
		"TaskCreate", "TaskUpdate", "TaskList", "TaskGet",
		"Bash", "Read", "Write", "Glob", "Grep",
	}

	filtered := d.FilterTools(allTools)

	// Should only contain delegate mode tools
	if len(filtered) != len(DelegateModeTools) {
		t.Fatalf("expected %d filtered tools, got %d: %v", len(DelegateModeTools), len(filtered), filtered)
	}

	allowed := make(map[string]bool)
	for _, name := range filtered {
		allowed[name] = true
	}

	// Bash, Read, Write, Glob, Grep should be filtered out
	for _, excluded := range []string{"Bash", "Read", "Write", "Glob", "Grep"} {
		if allowed[excluded] {
			t.Errorf("tool %s should be excluded in delegate mode", excluded)
		}
	}
}

func TestDelegateModeFilterToolsInactive(t *testing.T) {
	d := &DelegateModeState{}

	allTools := []string{"Bash", "Read", "Write", "TeamCreate"}
	filtered := d.FilterTools(allTools)

	if len(filtered) != len(allTools) {
		t.Errorf("expected all tools when delegate mode is inactive, got %d", len(filtered))
	}
}

func TestDelegateModeFilterToolsEmptyInput(t *testing.T) {
	d := &DelegateModeState{}
	d.Enable()

	filtered := d.FilterTools(nil)
	if len(filtered) != 0 {
		t.Errorf("expected 0 tools for nil input, got %d", len(filtered))
	}
}

func TestDelegateModeFilterToolsNoOverlap(t *testing.T) {
	d := &DelegateModeState{}
	d.Enable()

	// All tools that are NOT in delegate mode
	tools := []string{"Bash", "Read", "Write", "Glob", "Grep", "Agent", "WebFetch"}
	filtered := d.FilterTools(tools)

	if len(filtered) != 0 {
		t.Errorf("expected 0 allowed tools, got %d: %v", len(filtered), filtered)
	}
}

func TestDelegateModeToolsList(t *testing.T) {
	expected := map[string]bool{
		"TeamCreate": true,
		"SendMessage": true,
		"TeamDelete": true,
		"TaskCreate": true,
		"TaskUpdate": true,
		"TaskList":   true,
		"TaskGet":    true,
	}

	for _, tool := range DelegateModeTools {
		if !expected[tool] {
			t.Errorf("unexpected tool in DelegateModeTools: %s", tool)
		}
	}

	if len(DelegateModeTools) != len(expected) {
		t.Errorf("expected %d delegate tools, got %d", len(expected), len(DelegateModeTools))
	}
}
