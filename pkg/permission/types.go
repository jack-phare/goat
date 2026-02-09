package permission

import (
	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/tools"
	"github.com/jg-phare/goat/pkg/types"
)

// PermissionBehavior is the outcome of a permission check layer.
type PermissionBehavior string

const (
	BehaviorAllow PermissionBehavior = "allow"
	BehaviorDeny  PermissionBehavior = "deny"
	BehaviorAsk   PermissionBehavior = "ask"
)

// ToolRiskLevel classifies a tool's risk for permission defaults.
type ToolRiskLevel int

const (
	RiskNone     ToolRiskLevel = iota // FileRead, Glob, Grep, TodoWrite
	RiskLow                           // Config, ListMcpResources, ReadMcpResource
	RiskMedium                        // FileWrite, FileEdit, NotebookEdit
	RiskHigh                          // Bash, WebFetch, WebSearch, MCP tools
	RiskCritical                      // Agent (spawns subagent)
)

// SideEffectToRisk maps a tool's SideEffectType to a ToolRiskLevel.
func SideEffectToRisk(se tools.SideEffectType) ToolRiskLevel {
	switch se {
	case tools.SideEffectNone:
		return RiskNone
	case tools.SideEffectReadOnly:
		return RiskLow
	case tools.SideEffectMutating:
		return RiskMedium
	case tools.SideEffectNetwork:
		return RiskHigh
	case tools.SideEffectBlocking:
		return RiskLow
	case tools.SideEffectSpawns:
		return RiskCritical
	default:
		return RiskHigh
	}
}

// PermissionRule describes a permission rule for matching tool invocations.
type PermissionRule struct {
	ToolName    string
	RuleContent string             // pattern (glob/substring) for matching tool input
	Behavior    PermissionBehavior // allow or deny
	Source      string             // "userSettings", "projectSettings", "localSettings", "session", "cliArg"
}

// Matches checks if this rule applies to a given tool invocation.
// Exact tool name match required. If RuleContent is empty, matches all invocations.
// If RuleContent is non-empty, rule matching is delegated to matchRuleContent (Phase 3).
func (r *PermissionRule) Matches(toolName string, input map[string]any) bool {
	if r.ToolName != toolName {
		return false
	}
	if r.RuleContent == "" {
		return true // matches all invocations of this tool
	}
	return matchRuleContent(r.RuleContent, toolName, input)
}

// CheckerConfig holds all configuration for constructing a Checker.
type CheckerConfig struct {
	Mode                            string // PermissionMode value
	AllowedTools                    []string
	DisabledTools                   []string
	Rules                           []PermissionRule
	AllowDangerouslySkipPermissions bool
	ToolRegistry                    *tools.Registry  // for looking up SideEffectType
	HookRunner                      agent.HookRunner // for PermissionRequest hook
	CanUseTool                      types.CanUseToolFunc
	UserPrompter                    UserPrompter
	ToolAnnotationLookup            func(string) *MCPAnnotations // optional: resolves MCP annotations for a tool name
}
