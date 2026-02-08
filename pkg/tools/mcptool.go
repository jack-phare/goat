package tools

import (
	"context"
	"fmt"
	"strings"
)

// MCPTool represents a single tool exposed by an MCP server.
type MCPTool struct {
	ServerName  string
	ToolName    string
	Desc        string
	Schema      map[string]any
	Client      MCPClient
}

func (m *MCPTool) Name() string {
	return fmt.Sprintf("mcp__%s__%s", m.ServerName, m.ToolName)
}

func (m *MCPTool) Description() string { return m.Desc }

func (m *MCPTool) InputSchema() map[string]any {
	if m.Schema != nil {
		return m.Schema
	}
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (m *MCPTool) SideEffect() SideEffectType { return SideEffectNetwork }

func (m *MCPTool) Execute(ctx context.Context, input map[string]any) (ToolOutput, error) {
	client := m.Client
	if client == nil {
		client = &StubMCPClient{}
	}

	result, err := client.CallTool(ctx, m.ServerName, m.ToolName, input)
	if err != nil {
		return ToolOutput{
			Content: fmt.Sprintf("Error: %s", err),
			IsError: true,
		}, nil
	}

	return ToolOutput{Content: result}, nil
}

// RegisterMCPTool adds a dynamic MCP tool to the registry.
func (r *Registry) RegisterMCPTool(serverName, toolName, description string, schema map[string]any, client MCPClient) {
	tool := &MCPTool{
		ServerName: serverName,
		ToolName:   toolName,
		Desc:       description,
		Schema:     schema,
		Client:     client,
	}
	r.Register(tool)
}

// UnregisterMCPTools removes all tools for a given MCP server.
func (r *Registry) UnregisterMCPTools(serverName string) {
	prefix := fmt.Sprintf("mcp__%s__", serverName)
	for name := range r.tools {
		if strings.HasPrefix(name, prefix) {
			delete(r.tools, name)
		}
	}
}
