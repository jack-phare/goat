package subagent

import (
	"encoding/json"
	"fmt"

	"github.com/jg-phare/goat/pkg/types"
)

// Resolve merges agent definitions from multiple sources.
// Priority: builtIn (0) < fileBased (10-30) < cliAgents (100).
// CLI agents are highest priority and always win.
func Resolve(builtIn, cliAgents, fileBased map[string]Definition) map[string]Definition {
	result := make(map[string]Definition)

	// 1. Built-in (lowest priority)
	for name, def := range builtIn {
		result[name] = def
	}

	// 2. File-based (priority-ordered among themselves)
	for name, def := range fileBased {
		if existing, ok := result[name]; ok {
			if def.Priority >= existing.Priority {
				result[name] = def
			}
		} else {
			result[name] = def
		}
	}

	// 3. CLI agents (highest priority — always wins)
	for name, def := range cliAgents {
		result[name] = def
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
		result[name] = FromTypesDefinition(name, ad, SourceCLIFlag, 100)
	}

	return result, nil
}
