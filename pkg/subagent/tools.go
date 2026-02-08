package subagent

import "strings"

// TaskRestriction describes which agent types can be spawned by a subagent.
type TaskRestriction struct {
	Unrestricted bool     // true = any agent type
	AllowedTypes []string // specific agent types allowed (when Unrestricted is false)
}

// parseTaskRestriction extracts Task(type1,type2) entries from the tools list.
// Returns the parsed restriction and the remaining (non-Task) tool names.
func parseTaskRestriction(tools []string) (*TaskRestriction, []string) {
	var remaining []string
	var allowedTypes []string
	hasTask := false

	for _, t := range tools {
		t = strings.TrimSpace(t)
		if t == "Task" {
			hasTask = true
			// Unrestricted Task spawning
			return &TaskRestriction{Unrestricted: true}, filterOut(tools, isTaskEntry)
		}
		if strings.HasPrefix(t, "Task(") && strings.HasSuffix(t, ")") {
			hasTask = true
			inner := t[5 : len(t)-1]
			for _, typ := range strings.Split(inner, ",") {
				typ = strings.TrimSpace(typ)
				if typ != "" {
					allowedTypes = append(allowedTypes, typ)
				}
			}
		} else {
			remaining = append(remaining, t)
		}
	}

	if !hasTask {
		return nil, tools
	}

	return &TaskRestriction{AllowedTypes: allowedTypes}, remaining
}

// isTaskEntry returns true if the string is a Task or Task(...) entry.
func isTaskEntry(s string) bool {
	s = strings.TrimSpace(s)
	return s == "Task" || (strings.HasPrefix(s, "Task(") && strings.HasSuffix(s, ")"))
}

// resolveTools determines the final tool set for a subagent.
// If allowed is non-empty, only those tools are used (intersected with parent).
// Then disallowed tools are removed.
func resolveTools(allowed, disallowed, parentTools []string) []string {
	var base []string

	if len(allowed) > 0 {
		// Use only explicitly allowed tools that exist in parent
		parentSet := toSet(parentTools)
		for _, t := range allowed {
			if parentSet[t] {
				base = append(base, t)
			}
		}
	} else {
		// Inherit all parent tools
		base = make([]string, len(parentTools))
		copy(base, parentTools)
	}

	// Remove disallowed
	if len(disallowed) > 0 {
		disallowedSet := toSet(disallowed)
		base = filterFunc(base, func(s string) bool {
			return !disallowedSet[s]
		})
	}

	return base
}

// filterOut returns tools that don't match the predicate.
func filterOut(tools []string, pred func(string) bool) []string {
	var result []string
	for _, t := range tools {
		if !pred(t) {
			result = append(result, t)
		}
	}
	return result
}

// filterFunc returns tools that match the predicate.
func filterFunc(tools []string, pred func(string) bool) []string {
	var result []string
	for _, t := range tools {
		if pred(t) {
			result = append(result, t)
		}
	}
	return result
}

// ensureTools adds required tools if not already present.
func ensureTools(tools []string, required ...string) []string {
	set := toSet(tools)
	for _, r := range required {
		if !set[r] {
			tools = append(tools, r)
			set[r] = true
		}
	}
	return tools
}

// toSet converts a string slice to a set.
func toSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}
