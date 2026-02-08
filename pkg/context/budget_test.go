package context

import (
	"testing"

	"github.com/jg-phare/goat/pkg/agent"
)

func TestModelContextLimits(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		{"claude-sonnet-4-5-20250929", 200_000},
		{"claude-opus-4-5-20250514", 200_000},
		{"claude-haiku-4-5-20251001", 200_000},
		{"claude-opus-4-6", 200_000},
	}
	for _, tt := range tests {
		if got := ModelContextLimits[tt.model]; got != tt.want {
			t.Errorf("ModelContextLimits[%s] = %d, want %d", tt.model, got, tt.want)
		}
	}
}

func TestGetContextLimit(t *testing.T) {
	tests := []struct {
		name  string
		model string
		betas []string
		want  int
	}{
		{"sonnet default", "claude-sonnet-4-5-20250929", nil, 200_000},
		{"opus default", "claude-opus-4-5-20250514", nil, 200_000},
		{"haiku default", "claude-haiku-4-5-20251001", nil, 200_000},
		{"unknown model", "gpt-4", nil, DefaultContextLimit},
		{"empty betas", "claude-sonnet-4-5-20250929", []string{}, 200_000},
		{"beta 1M sonnet", "claude-sonnet-4-5-20250929", []string{Beta1MFlag}, 1_000_000},
		{"beta 1M opus (not supported)", "claude-opus-4-5-20250514", []string{Beta1MFlag}, 200_000},
		{"beta 1M haiku (not supported)", "claude-haiku-4-5-20251001", []string{Beta1MFlag}, 200_000},
		{"irrelevant beta", "claude-sonnet-4-5-20250929", []string{"some-other-beta"}, 200_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetContextLimit(tt.model, tt.betas)
			if got != tt.want {
				t.Errorf("GetContextLimit(%s, %v) = %d, want %d", tt.model, tt.betas, got, tt.want)
			}
		})
	}
}

func TestTokenBudget_UtilizationPct(t *testing.T) {
	tests := []struct {
		name   string
		budget agent.TokenBudget
		want   float64
	}{
		{
			"empty",
			agent.TokenBudget{ContextLimit: 200_000},
			0.0,
		},
		{
			"50%",
			agent.TokenBudget{ContextLimit: 200_000, SystemPromptTkns: 50_000, MessageTkns: 50_000, MaxOutputTkns: 0},
			0.5,
		},
		{
			"80% threshold",
			agent.TokenBudget{ContextLimit: 200_000, SystemPromptTkns: 10_000, MessageTkns: 140_000, MaxOutputTkns: 10_000},
			0.8,
		},
		{
			"overflow",
			agent.TokenBudget{ContextLimit: 200_000, SystemPromptTkns: 100_000, MessageTkns: 150_000, MaxOutputTkns: 16384},
			float64(100_000+150_000+16384) / 200_000,
		},
		{
			"zero context limit",
			agent.TokenBudget{ContextLimit: 0, MessageTkns: 100},
			1.0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.budget.UtilizationPct()
			if got != tt.want {
				t.Errorf("UtilizationPct() = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestTokenBudget_IsOverflow(t *testing.T) {
	tests := []struct {
		name   string
		budget agent.TokenBudget
		want   bool
	}{
		{
			"not overflowing",
			agent.TokenBudget{ContextLimit: 200_000, SystemPromptTkns: 10_000, MessageTkns: 100_000, MaxOutputTkns: 16384},
			false,
		},
		{
			"exactly at limit",
			agent.TokenBudget{ContextLimit: 200_000, SystemPromptTkns: 100_000, MessageTkns: 83_616, MaxOutputTkns: 16384},
			false,
		},
		{
			"overflowing",
			agent.TokenBudget{ContextLimit: 200_000, SystemPromptTkns: 100_000, MessageTkns: 100_000, MaxOutputTkns: 16384},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.budget.IsOverflow()
			if got != tt.want {
				t.Errorf("IsOverflow() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTokenBudget_Available(t *testing.T) {
	tests := []struct {
		name   string
		budget agent.TokenBudget
		want   int
	}{
		{
			"has room",
			agent.TokenBudget{ContextLimit: 200_000, SystemPromptTkns: 10_000, MessageTkns: 50_000, MaxOutputTkns: 16384},
			123_616,
		},
		{
			"no room",
			agent.TokenBudget{ContextLimit: 200_000, SystemPromptTkns: 100_000, MessageTkns: 100_000, MaxOutputTkns: 16384},
			0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.budget.Available()
			if got != tt.want {
				t.Errorf("Available() = %d, want %d", got, tt.want)
			}
		})
	}
}
