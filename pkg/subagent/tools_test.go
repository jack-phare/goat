package subagent

import (
	"reflect"
	"sort"
	"testing"
)

func TestParseTaskRestriction_Unrestricted(t *testing.T) {
	tools := []string{"Read", "Task", "Glob"}
	restriction, remaining := parseTaskRestriction(tools)
	if restriction == nil {
		t.Fatal("expected restriction")
	}
	if !restriction.Unrestricted {
		t.Error("expected Unrestricted=true")
	}
	if containsStr(remaining, "Task") {
		t.Error("remaining should not contain 'Task'")
	}
}

func TestParseTaskRestriction_Typed(t *testing.T) {
	tools := []string{"Read", "Task(Explore,Plan)", "Glob"}
	restriction, remaining := parseTaskRestriction(tools)
	if restriction == nil {
		t.Fatal("expected restriction")
	}
	if restriction.Unrestricted {
		t.Error("expected Unrestricted=false")
	}
	if len(restriction.AllowedTypes) != 2 {
		t.Fatalf("expected 2 allowed types, got %d", len(restriction.AllowedTypes))
	}
	if restriction.AllowedTypes[0] != "Explore" || restriction.AllowedTypes[1] != "Plan" {
		t.Errorf("AllowedTypes = %v", restriction.AllowedTypes)
	}
	if len(remaining) != 2 {
		t.Errorf("remaining = %v, want [Read, Glob]", remaining)
	}
}

func TestParseTaskRestriction_NoTask(t *testing.T) {
	tools := []string{"Read", "Glob", "Grep"}
	restriction, remaining := parseTaskRestriction(tools)
	if restriction != nil {
		t.Errorf("expected nil restriction, got %+v", restriction)
	}
	if len(remaining) != 3 {
		t.Errorf("remaining = %v", remaining)
	}
}

func TestResolveTools_AllowedSubset(t *testing.T) {
	allowed := []string{"Read", "Glob"}
	parent := []string{"Read", "Glob", "Grep", "Bash"}

	result := resolveTools(allowed, nil, parent)
	sort.Strings(result)
	expected := []string{"Glob", "Read"}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("result = %v, want %v", result, expected)
	}
}

func TestResolveTools_InheritAll(t *testing.T) {
	parent := []string{"Read", "Glob", "Grep"}
	result := resolveTools(nil, nil, parent)
	if !reflect.DeepEqual(result, parent) {
		t.Errorf("result = %v, want %v", result, parent)
	}
}

func TestResolveTools_Disallowed(t *testing.T) {
	parent := []string{"Read", "Glob", "Bash", "Grep"}
	disallowed := []string{"Bash"}

	result := resolveTools(nil, disallowed, parent)
	if containsStr(result, "Bash") {
		t.Errorf("result should not contain Bash: %v", result)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 tools, got %d", len(result))
	}
}

func TestResolveTools_AllowedNotInParent(t *testing.T) {
	allowed := []string{"Read", "NonExistent"}
	parent := []string{"Read", "Glob"}

	result := resolveTools(allowed, nil, parent)
	if len(result) != 1 || result[0] != "Read" {
		t.Errorf("result = %v, want [Read]", result)
	}
}

func TestEnsureTools(t *testing.T) {
	tools := []string{"Read", "Glob"}
	result := ensureTools(tools, "Grep", "Read") // Read already present
	if len(result) != 3 {
		t.Fatalf("expected 3 tools, got %d: %v", len(result), result)
	}
	if !containsStr(result, "Grep") {
		t.Error("expected Grep to be added")
	}
}

func TestEnsureTools_AllPresent(t *testing.T) {
	tools := []string{"Read", "Glob"}
	result := ensureTools(tools, "Read", "Glob")
	if len(result) != 2 {
		t.Errorf("expected 2 tools (no additions), got %d", len(result))
	}
}

func TestFilterOut(t *testing.T) {
	tools := []string{"Read", "Task(Explore)", "Glob", "Task"}
	result := filterOut(tools, isTaskEntry)
	if len(result) != 2 {
		t.Errorf("expected 2 non-Task tools, got %d: %v", len(result), result)
	}
}

func TestResolveTools_AllowedAndDisallowed(t *testing.T) {
	allowed := []string{"Read", "Glob", "Bash"}
	disallowed := []string{"Bash"}
	parent := []string{"Read", "Glob", "Bash", "Grep"}

	result := resolveTools(allowed, disallowed, parent)
	sort.Strings(result)
	expected := []string{"Glob", "Read"}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("result = %v, want %v", result, expected)
	}
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
