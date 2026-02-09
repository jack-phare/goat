package permission

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/jg-phare/goat/pkg/agent"
)

// SkillPermissionScope wraps an inner PermissionChecker and auto-allows tools
// that match the skill's allowed-tools patterns.
type SkillPermissionScope struct {
	AllowedTools []string
	Inner        agent.PermissionChecker
}

// Check evaluates whether a tool is allowed under the skill's scope.
// If the tool matches any allowed-tools pattern, it is permitted.
// Otherwise, the check delegates to the inner checker.
func (s *SkillPermissionScope) Check(ctx context.Context, toolName string, input map[string]any) (agent.PermissionResult, error) {
	if s.matchesAllowed(toolName, input) {
		return agent.PermissionResult{Behavior: "allow"}, nil
	}
	return s.Inner.Check(ctx, toolName, input)
}

// matchesAllowed checks if toolName matches any pattern in AllowedTools.
// Patterns can be:
//   - Exact tool name: "Bash"
//   - Tool name with input constraint: "Bash(gh:*)" matches Bash when command starts with "gh"
//   - Glob patterns: "mcp__*"
func (s *SkillPermissionScope) matchesAllowed(toolName string, input map[string]any) bool {
	for _, pattern := range s.AllowedTools {
		if matchToolPattern(pattern, toolName, input) {
			return true
		}
	}
	return false
}

// matchToolPattern matches a single allowed-tools pattern against a tool name and input.
func matchToolPattern(pattern, toolName string, input map[string]any) bool {
	// Check for "ToolName(constraint)" format
	if parenIdx := strings.Index(pattern, "("); parenIdx >= 0 {
		namePattern := pattern[:parenIdx]
		constraint := strings.TrimSuffix(pattern[parenIdx+1:], ")")

		// Match tool name (exact or glob)
		if !matchName(namePattern, toolName) {
			return false
		}

		// Match constraint against the tool's primary input
		return matchConstraint(constraint, toolName, input)
	}

	// Simple name match (exact or glob)
	return matchName(pattern, toolName)
}

// matchName checks if a tool name matches a pattern (exact or glob).
func matchName(pattern, name string) bool {
	if pattern == name {
		return true
	}
	matched, _ := filepath.Match(pattern, name)
	return matched
}

// matchConstraint checks if a tool's primary input matches a constraint pattern.
// For Bash tools, this checks the "command" input field.
func matchConstraint(constraint, toolName string, input map[string]any) bool {
	// Get the primary input value to check
	var value string
	switch toolName {
	case "Bash":
		if cmd, ok := input["command"].(string); ok {
			value = cmd
		}
	default:
		// For other tools, check common input fields
		for _, key := range []string{"command", "path", "file_path", "url"} {
			if v, ok := input[key].(string); ok {
				value = v
				break
			}
		}
	}

	if value == "" {
		return false
	}

	// Constraint matching: "gh:*" means value starts with "gh "
	if strings.HasSuffix(constraint, ":*") {
		prefix := strings.TrimSuffix(constraint, ":*")
		return strings.HasPrefix(value, prefix+" ") || value == prefix
	}

	// Exact match or glob
	if constraint == value {
		return true
	}
	matched, _ := filepath.Match(constraint, value)
	return matched
}
