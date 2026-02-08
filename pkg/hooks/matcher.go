package hooks

import "path/filepath"

// matchToolName checks if the tool name in the input matches the pattern.
// An empty pattern matches everything.
// Supports exact match ("Bash") and glob patterns ("mcp__*").
func matchToolName(pattern string, input any) bool {
	if pattern == "" {
		return true
	}

	toolName := extractToolName(input)
	if toolName == "" {
		return true // no tool name to filter on = match
	}

	// Try exact match first
	if pattern == toolName {
		return true
	}

	// Try glob match
	matched, err := filepath.Match(pattern, toolName)
	if err != nil {
		return false
	}
	return matched
}

// extractToolName extracts the tool_name field from hook input.
// Supports both typed structs and map[string]any.
func extractToolName(input any) string {
	switch v := input.(type) {
	case *PreToolUseHookInput:
		return v.ToolName
	case PreToolUseHookInput:
		return v.ToolName
	case *PostToolUseHookInput:
		return v.ToolName
	case PostToolUseHookInput:
		return v.ToolName
	case *PostToolUseFailureHookInput:
		return v.ToolName
	case PostToolUseFailureHookInput:
		return v.ToolName
	case *PermissionRequestHookInput:
		return v.ToolName
	case PermissionRequestHookInput:
		return v.ToolName
	case map[string]any:
		if name, ok := v["tool_name"].(string); ok {
			return name
		}
	}
	return ""
}
