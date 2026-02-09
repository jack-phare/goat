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
	return ToolRiskWithAnnotations(toolName, nil)
}

// MCPAnnotations describes MCP tool behavior metadata for permission decisions.
type MCPAnnotations struct {
	ReadOnly    bool
	Destructive bool
	OpenWorld   bool
}

// ToolRiskWithAnnotations returns the risk level for a tool, consulting MCP annotations if present.
func ToolRiskWithAnnotations(toolName string, annotations *MCPAnnotations) ToolRiskLevel {
	if risk, ok := toolRiskByName[toolName]; ok {
		return risk
	}
	// MCP tools: use annotations when available
	if len(toolName) > 5 && toolName[:5] == "mcp__" {
		if annotations != nil {
			if annotations.Destructive {
				return RiskCritical
			}
			if annotations.ReadOnly {
				return RiskLow
			}
		}
		return RiskHigh
	}
	return RiskHigh // unknown tools default to high
}

// DefaultBehaviorForTool returns the default permission behavior for a tool
// given the current permission mode, based on the mode behavior matrix.
// If annotations are provided, they are used for MCP tool risk assessment.
func DefaultBehaviorForTool(mode types.PermissionMode, toolName string, annotations *MCPAnnotations) PermissionBehavior {
	risk := ToolRiskWithAnnotations(toolName, annotations)

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
