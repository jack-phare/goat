package llm

import (
	"sync"

	"github.com/jg-phare/goat/pkg/types"
)

// ModelPricing holds per-model token costs.
type ModelPricing struct {
	InputPerMTok       float64 // USD per 1M input tokens
	OutputPerMTok      float64 // USD per 1M output tokens
	CacheReadPerMTok   float64 // USD per 1M cache-read tokens
	CacheCreatePerMTok float64 // USD per 1M cache-creation tokens
}

// DefaultPricing for known models. Access via GetPricing/SetPricing for thread safety.
var DefaultPricing = map[string]ModelPricing{
	"claude-opus-4-5-20250514":   {InputPerMTok: 15.0, OutputPerMTok: 75.0, CacheReadPerMTok: 1.50, CacheCreatePerMTok: 18.75},
	"claude-sonnet-4-5-20250929": {InputPerMTok: 3.0, OutputPerMTok: 15.0, CacheReadPerMTok: 0.30, CacheCreatePerMTok: 3.75},
	"claude-haiku-4-5-20251001":  {InputPerMTok: 0.80, OutputPerMTok: 4.0, CacheReadPerMTok: 0.08, CacheCreatePerMTok: 1.0},
}

var pricingMu sync.RWMutex

// GetPricing returns the pricing for a model and whether it was found.
func GetPricing(model string) (ModelPricing, bool) {
	pricingMu.RLock()
	defer pricingMu.RUnlock()
	p, ok := DefaultPricing[model]
	return p, ok
}

// SetPricing sets the pricing for a model. Safe for concurrent use.
func SetPricing(model string, p ModelPricing) {
	pricingMu.Lock()
	defer pricingMu.Unlock()
	DefaultPricing[model] = p
}

// CalculateCost computes the USD cost for a single API response.
func CalculateCost(model string, usage types.BetaUsage) float64 {
	pricing, ok := GetPricing(model)
	if !ok {
		return 0
	}
	cost := float64(usage.InputTokens) * pricing.InputPerMTok / 1_000_000
	cost += float64(usage.OutputTokens) * pricing.OutputPerMTok / 1_000_000
	cost += float64(usage.CacheReadInputTokens) * pricing.CacheReadPerMTok / 1_000_000
	cost += float64(usage.CacheCreationInputTokens) * pricing.CacheCreatePerMTok / 1_000_000
	return cost
}

// CostTracker accumulates costs across multiple requests for budget enforcement.
// Safe for concurrent use.
type CostTracker struct {
	mu         sync.Mutex
	totalCost  float64
	modelUsage map[string]*ModelUsageAccum
}

// ModelUsageAccum holds per-model token accumulation.
type ModelUsageAccum struct {
	InputTokens              int
	OutputTokens             int
	CacheReadInputTokens     int
	CacheCreationInputTokens int
	CostUSD                  float64
}

// NewCostTracker creates a new CostTracker.
func NewCostTracker() *CostTracker {
	return &CostTracker{
		modelUsage: make(map[string]*ModelUsageAccum),
	}
}

// Add records usage from a single API response and returns cumulative cost.
func (ct *CostTracker) Add(model string, usage types.BetaUsage) float64 {
	cost := CalculateCost(model, usage)
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.totalCost += cost

	accum, ok := ct.modelUsage[model]
	if !ok {
		accum = &ModelUsageAccum{}
		ct.modelUsage[model] = accum
	}
	accum.InputTokens += usage.InputTokens
	accum.OutputTokens += usage.OutputTokens
	accum.CacheReadInputTokens += usage.CacheReadInputTokens
	accum.CacheCreationInputTokens += usage.CacheCreationInputTokens
	accum.CostUSD += cost

	return ct.totalCost
}

// TotalCost returns the cumulative cost in USD.
func (ct *CostTracker) TotalCost() float64 {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	return ct.totalCost
}

// ModelBreakdown returns a copy of per-model usage accumulation.
func (ct *CostTracker) ModelBreakdown() map[string]ModelUsageAccum {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	result := make(map[string]ModelUsageAccum, len(ct.modelUsage))
	for k, v := range ct.modelUsage {
		result[k] = *v
	}
	return result
}
