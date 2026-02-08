package prompt

import (
	"testing"

	"github.com/jg-phare/goat/pkg/agent"
)

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"abcd", 1},
		{"abcdefgh", 2},
		{"a", 0}, // rounds down
	}

	for _, tt := range tests {
		got := EstimateTokens(tt.input)
		if got != tt.expected {
			t.Errorf("EstimateTokens(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestValidate_PassesForNormalPrompt(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		PromptVersion: "2.1.37",
	}

	err := a.Validate(config, 100000) // generous budget
	if err != nil {
		t.Errorf("expected validation to pass, got: %v", err)
	}
}

func TestValidate_FailsForTinyBudget(t *testing.T) {
	a := &Assembler{}
	config := &agent.AgentConfig{
		PromptVersion: "2.1.37",
	}

	err := a.Validate(config, 10) // impossibly small budget
	if err == nil {
		t.Error("expected validation to fail with tiny budget")
	}
}
