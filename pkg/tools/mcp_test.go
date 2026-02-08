package tools

import (
	"context"
	"strings"
	"testing"
)

type mockMCPClient struct {
	resources       []MCPResource
	resourceContent MCPResourceContent
	toolResult      MCPToolCallResult
	err             error
}

func (m *mockMCPClient) ListResources(_ context.Context, _ string) ([]MCPResource, error) {
	return m.resources, m.err
}

func (m *mockMCPClient) ReadResource(_ context.Context, _, _ string) (MCPResourceContent, error) {
	return m.resourceContent, m.err
}

func (m *mockMCPClient) CallTool(_ context.Context, _, _ string, _ map[string]any) (MCPToolCallResult, error) {
	return m.toolResult, m.err
}

func TestMCP_ListResources(t *testing.T) {
	client := &mockMCPClient{
		resources: []MCPResource{
			{URI: "file:///readme.md", Name: "readme", Description: "Project readme", MimeType: "text/markdown"},
		},
	}
	tool := &ListMcpResourcesTool{Client: client}
	out, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "readme") {
		t.Errorf("expected resource name, got %q", out.Content)
	}
}

func TestMCP_ListResourcesStub(t *testing.T) {
	tool := &ListMcpResourcesTool{} // no client
	out, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error from stub client")
	}
	if !strings.Contains(out.Content, "MCP not configured") {
		t.Errorf("expected 'MCP not configured', got %q", out.Content)
	}
}

func TestMCP_ReadResource(t *testing.T) {
	client := &mockMCPClient{resourceContent: MCPResourceContent{
		URI:  "file:///readme.md",
		Text: "# Hello World",
	}}
	tool := &ReadMcpResourceTool{Client: client}
	out, err := tool.Execute(context.Background(), map[string]any{
		"server_name": "test-server",
		"uri":         "file:///readme.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if out.Content != "# Hello World" {
		t.Errorf("expected content, got %q", out.Content)
	}
}

func TestMCP_ReadResourceMissingFields(t *testing.T) {
	tool := &ReadMcpResourceTool{Client: &mockMCPClient{}}

	out, _ := tool.Execute(context.Background(), map[string]any{"uri": "test"})
	if !out.IsError {
		t.Error("expected error for missing server_name")
	}

	out, _ = tool.Execute(context.Background(), map[string]any{"server_name": "test"})
	if !out.IsError {
		t.Error("expected error for missing uri")
	}
}

func TestMCP_ReadResourceStub(t *testing.T) {
	tool := &ReadMcpResourceTool{} // no client
	out, err := tool.Execute(context.Background(), map[string]any{
		"server_name": "test",
		"uri":         "test://resource",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error from stub client")
	}
}
