package context

import (
	"strings"
	"sync"
)

// ModelContextLimits maps model IDs to their default context window sizes.
var ModelContextLimits = map[string]int{
	"claude-sonnet-4-5-20250929": 200_000,
	"claude-opus-4-5-20250514":   200_000,
	"claude-haiku-4-5-20251001":  200_000,
	"claude-opus-4-6":            200_000,
}

var contextMu sync.RWMutex

// Beta1MFlag is the beta flag that enables 1M context windows.
const Beta1MFlag = "context-1m-2025-08-07"

// DefaultContextLimit is used when the model is not recognized.
const DefaultContextLimit = 200_000

// SetContextLimit sets the context limit for a model. Safe for concurrent use.
func SetContextLimit(model string, limit int) {
	contextMu.Lock()
	defer contextMu.Unlock()
	ModelContextLimits[model] = limit
}

// GetContextLimit returns the effective context limit for a model,
// accounting for beta flags (e.g., 1M context window).
func GetContextLimit(model string, betas []string) int {
	for _, beta := range betas {
		if beta == Beta1MFlag {
			if strings.Contains(model, "sonnet") {
				return 1_000_000
			}
		}
	}
	contextMu.RLock()
	defer contextMu.RUnlock()
	if limit, ok := ModelContextLimits[model]; ok {
		return limit
	}
	return DefaultContextLimit
}
