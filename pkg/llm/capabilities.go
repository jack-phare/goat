package llm

import "sync"

// ModelCapabilities describes what a model supports.
type ModelCapabilities struct {
	SupportsToolUse  bool
	SupportsThinking bool
	MaxInputTokens   int
	MaxOutputTokens  int
}

var (
	capabilityMu sync.RWMutex
	modelCaps    = map[string]ModelCapabilities{
		"claude-opus-4-5-20250514":   {SupportsToolUse: true, SupportsThinking: true, MaxInputTokens: 200_000, MaxOutputTokens: 16384},
		"claude-sonnet-4-5-20250929": {SupportsToolUse: true, SupportsThinking: true, MaxInputTokens: 200_000, MaxOutputTokens: 16384},
		"claude-haiku-4-5-20251001":  {SupportsToolUse: true, SupportsThinking: true, MaxInputTokens: 200_000, MaxOutputTokens: 16384},
	}
)

// GetCapabilities returns the capabilities for a model and whether it was found.
func GetCapabilities(model string) (ModelCapabilities, bool) {
	capabilityMu.RLock()
	defer capabilityMu.RUnlock()
	c, ok := modelCaps[model]
	return c, ok
}

// SetCapabilities sets the capabilities for a model. Safe for concurrent use.
func SetCapabilities(model string, caps ModelCapabilities) {
	capabilityMu.Lock()
	defer capabilityMu.Unlock()
	modelCaps[model] = caps
}
