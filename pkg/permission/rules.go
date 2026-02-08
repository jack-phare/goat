package permission

import (
	"fmt"
	"strings"
	"sync"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/jg-phare/goat/pkg/types"
)

// matchRuleContent checks if a rule's content pattern matches the tool input.
// Uses tool-specific field matching with substring and glob patterns.
func matchRuleContent(ruleContent string, toolName string, input map[string]any) bool {
	if ruleContent == "" {
		return true
	}
	if input == nil {
		return false
	}

	// Tool-specific field matching
	switch toolName {
	case "Bash":
		return matchField(ruleContent, input, "command")
	case "Write", "FileWrite", "Edit", "FileEdit":
		return matchField(ruleContent, input, "file_path")
	case "Glob":
		return matchField(ruleContent, input, "pattern") || matchField(ruleContent, input, "path")
	case "Grep":
		return matchField(ruleContent, input, "pattern") || matchField(ruleContent, input, "path")
	default:
		// Generic: match against any string-valued input field
		return matchAnyStringField(ruleContent, input)
	}
}

// matchField checks a specific input field against the rule content pattern.
func matchField(ruleContent string, input map[string]any, fieldName string) bool {
	val, ok := input[fieldName]
	if !ok {
		return false
	}
	str, ok := val.(string)
	if !ok {
		return false
	}
	return matchPattern(ruleContent, str)
}

// matchAnyStringField checks all string-valued input fields against the pattern.
func matchAnyStringField(ruleContent string, input map[string]any) bool {
	for _, val := range input {
		str, ok := val.(string)
		if !ok {
			continue
		}
		if matchPattern(ruleContent, str) {
			return true
		}
	}
	return false
}

// matchPattern checks if a value matches a rule content pattern.
// Tries glob matching first, then falls back to substring matching.
func matchPattern(pattern, value string) bool {
	// Try glob matching (patterns with *, ?, [, {)
	if isGlobPattern(pattern) {
		matched, err := doublestar.Match(pattern, value)
		if err == nil && matched {
			return true
		}
	}

	// Substring matching (case-insensitive)
	return strings.Contains(strings.ToLower(value), strings.ToLower(pattern))
}

// isGlobPattern returns true if the pattern contains glob metacharacters.
func isGlobPattern(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[{")
}

// ApplyUpdate applies a permission update to the Checker's state.
// This is used for session-scoped rule accumulation.
func (c *Checker) ApplyUpdate(update types.PermissionUpdate) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch update.Type {
	case "addRules":
		return c.applyAddRules(update)
	case "replaceRules":
		return c.applyReplaceRules(update)
	case "removeRules":
		return c.applyRemoveRules(update)
	case "setMode":
		if update.Mode != nil {
			c.mode = *update.Mode
		}
		return nil
	case "addDirectories":
		// Parsed but not enforced (out of scope per plan)
		return nil
	case "removeDirectories":
		// Parsed but not enforced (out of scope per plan)
		return nil
	default:
		return fmt.Errorf("unknown permission update type: %q", update.Type)
	}
}

func (c *Checker) applyAddRules(update types.PermissionUpdate) error {
	if update.Rule == nil {
		return nil
	}
	rule := PermissionRule{
		ToolName:    update.Rule.ToolName,
		RuleContent: update.Rule.RuleContent,
		Behavior:    BehaviorAllow, // addRules implies allow
		Source:      update.Destination,
	}
	if update.Destination == "session" {
		c.sessionRules = append(c.sessionRules, rule)
	} else {
		c.configRules = append(c.configRules, rule)
	}
	return nil
}

func (c *Checker) applyReplaceRules(update types.PermissionUpdate) error {
	if update.Rule == nil {
		return nil
	}
	newRule := PermissionRule{
		ToolName:    update.Rule.ToolName,
		RuleContent: update.Rule.RuleContent,
		Behavior:    BehaviorAllow,
		Source:      update.Destination,
	}

	if update.Destination == "session" {
		// Remove existing rules for same tool, then add new
		c.sessionRules = removeRulesForTool(c.sessionRules, update.Rule.ToolName)
		c.sessionRules = append(c.sessionRules, newRule)
	} else {
		c.configRules = removeRulesForTool(c.configRules, update.Rule.ToolName)
		c.configRules = append(c.configRules, newRule)
	}
	return nil
}

func (c *Checker) applyRemoveRules(update types.PermissionUpdate) error {
	if update.Rule == nil {
		return nil
	}
	if update.Destination == "session" {
		c.sessionRules = removeMatchingRule(c.sessionRules, update.Rule.ToolName, update.Rule.RuleContent)
	} else {
		c.configRules = removeMatchingRule(c.configRules, update.Rule.ToolName, update.Rule.RuleContent)
	}
	return nil
}

func removeRulesForTool(rules []PermissionRule, toolName string) []PermissionRule {
	filtered := rules[:0]
	for _, r := range rules {
		if r.ToolName != toolName {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

func removeMatchingRule(rules []PermissionRule, toolName, ruleContent string) []PermissionRule {
	filtered := rules[:0]
	for _, r := range rules {
		if r.ToolName == toolName && r.RuleContent == ruleContent {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered
}

// SessionRules returns a copy of the current session rules (for testing).
func (c *Checker) SessionRules() []PermissionRule {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]PermissionRule, len(c.sessionRules))
	copy(out, c.sessionRules)
	return out
}

// Ensure Checker can be used concurrently.
var _ sync.Locker = (*sync.RWMutex)(nil)
