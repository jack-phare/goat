package subagent

import (
	"encoding/json"
	"fmt"

	"github.com/jg-phare/goat/pkg/types"
)

// Resolve merges agent definitions from multiple sources.
// Priority: builtIn (0) < cliAgents (5) < fileBased (10-30).
// Higher priority overwrites lower.
func Resolve(builtIn, cliAgents, fileBased map[string]Definition) map[string]Definition {
	result := make(map[string]Definition)

	// 1. Built-in (lowest priority)
	for name, def := range builtIn {
		result[name] = def
	}

	// 2. CLI agents
	for name, def := range cliAgents {
		result[name] = def
	}

	// 3. File-based (highest priority — each already has its own priority)
	for name, def := range fileBased {
		if existing, ok := result[name]; ok {
			// Only overwrite if new def has higher or equal priority
			if def.Priority >= existing.Priority {
				result[name] = def
			}
		} else {
			result[name] = def
		}
	}

	return result
}

// ParseCLIAgents parses agent definitions from a JSON string (--agents flag).
// The JSON should be a map of name → AgentDefinition.
func ParseCLIAgents(jsonStr string) (map[string]Definition, error) {
	if jsonStr == "" {
		return nil, nil
	}

	var raw map[string]types.AgentDefinition
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil, fmt.Errorf("parsing --agents JSON: %w", err)
	}

	result := make(map[string]Definition, len(raw))
	for name, ad := range raw {
		result[name] = FromTypesDefinition(name, ad, SourceCLIFlag, 5)
	}

	return result, nil
}
