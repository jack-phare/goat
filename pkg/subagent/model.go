package subagent

import "sync"

// modelAliases maps short names to full model IDs.
var modelAliases = map[string]string{
	"sonnet": "claude-sonnet-4-5-20250929",
	"opus":   "claude-opus-4-5-20250514",
	"haiku":  "claude-haiku-4-5-20251001",
	"mini":   "gpt-5-mini",
	"nano":   "gpt-5-nano",
}

var aliasMu sync.RWMutex

// RegisterModelAlias adds or overwrites a model alias at runtime.
func RegisterModelAlias(alias, fullModelID string) {
	aliasMu.Lock()
	defer aliasMu.Unlock()
	modelAliases[alias] = fullModelID
}

// ModelAliases returns a snapshot copy of the current alias map.
func ModelAliases() map[string]string {
	aliasMu.RLock()
	defer aliasMu.RUnlock()
	result := make(map[string]string, len(modelAliases))
	for k, v := range modelAliases {
		result[k] = v
	}
	return result
}

// resolveModel determines the model for a subagent.
// Priority: input override > definition > parent default.
func resolveModel(defModel string, inputModel *string, parentModel string) string {
	// Input override takes highest priority
	if inputModel != nil && *inputModel != "" {
		return expandModelAlias(*inputModel)
	}

	// Definition model
	if defModel != "" {
		return expandModelAlias(defModel)
	}

	// Fall back to parent
	return parentModel
}

// expandModelAlias expands a short alias to its full model ID.
// Returns the input unchanged if it's not a known alias.
func expandModelAlias(alias string) string {
	aliasMu.RLock()
	defer aliasMu.RUnlock()
	if full, ok := modelAliases[alias]; ok {
		return full
	}
	return alias
}
