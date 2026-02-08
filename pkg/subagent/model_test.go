package subagent

import "testing"

func TestResolveModel_InputOverride(t *testing.T) {
	input := strPtr("opus")
	result := resolveModel("sonnet", input, "claude-sonnet-4-5-20250929")
	if result != "claude-opus-4-5-20250514" {
		t.Errorf("result = %q, want opus full ID", result)
	}
}

func TestResolveModel_DefinitionModel(t *testing.T) {
	result := resolveModel("haiku", nil, "claude-sonnet-4-5-20250929")
	if result != "claude-haiku-4-5-20251001" {
		t.Errorf("result = %q, want haiku full ID", result)
	}
}

func TestResolveModel_ParentFallback(t *testing.T) {
	result := resolveModel("", nil, "claude-sonnet-4-5-20250929")
	if result != "claude-sonnet-4-5-20250929" {
		t.Errorf("result = %q, want parent model", result)
	}
}

func TestResolveModel_EmptyInput(t *testing.T) {
	empty := ""
	result := resolveModel("haiku", &empty, "claude-sonnet-4-5-20250929")
	if result != "claude-haiku-4-5-20251001" {
		t.Errorf("result = %q, want haiku (empty input should be ignored)", result)
	}
}

func TestExpandModelAlias_Known(t *testing.T) {
	tests := []struct {
		alias string
		want  string
	}{
		{"sonnet", "claude-sonnet-4-5-20250929"},
		{"opus", "claude-opus-4-5-20250514"},
		{"haiku", "claude-haiku-4-5-20251001"},
	}
	for _, tt := range tests {
		if got := expandModelAlias(tt.alias); got != tt.want {
			t.Errorf("expandModelAlias(%q) = %q, want %q", tt.alias, got, tt.want)
		}
	}
}

func TestExpandModelAlias_Unknown(t *testing.T) {
	result := expandModelAlias("claude-custom-model-v1")
	if result != "claude-custom-model-v1" {
		t.Errorf("result = %q, want passthrough", result)
	}
}

func TestResolveModel_FullModelID(t *testing.T) {
	input := strPtr("claude-opus-4-5-20250514")
	result := resolveModel("", input, "claude-sonnet-4-5-20250929")
	if result != "claude-opus-4-5-20250514" {
		t.Errorf("result = %q, want full ID passthrough", result)
	}
}

func TestResolveModel_AllEmpty(t *testing.T) {
	result := resolveModel("", nil, "")
	if result != "" {
		t.Errorf("result = %q, want empty", result)
	}
}

func strPtr(s string) *string { return &s }
