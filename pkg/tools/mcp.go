package tools

import (
	"context"
	"fmt"
	"strings"
)

// MCPResource represents a resource available from an MCP server.
type MCPResource struct {
	URI         string
	Name        string
	Description string
	MimeType    string
}

// MCPClient communicates with MCP servers.
type MCPClient interface {
	ListResources(ctx context.Context, serverName string) ([]MCPResource, error)
	ReadResource(ctx context.Context, serverName, uri string) (string, error)
	CallTool(ctx context.Context, serverName, toolName string, args map[string]any) (string, error)
}

// StubMCPClient returns a not-configured message for all operations.
type StubMCPClient struct{}

func (s *StubMCPClient) ListResources(_ context.Context, _ string) ([]MCPResource, error) {
	return nil, fmt.Errorf("MCP not configured")
}

func (s *StubMCPClient) ReadResource(_ context.Context, _, _ string) (string, error) {
	return "", fmt.Errorf("MCP not configured")
}

func (s *StubMCPClient) CallTool(_ context.Context, _, _ string, _ map[string]any) (string, error) {
	return "", fmt.Errorf("MCP not configured")
}

// ListMcpResourcesTool lists resources from MCP servers.
type ListMcpResourcesTool struct {
	Client MCPClient
}

func (l *ListMcpResourcesTool) Name() string { return "ListMcpResources" }

func (l *ListMcpResourcesTool) Description() string {
	return "Lists resources available from MCP servers."
}

func (l *ListMcpResourcesTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"server_name": map[string]any{
				"type":        "string",
				"description": "MCP server name (lists all if empty)",
			},
		},
	}
}

func (l *ListMcpResourcesTool) SideEffect() SideEffectType { return SideEffectNone }

func (l *ListMcpResourcesTool) Execute(ctx context.Context, input map[string]any) (ToolOutput, error) {
	client := l.Client
	if client == nil {
		client = &StubMCPClient{}
	}

	serverName, _ := input["server_name"].(string)

	resources, err := client.ListResources(ctx, serverName)
	if err != nil {
		return ToolOutput{
			Content: fmt.Sprintf("Error: %s", err),
			IsError: true,
		}, nil
	}

	if len(resources) == 0 {
		return ToolOutput{Content: "No resources found."}, nil
	}

	var b strings.Builder
	b.WriteString("MCP Resources:\n")
	for _, r := range resources {
		fmt.Fprintf(&b, "- %s (%s): %s [%s]\n", r.Name, r.URI, r.Description, r.MimeType)
	}

	return ToolOutput{Content: strings.TrimRight(b.String(), "\n")}, nil
}

// ReadMcpResourceTool reads a specific MCP resource.
type ReadMcpResourceTool struct {
	Client MCPClient
}

func (r *ReadMcpResourceTool) Name() string { return "ReadMcpResource" }

func (r *ReadMcpResourceTool) Description() string {
	return "Reads a specific resource from an MCP server."
}

func (r *ReadMcpResourceTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"server_name": map[string]any{
				"type":        "string",
				"description": "The MCP server name",
			},
			"uri": map[string]any{
				"type":        "string",
				"description": "The resource URI",
			},
		},
		"required": []string{"server_name", "uri"},
	}
}

func (r *ReadMcpResourceTool) SideEffect() SideEffectType { return SideEffectNone }

func (r *ReadMcpResourceTool) Execute(ctx context.Context, input map[string]any) (ToolOutput, error) {
	client := r.Client
	if client == nil {
		client = &StubMCPClient{}
	}

	serverName, ok := input["server_name"].(string)
	if !ok || serverName == "" {
		return ToolOutput{Content: "Error: server_name is required", IsError: true}, nil
	}

	uri, ok := input["uri"].(string)
	if !ok || uri == "" {
		return ToolOutput{Content: "Error: uri is required", IsError: true}, nil
	}

	content, err := client.ReadResource(ctx, serverName, uri)
	if err != nil {
		return ToolOutput{
			Content: fmt.Sprintf("Error: %s", err),
			IsError: true,
		}, nil
	}

	return ToolOutput{Content: content}, nil
}
