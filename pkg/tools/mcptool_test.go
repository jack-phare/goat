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
	}, client, nil)

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

	registry.RegisterMCPTool("srv1", "tool_a", "desc", nil, client, nil)
	registry.RegisterMCPTool("srv1", "tool_b", "desc", nil, client, nil)
	registry.RegisterMCPTool("srv2", "tool_c", "desc", nil, client, nil)

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

func TestMCPTool_RequiredFieldValidation(t *testing.T) {
	client := &mockMCPClient{toolResult: MCPToolCallResult{
		Content: []MCPContentBlock{{Type: "text", Text: "ok"}},
	}}

	t.Run("missing required field", func(t *testing.T) {
		tool := &MCPTool{
			ServerName: "srv",
			ToolName:   "search",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
				},
				"required": []any{"query"},
			},
			Client: client,
		}

		out, err := tool.Execute(context.Background(), map[string]any{})
		if err != nil {
			t.Fatal(err)
		}
		if !out.IsError {
			t.Error("expected error for missing required field")
		}
		if out.Content == "" {
			t.Error("expected error message")
		}
	})

	t.Run("all required fields present", func(t *testing.T) {
		tool := &MCPTool{
			ServerName: "srv",
			ToolName:   "search",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
				},
				"required": []any{"query"},
			},
			Client: client,
		}

		out, err := tool.Execute(context.Background(), map[string]any{"query": "hello"})
		if err != nil {
			t.Fatal(err)
		}
		if out.IsError {
			t.Errorf("unexpected error: %s", out.Content)
		}
	})

	t.Run("no required fields in schema", func(t *testing.T) {
		tool := &MCPTool{
			ServerName: "srv",
			ToolName:   "search",
			Schema: map[string]any{
				"type": "object",
			},
			Client: client,
		}

		out, err := tool.Execute(context.Background(), map[string]any{})
		if err != nil {
			t.Fatal(err)
		}
		if out.IsError {
			t.Errorf("unexpected error: %s", out.Content)
		}
	})

	t.Run("nil schema", func(t *testing.T) {
		tool := &MCPTool{
			ServerName: "srv",
			ToolName:   "search",
			Client:     client,
		}

		out, err := tool.Execute(context.Background(), map[string]any{})
		if err != nil {
			t.Fatal(err)
		}
		if out.IsError {
			t.Errorf("unexpected error: %s", out.Content)
		}
	})

	t.Run("required as string slice", func(t *testing.T) {
		tool := &MCPTool{
			ServerName: "srv",
			ToolName:   "search",
			Schema: map[string]any{
				"type":     "object",
				"required": []string{"query", "limit"},
			},
			Client: client,
		}

		out, err := tool.Execute(context.Background(), map[string]any{"query": "hello"})
		if err != nil {
			t.Fatal(err)
		}
		if !out.IsError {
			t.Error("expected error for missing 'limit' field")
		}
	})
}

func TestMCPTool_AnnotationsAccessor(t *testing.T) {
	ro := true
	destr := false

	t.Run("with annotations", func(t *testing.T) {
		tool := &MCPTool{
			ServerName: "srv",
			ToolName:   "delete_item",
			Desc:       "Deletes an item",
			ToolAnnotations: &MCPToolAnnotations{
				ReadOnly:    &ro,
				Destructive: &destr,
			},
		}

		ann := tool.Annotations()
		if ann == nil {
			t.Fatal("expected annotations, got nil")
		}
		if ann.ReadOnly == nil || *ann.ReadOnly != true {
			t.Error("expected ReadOnly=true")
		}
		if ann.Destructive == nil || *ann.Destructive != false {
			t.Error("expected Destructive=false")
		}
		if ann.OpenWorld != nil {
			t.Errorf("expected OpenWorld=nil, got %v", *ann.OpenWorld)
		}
	})

	t.Run("nil annotations", func(t *testing.T) {
		tool := &MCPTool{ServerName: "srv", ToolName: "search"}
		if tool.Annotations() != nil {
			t.Error("expected nil annotations")
		}
	})
}
