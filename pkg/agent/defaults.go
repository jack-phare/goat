package agent

import (
	"os"

	"github.com/jg-phare/goat/pkg/tools"
)

// DefaultRegistry creates a Registry with all tools configured for the given working directory.
// mcpClient may be nil; MCP resource tools will fall back to a stub that returns "not configured".
func DefaultRegistry(cwd string, mcpClient tools.MCPClient) *tools.Registry {
	tm := tools.NewTaskManager()

	registry := tools.NewRegistry(
		// Read-only tools are auto-allowed
		tools.WithAllowed("Read", "Glob", "Grep", "ListMcpResources", "ReadMcpResource"),
	)

	// Core 6 (existing)
	registry.Register(&tools.BashTool{CWD: cwd, TaskManager: tm})
	registry.Register(&tools.FileReadTool{})
	registry.Register(&tools.FileWriteTool{})
	registry.Register(&tools.FileEditTool{})
	registry.Register(&tools.GlobTool{CWD: cwd})
	registry.Register(&tools.GrepTool{CWD: cwd})

	// Background task tools
	registry.Register(&tools.TaskOutputTool{TaskManager: tm})
	registry.Register(&tools.TaskStopTool{TaskManager: tm})

	// State management tools
	registry.Register(&tools.TodoWriteTool{})
	registry.Register(&tools.ConfigTool{Store: tools.NewInMemoryConfigStore()})
	registry.Register(&tools.ExitPlanModeTool{})
	registry.Register(&tools.AskUserQuestionTool{}) // Handler set by host app

	// Network tools
	registry.Register(&tools.WebFetchTool{})
	registry.Register(&tools.WebSearchTool{}) // Provider set by host app

	// Subagent
	registry.Register(&tools.AgentTool{}) // Spawner set by host app

	// Skill
	registry.Register(&tools.SkillTool{}) // Skills provider set by host app

	// MCP resource tools (mcpClient may be nil → falls back to StubMCPClient)
	registry.Register(&tools.ListMcpResourcesTool{Client: mcpClient})
	registry.Register(&tools.ReadMcpResourceTool{Client: mcpClient})
	// Dynamic mcp__* tools registered at runtime by mcp.Client.Connect()

	// NotebookEdit
	registry.Register(&tools.NotebookEditTool{})

	// Team tools (feature-gated)
	if os.Getenv("CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS") == "1" {
		registry.Register(&tools.TeamCreateTool{})   // Coordinator set by host app
		registry.Register(&tools.SendMessageTool{})   // Coordinator set by host app
		registry.Register(&tools.TeamDeleteTool{})     // Coordinator set by host app
	}

	return registry
}
