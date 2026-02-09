package permission

import (
	"testing"

	"github.com/jg-phare/goat/pkg/tools"
	"github.com/jg-phare/goat/pkg/types"
)

func TestSideEffectToRisk(t *testing.T) {
	tests := []struct {
		se   tools.SideEffectType
		want ToolRiskLevel
	}{
		{tools.SideEffectNone, RiskNone},
		{tools.SideEffectReadOnly, RiskLow},
		{tools.SideEffectMutating, RiskMedium},
		{tools.SideEffectNetwork, RiskHigh},
		{tools.SideEffectBlocking, RiskLow},
		{tools.SideEffectSpawns, RiskCritical},
	}

	for _, tt := range tests {
		got := SideEffectToRisk(tt.se)
		if got != tt.want {
			t.Errorf("SideEffectToRisk(%d) = %d, want %d", tt.se, got, tt.want)
		}
	}
}

func TestToolRisk_KnownTools(t *testing.T) {
	tests := []struct {
		tool string
		want ToolRiskLevel
	}{
		{"Read", RiskNone},
		{"Glob", RiskNone},
		{"Grep", RiskNone},
		{"TodoWrite", RiskNone},
		{"Config", RiskLow},
		{"ListMcpResources", RiskLow},
		{"AskUserQuestion", RiskLow},
		{"Write", RiskMedium},
		{"Edit", RiskMedium},
		{"NotebookEdit", RiskMedium},
		{"Bash", RiskHigh},
		{"WebFetch", RiskHigh},
		{"WebSearch", RiskHigh},
		{"Agent", RiskCritical},
	}

	for _, tt := range tests {
		got := ToolRisk(tt.tool)
		if got != tt.want {
			t.Errorf("ToolRisk(%q) = %d, want %d", tt.tool, got, tt.want)
		}
	}
}

func TestToolRisk_MCPTools(t *testing.T) {
	if got := ToolRisk("mcp__server__tool"); got != RiskHigh {
		t.Errorf("mcp tool risk = %d, want %d (RiskHigh)", got, RiskHigh)
	}
}

func TestToolRisk_Unknown(t *testing.T) {
	if got := ToolRisk("UnknownTool"); got != RiskHigh {
		t.Errorf("unknown tool risk = %d, want %d (RiskHigh)", got, RiskHigh)
	}
}

func TestToolRiskWithAnnotations_MCPReadOnly(t *testing.T) {
	risk := ToolRiskWithAnnotations("mcp__server__read_tool", &MCPAnnotations{ReadOnly: true})
	if risk != RiskLow {
		t.Errorf("readOnly MCP tool risk = %d, want %d (RiskLow)", risk, RiskLow)
	}
}

func TestToolRiskWithAnnotations_MCPDestructive(t *testing.T) {
	risk := ToolRiskWithAnnotations("mcp__server__delete_tool", &MCPAnnotations{Destructive: true})
	if risk != RiskCritical {
		t.Errorf("destructive MCP tool risk = %d, want %d (RiskCritical)", risk, RiskCritical)
	}
}

func TestToolRiskWithAnnotations_MCPNoAnnotations(t *testing.T) {
	risk := ToolRiskWithAnnotations("mcp__server__tool", nil)
	if risk != RiskHigh {
		t.Errorf("MCP tool without annotations risk = %d, want %d (RiskHigh)", risk, RiskHigh)
	}
}

func TestToolRiskWithAnnotations_NonMCPTool(t *testing.T) {
	// Known tools should ignore annotations
	risk := ToolRiskWithAnnotations("Read", &MCPAnnotations{Destructive: true})
	if risk != RiskNone {
		t.Errorf("Read tool risk = %d, want %d (RiskNone)", risk, RiskNone)
	}
}

func TestToolRiskWithAnnotations_DestructiveOverridesReadOnly(t *testing.T) {
	// When both are set, destructive takes precedence
	risk := ToolRiskWithAnnotations("mcp__s__t", &MCPAnnotations{ReadOnly: true, Destructive: true})
	if risk != RiskCritical {
		t.Errorf("destructive+readOnly MCP tool risk = %d, want %d (RiskCritical)", risk, RiskCritical)
	}
}

func TestDefaultBehaviorForTool_MCPWithAnnotations(t *testing.T) {
	// MCP tool with readOnly annotation → RiskLow → allowed in default mode
	got := DefaultBehaviorForTool(types.PermissionModeDefault, "mcp__server__read_tool", &MCPAnnotations{ReadOnly: true})
	if got != BehaviorAllow {
		t.Errorf("readOnly MCP tool: got %q, want allow", got)
	}

	// MCP tool with destructive annotation → RiskCritical → ask in default mode
	got = DefaultBehaviorForTool(types.PermissionModeDefault, "mcp__server__delete_tool", &MCPAnnotations{Destructive: true})
	if got != BehaviorAsk {
		t.Errorf("destructive MCP tool: got %q, want ask", got)
	}

	// MCP tool without annotations → RiskHigh → ask in default mode
	got = DefaultBehaviorForTool(types.PermissionModeDefault, "mcp__server__tool", nil)
	if got != BehaviorAsk {
		t.Errorf("unannotated MCP tool: got %q, want ask", got)
	}

	// readOnly MCP in acceptEdits → RiskLow → allow (medium and below allowed)
	got = DefaultBehaviorForTool(types.PermissionModeAcceptEdits, "mcp__server__read_tool", &MCPAnnotations{ReadOnly: true})
	if got != BehaviorAllow {
		t.Errorf("readOnly MCP in acceptEdits: got %q, want allow", got)
	}
}

func TestDefaultBehaviorForTool_ModeBehaviorMatrix(t *testing.T) {
	tests := []struct {
		mode types.PermissionMode
		tool string
		want PermissionBehavior
	}{
		// default mode
		{types.PermissionModeDefault, "Read", BehaviorAllow},
		{types.PermissionModeDefault, "Glob", BehaviorAllow},
		{types.PermissionModeDefault, "Config", BehaviorAllow},
		{types.PermissionModeDefault, "Write", BehaviorAsk},
		{types.PermissionModeDefault, "Bash", BehaviorAsk},
		{types.PermissionModeDefault, "WebFetch", BehaviorAsk},
		{types.PermissionModeDefault, "Agent", BehaviorAsk},

		// acceptEdits mode
		{types.PermissionModeAcceptEdits, "Read", BehaviorAllow},
		{types.PermissionModeAcceptEdits, "Write", BehaviorAllow},
		{types.PermissionModeAcceptEdits, "Edit", BehaviorAllow},
		{types.PermissionModeAcceptEdits, "Bash", BehaviorAsk},
		{types.PermissionModeAcceptEdits, "Agent", BehaviorAsk},

		// bypassPermissions mode
		{types.PermissionModeBypassPermissions, "Bash", BehaviorAllow},
		{types.PermissionModeBypassPermissions, "Agent", BehaviorAllow},

		// plan mode
		{types.PermissionModePlan, "Read", BehaviorDeny},
		{types.PermissionModePlan, "Bash", BehaviorDeny},

		// delegate mode
		{types.PermissionModeDelegate, "Agent", BehaviorAllow},
		{types.PermissionModeDelegate, "Bash", BehaviorDeny},
		{types.PermissionModeDelegate, "Read", BehaviorDeny},

		// dontAsk mode
		{types.PermissionModeDontAsk, "Read", BehaviorAllow},
		{types.PermissionModeDontAsk, "Config", BehaviorAllow},
		{types.PermissionModeDontAsk, "Write", BehaviorDeny},
		{types.PermissionModeDontAsk, "Bash", BehaviorDeny},
		{types.PermissionModeDontAsk, "WebFetch", BehaviorDeny},
	}

	for _, tt := range tests {
		got := DefaultBehaviorForTool(tt.mode, tt.tool, nil)
		if got != tt.want {
			t.Errorf("DefaultBehaviorForTool(%q, %q, nil) = %q, want %q", tt.mode, tt.tool, got, tt.want)
		}
	}
}
