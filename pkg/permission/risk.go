package permission

import "github.com/jg-phare/goat/pkg/types"

// toolRiskByName maps well-known tool names to risk levels.
// Tools not in this map use their SideEffectType for classification.
var toolRiskByName = map[string]ToolRiskLevel{
	// RiskNone — read-only, no side effects
	"Read":     RiskNone,
	"FileRead": RiskNone,
	"Glob":     RiskNone,
	"Grep":     RiskNone,
	"TodoWrite": RiskNone,

	// RiskLow — informational, minimal impact
	"Config":           RiskLow,
	"ListMcpResources": RiskLow,
	"ReadMcpResource":  RiskLow,
	"AskUserQuestion":  RiskLow,
	"ExitPlanMode":     RiskLow,
	"TaskOutput":       RiskLow,
	"TaskStop":         RiskLow,

	// RiskMedium — file mutations
	"Write":        RiskMedium,
	"FileWrite":    RiskMedium,
	"Edit":         RiskMedium,
	"FileEdit":     RiskMedium,
	"NotebookEdit": RiskMedium,

	// RiskHigh — shell execution, network access
	"Bash":      RiskHigh,
	"WebFetch":  RiskHigh,
	"WebSearch": RiskHigh,

	// RiskCritical — spawns subagents
	"Agent": RiskCritical,
}

// ToolRisk returns the risk level for a tool by name.
// MCP tools (mcp__*) default to RiskHigh.
func ToolRisk(toolName string) ToolRiskLevel {
	if risk, ok := toolRiskByName[toolName]; ok {
		return risk
	}
	// MCP tools default to high risk
	if len(toolName) > 5 && toolName[:5] == "mcp__" {
		return RiskHigh
	}
	return RiskHigh // unknown tools default to high
}

// DefaultBehaviorForTool returns the default permission behavior for a tool
// given the current permission mode, based on the mode behavior matrix.
func DefaultBehaviorForTool(mode types.PermissionMode, toolName string) PermissionBehavior {
	risk := ToolRisk(toolName)

	switch mode {
	case types.PermissionModeDefault:
		if risk <= RiskLow {
			return BehaviorAllow
		}
		return BehaviorAsk

	case types.PermissionModeAcceptEdits:
		if risk <= RiskMedium {
			return BehaviorAllow
		}
		return BehaviorAsk

	case types.PermissionModeBypassPermissions:
		return BehaviorAllow

	case types.PermissionModeDontAsk:
		if risk <= RiskLow {
			return BehaviorAllow
		}
		return BehaviorDeny

	case types.PermissionModePlan:
		return BehaviorDeny

	case types.PermissionModeDelegate:
		// Only Agent tool is allowed in delegate mode
		if toolName == "Agent" {
			return BehaviorAllow
		}
		return BehaviorDeny
	}

	return BehaviorAsk
}
