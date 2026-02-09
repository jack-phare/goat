package llm

import (
	"math"
	"sync"
	"testing"

	"github.com/jg-phare/goat/pkg/types"
)

func TestCalculateCost(t *testing.T) {
	t.Run("claude-opus-4-5 known value", func(t *testing.T) {
		// 1000 input tokens on opus = 1000 * 15.0 / 1_000_000 = $0.015
		usage := types.BetaUsage{InputTokens: 1000}
		cost := CalculateCost("claude-opus-4-5-20250514", usage)
		if math.Abs(cost-0.015) > 1e-10 {
			t.Errorf("CalculateCost = %f, want 0.015", cost)
		}
	})

	t.Run("claude-sonnet-4-5 full usage", func(t *testing.T) {
		usage := types.BetaUsage{
			InputTokens:              1000,
			OutputTokens:             500,
			CacheReadInputTokens:     200,
			CacheCreationInputTokens: 100,
		}
		cost := CalculateCost("claude-sonnet-4-5-20250929", usage)
		// 1000 * 3.0 / 1M + 500 * 15.0 / 1M + 200 * 0.30 / 1M + 100 * 3.75 / 1M
		// = 0.003 + 0.0075 + 0.00006 + 0.000375 = 0.010935
		expected := 0.010935
		if math.Abs(cost-expected) > 1e-10 {
			t.Errorf("CalculateCost = %f, want %f", cost, expected)
		}
	})

	t.Run("claude-haiku-4-5", func(t *testing.T) {
		usage := types.BetaUsage{InputTokens: 10000, OutputTokens: 5000}
		cost := CalculateCost("claude-haiku-4-5-20251001", usage)
		// 10000 * 0.80 / 1M + 5000 * 4.0 / 1M = 0.008 + 0.02 = 0.028
		expected := 0.028
		if math.Abs(cost-expected) > 1e-10 {
			t.Errorf("CalculateCost = %f, want %f", cost, expected)
		}
	})

	t.Run("unknown model returns 0", func(t *testing.T) {
		cost := CalculateCost("unknown-model", types.BetaUsage{InputTokens: 1000})
		if cost != 0 {
			t.Errorf("CalculateCost for unknown model = %f, want 0", cost)
		}
	})

	t.Run("zero usage", func(t *testing.T) {
		cost := CalculateCost("claude-opus-4-5-20250514", types.BetaUsage{})
		if cost != 0 {
			t.Errorf("CalculateCost with zero usage = %f, want 0", cost)
		}
	})
}

func TestNormalizeModelID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"anthropic/claude-sonnet-4-5-20250929", "claude-sonnet-4-5-20250929"},
		{"openai/gpt-5-nano", "gpt-5-nano"},
		{"claude-haiku-4-5-20251001", "claude-haiku-4-5-20251001"},
		{"gpt-5-mini", "gpt-5-mini"},
		{"a/b/c", "b/c"}, // only strips first prefix
	}
	for _, tt := range tests {
		got := normalizeModelID(tt.input)
		if got != tt.want {
			t.Errorf("normalizeModelID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCalculateCostWithPrefix(t *testing.T) {
	// CalculateCost should work with prefixed model IDs
	usage := types.BetaUsage{InputTokens: 1000, OutputTokens: 500}
	cost := CalculateCost("anthropic/claude-sonnet-4-5-20250929", usage)
	// 1000 * 3.0 / 1M + 500 * 15.0 / 1M = 0.003 + 0.0075 = 0.0105
	expected := 0.0105
	if math.Abs(cost-expected) > 1e-10 {
		t.Errorf("CalculateCost with prefix = %f, want %f", cost, expected)
	}
}

func TestCostTracker(t *testing.T) {
	t.Run("basic add and total", func(t *testing.T) {
		ct := NewCostTracker()
		ct.Add("claude-opus-4-5-20250514", types.BetaUsage{InputTokens: 1000})
		total := ct.TotalCost()
		if math.Abs(total-0.015) > 1e-10 {
			t.Errorf("TotalCost = %f, want 0.015", total)
		}
	})

	t.Run("multiple adds accumulate", func(t *testing.T) {
		ct := NewCostTracker()
		ct.Add("claude-opus-4-5-20250514", types.BetaUsage{InputTokens: 1000})
		ct.Add("claude-opus-4-5-20250514", types.BetaUsage{InputTokens: 1000})
		total := ct.TotalCost()
		if math.Abs(total-0.030) > 1e-10 {
			t.Errorf("TotalCost = %f, want 0.030", total)
		}
	})

	t.Run("model breakdown", func(t *testing.T) {
		ct := NewCostTracker()
		ct.Add("claude-opus-4-5-20250514", types.BetaUsage{InputTokens: 1000, OutputTokens: 500})
		ct.Add("claude-haiku-4-5-20251001", types.BetaUsage{InputTokens: 2000})

		breakdown := ct.ModelBreakdown()
		if len(breakdown) != 2 {
			t.Fatalf("ModelBreakdown has %d models, want 2", len(breakdown))
		}
		if breakdown["claude-opus-4-5-20250514"].InputTokens != 1000 {
			t.Errorf("opus InputTokens = %d, want 1000", breakdown["claude-opus-4-5-20250514"].InputTokens)
		}
		if breakdown["claude-haiku-4-5-20251001"].InputTokens != 2000 {
			t.Errorf("haiku InputTokens = %d, want 2000", breakdown["claude-haiku-4-5-20251001"].InputTokens)
		}
	})

	t.Run("normalizes prefixed model IDs", func(t *testing.T) {
		ct := NewCostTracker()
		ct.Add("anthropic/claude-haiku-4-5-20251001", types.BetaUsage{InputTokens: 2000})
		breakdown := ct.ModelBreakdown()
		// Should be stored under the bare key
		if _, ok := breakdown["claude-haiku-4-5-20251001"]; !ok {
			t.Error("expected bare key 'claude-haiku-4-5-20251001' in breakdown")
		}
		if _, ok := breakdown["anthropic/claude-haiku-4-5-20251001"]; ok {
			t.Error("did not expect prefixed key in breakdown")
		}
		// Cost should be non-zero
		if ct.TotalCost() == 0 {
			t.Error("expected non-zero total cost for prefixed model")
		}
	})

	t.Run("concurrent safety", func(t *testing.T) {
		ct := NewCostTracker()
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ct.Add("claude-opus-4-5-20250514", types.BetaUsage{InputTokens: 1000})
			}()
		}
		wg.Wait()

		total := ct.TotalCost()
		expected := 100 * 0.015
		if math.Abs(total-expected) > 1e-6 {
			t.Errorf("TotalCost after 100 concurrent adds = %f, want %f", total, expected)
		}
	})
}
