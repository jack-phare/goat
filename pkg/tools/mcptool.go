package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// MCPToolAnnotations provides metadata about a tool's behavior (from MCP server).
type MCPToolAnnotations struct {
	ReadOnly    *bool `json:"readOnly,omitempty"`
	Destructive *bool `json:"destructive,omitempty"`
	OpenWorld   *bool `json:"openWorld,omitempty"`
}

// MCPTool represents a single tool exposed by an MCP server.
type MCPTool struct {
	ServerName     string
	ToolName       string
	Desc           string
	Schema         map[string]any
	Client         MCPClient
	ToolAnnotations *MCPToolAnnotations
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

// Annotations returns the MCP tool annotations, or nil if not set.
func (m *MCPTool) Annotations() *MCPToolAnnotations { return m.ToolAnnotations }

func (m *MCPTool) Execute(ctx context.Context, input map[string]any) (ToolOutput, error) {
	// Validate required fields before making the RPC call
	if err := validateMCPRequiredFields(m.Schema, input); err != nil {
		return ToolOutput{
			Content: fmt.Sprintf("Error: %s", err),
			IsError: true,
		}, nil
	}

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

	// Concatenate text content blocks
	var b strings.Builder
	for _, block := range result.Content {
		if block.Type == "text" && block.Text != "" {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(block.Text)
		}
	}

	return ToolOutput{
		Content: b.String(),
		IsError: result.IsError,
	}, nil
}

// validateMCPRequiredFields checks that all required fields from the tool's
// input schema are present in the arguments before sending to the server.
func validateMCPRequiredFields(schema map[string]any, args map[string]any) error {
	if schema == nil {
		return nil
	}
	// Extract "required" from schema — it may be stored as []string or []any
	req, ok := schema["required"]
	if !ok {
		return nil
	}
	var required []string
	switch v := req.(type) {
	case []string:
		required = v
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				required = append(required, s)
			}
		}
	case json.RawMessage:
		json.Unmarshal(v, &required)
	}
	var missing []string
	for _, field := range required {
		if _, ok := args[field]; !ok {
			missing = append(missing, field)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required field(s): %s", strings.Join(missing, ", "))
	}
	return nil
}

// RegisterMCPTool adds a dynamic MCP tool to the registry.
func (r *Registry) RegisterMCPTool(serverName, toolName, description string, schema map[string]any, client MCPClient, annotations *MCPToolAnnotations) {
	tool := &MCPTool{
		ServerName:      serverName,
		ToolName:        toolName,
		Desc:            description,
		Schema:          schema,
		Client:          client,
		ToolAnnotations: annotations,
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
