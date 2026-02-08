package tools

import (
	"context"
	"testing"
)

func TestMCPTool_Registration(t *testing.T) {
	registry := NewRegistry()
	client := &mockMCPClient{toolResult: MCPToolCallResult{
		Content: []MCPContentBlock{{Type: "text", Text: "tool output"}},
	}}

	registry.RegisterMCPTool("myserver", "search", "Search for things", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string"},
		},
	}, client)

	tool, ok := registry.Get("mcp__myserver__search")
	if !ok {
		t.Fatal("expected tool to be registered")
	}
	if tool.Name() != "mcp__myserver__search" {
		t.Errorf("expected mcp__myserver__search, got %q", tool.Name())
	}
}

func TestMCPTool_Execution(t *testing.T) {
	client := &mockMCPClient{toolResult: MCPToolCallResult{
		Content: []MCPContentBlock{{Type: "text", Text: "search results"}},
	}}
	tool := &MCPTool{
		ServerName: "srv",
		ToolName:   "search",
		Desc:       "Search tool",
		Client:     client,
	}

	out, err := tool.Execute(context.Background(), map[string]any{"query": "test"})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if out.Content != "search results" {
		t.Errorf("got %q, want %q", out.Content, "search results")
	}
}

func TestMCPTool_StubClient(t *testing.T) {
	tool := &MCPTool{
		ServerName: "srv",
		ToolName:   "search",
		Desc:       "Search tool",
	}

	out, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error from stub client")
	}
}

func TestMCPTool_UnregisterTools(t *testing.T) {
	registry := NewRegistry()
	client := &mockMCPClient{}

	registry.RegisterMCPTool("srv1", "tool_a", "desc", nil, client)
	registry.RegisterMCPTool("srv1", "tool_b", "desc", nil, client)
	registry.RegisterMCPTool("srv2", "tool_c", "desc", nil, client)

	// Should have 3 tools
	names := registry.Names()
	if len(names) != 3 {
		t.Fatalf("expected 3 tools, got %d: %v", len(names), names)
	}

	// Unregister srv1's tools
	registry.UnregisterMCPTools("srv1")

	names = registry.Names()
	if len(names) != 1 {
		t.Fatalf("expected 1 tool after unregister, got %d: %v", len(names), names)
	}
	if names[0] != "mcp__srv2__tool_c" {
		t.Errorf("expected srv2 tool to remain, got %q", names[0])
	}
}

func TestMCPTool_NameFormat(t *testing.T) {
	tool := &MCPTool{ServerName: "my_server", ToolName: "my_tool"}
	expected := "mcp__my_server__my_tool"
	if tool.Name() != expected {
		t.Errorf("got %q, want %q", tool.Name(), expected)
	}
}

func TestMCPTool_DefaultSchema(t *testing.T) {
	tool := &MCPTool{ServerName: "s", ToolName: "t"}
	schema := tool.InputSchema()
	if schema["type"] != "object" {
		t.Error("expected default schema to be object type")
	}
}

func TestMCPTool_SideEffect(t *testing.T) {
	tool := &MCPTool{}
	if tool.SideEffect() != SideEffectNetwork {
		t.Error("expected SideEffectNetwork")
	}
}

func TestMCPTool_EmptyResult(t *testing.T) {
	client := &mockMCPClient{toolResult: MCPToolCallResult{}}
	tool := &MCPTool{ServerName: "s", ToolName: "t", Client: client}
	out, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	// With nil error and empty content, should return empty content (not error)
	if out.IsError {
		t.Error("should not be error with nil error")
	}
}
