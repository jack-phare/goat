package subagent

// modelAliases maps short names to full model IDs.
var modelAliases = map[string]string{
	"sonnet": "claude-sonnet-4-5-20250929",
	"opus":   "claude-opus-4-5-20250514",
	"haiku":  "claude-haiku-4-5-20251001",
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
	if full, ok := modelAliases[alias]; ok {
		return full
	}
	return alias
}
