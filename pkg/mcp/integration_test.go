package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jg-phare/goat/pkg/tools"
	"github.com/jg-phare/goat/pkg/types"
)

// TestIntegration_FullLifecycle tests: connect → tools registered → call tool → result → disconnect → tools removed.
func TestIntegration_FullLifecycle(t *testing.T) {
	registry := tools.NewRegistry()
	client := NewClient(registry)

	// Set up mock transport
	mock := newMockTransport().
		withInitialize(ServerCapabilities{
			Tools:     &ToolsCapability{ListChanged: true},
			Resources: &ResourcesCapability{},
		}).
		withTools([]ToolInfo{
			{
				Name:        "search",
				Description: "Search for things",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
			},
			{
				Name:        "read_file",
				Description: "Read a file",
			},
		}).
		withResources([]Resource{
			{URI: "file:///readme.md", Name: "readme", MimeType: "text/markdown"},
		}).
		withToolCall(ToolResult{
			Content: []ContentBlock{
				{Type: "text", Text: "search result: found 3 items"},
			},
		}).
		withResourceRead(ResourceReadResult{
			Contents: []ResourceContent{
				{URI: "file:///readme.md", Text: "# Hello World"},
			},
		})

	// Connect (using mock helper)
	connectWithMock(t, client, "test-server", mock)

	// 1. Verify tools registered
	names := registry.Names()
	if len(names) != 2 {
		t.Fatalf("expected 2 tools, got %d: %v", len(names), names)
	}
	if _, ok := registry.Get("mcp__test-server__search"); !ok {
		t.Error("expected mcp__test-server__search in registry")
	}
	if _, ok := registry.Get("mcp__test-server__read_file"); !ok {
		t.Error("expected mcp__test-server__read_file in registry")
	}

	// 2. Verify server status
	status, err := client.ServerStatus("test-server")
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != StatusConnected {
		t.Errorf("expected connected, got %s", status.Status)
	}
	if status.ServerInfo.Name != "mock-server" {
		t.Errorf("server name: got %q", status.ServerInfo.Name)
	}
	if len(status.Tools) != 2 {
		t.Errorf("expected 2 tools in status, got %d", len(status.Tools))
	}

	// 3. Call a tool via the client
	ctx := context.Background()
	result, err := client.CallTool(ctx, "test-server", "search", map[string]any{"query": "test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 1 || result.Content[0].Text != "search result: found 3 items" {
		t.Errorf("unexpected tool result: %+v", result)
	}

	// 4. Call a tool via the registry (like the agentic loop would)
	registryTool, _ := registry.Get("mcp__test-server__search")
	output, err := registryTool.(tools.Tool).Execute(ctx, map[string]any{"query": "test"})
	if err != nil {
		t.Fatal(err)
	}
	if output.Content != "search result: found 3 items" {
		t.Errorf("registry tool output: got %q", output.Content)
	}

	// 5. List resources
	resources, err := client.ListResources(ctx, "test-server")
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 || resources[0].URI != "file:///readme.md" {
		t.Errorf("unexpected resources: %+v", resources)
	}

	// 6. Read resource
	content, err := client.ReadResource(ctx, "test-server", "file:///readme.md")
	if err != nil {
		t.Fatal(err)
	}
	if content.Text != "# Hello World" {
		t.Errorf("resource content: got %q", content.Text)
	}

	// 7. Disconnect
	if err := client.Disconnect("test-server"); err != nil {
		t.Fatal(err)
	}

	// 8. Verify tools removed
	if len(registry.Names()) != 0 {
		t.Errorf("expected 0 tools after disconnect, got %d: %v", len(registry.Names()), registry.Names())
	}
}

// TestIntegration_MultipleServers tests connecting to multiple servers simultaneously.
func TestIntegration_MultipleServers(t *testing.T) {
	registry := tools.NewRegistry()
	client := NewClient(registry)

	mock1 := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
		withTools([]ToolInfo{
			{Name: "tool_a", Description: "Tool A"},
		}).
		withToolCall(ToolResult{Content: []ContentBlock{{Type: "text", Text: "result_a"}}})

	mock2 := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
		withTools([]ToolInfo{
			{Name: "tool_b", Description: "Tool B"},
			{Name: "tool_c", Description: "Tool C"},
		}).
		withToolCall(ToolResult{Content: []ContentBlock{{Type: "text", Text: "result_bc"}}})

	connectWithMock(t, client, "server1", mock1)
	connectWithMock(t, client, "server2", mock2)

	// 3 total tools across 2 servers
	if len(registry.Names()) != 3 {
		t.Fatalf("expected 3 tools, got %d: %v", len(registry.Names()), registry.Names())
	}

	// Disconnect server1 → only server2 tools remain
	client.Disconnect("server1")
	names := registry.Names()
	if len(names) != 2 {
		t.Fatalf("expected 2 tools, got %d: %v", len(names), names)
	}
	if _, ok := registry.Get("mcp__server2__tool_b"); !ok {
		t.Error("expected server2 tool_b to remain")
	}

	// Close all
	client.Close()
	if len(registry.Names()) != 0 {
		t.Error("expected 0 tools after close")
	}
}

// TestIntegration_SetServersWithMocks tests the SetServers diff logic.
func TestIntegration_SetServersWithMocks(t *testing.T) {
	registry := tools.NewRegistry()
	client := NewClient(registry)

	// Pre-connect old_server
	mockOld := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
		withTools([]ToolInfo{{Name: "old_tool"}})
	connectWithMock(t, client, "old_server", mockOld)

	// Also connect keep_server
	mockKeep := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
		withTools([]ToolInfo{{Name: "keep_tool"}})
	connectWithMock(t, client, "keep_server", mockKeep)

	if len(registry.Names()) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(registry.Names()))
	}

	// SetServers: keep keep_server (same config), remove old_server, add new_server (will fail)
	result := client.SetServers(context.Background(), map[string]types.McpServerConfig{
		"keep_server": {}, // same empty config as connectWithMock uses → unchanged
		"new_server":  {Type: "stdio", Command: "nonexistent"},
	})

	// old_server removed
	found := false
	for _, name := range result.Removed {
		if name == "old_server" {
			found = true
		}
	}
	if !found {
		t.Error("expected old_server in removed list")
	}

	// new_server failed
	if _, ok := result.Errors["new_server"]; !ok {
		t.Error("expected error for new_server")
	}

	// old_server tools gone, keep_server tools remain
	if _, ok := registry.Get("mcp__old_server__old_tool"); ok {
		t.Error("old_server tool should be removed")
	}
	if _, ok := registry.Get("mcp__keep_server__keep_tool"); !ok {
		t.Error("keep_server tool should remain")
	}
}

// TestIntegration_ToggleDisableEnable tests toggling a server off and back on.
func TestIntegration_ToggleDisableEnable(t *testing.T) {
	registry := tools.NewRegistry()
	client := NewClient(registry)

	mock := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
		withTools([]ToolInfo{{Name: "my_tool"}}).
		withToolCall(ToolResult{Content: []ContentBlock{{Type: "text", Text: "works"}}})

	connectWithMock(t, client, "srv", mock)

	// Verify tool exists
	if _, ok := registry.Get("mcp__srv__my_tool"); !ok {
		t.Fatal("expected tool after connect")
	}

	// Disable
	client.Toggle("srv", false)
	if _, ok := registry.Get("mcp__srv__my_tool"); ok {
		t.Error("tool should be removed after disable")
	}
	status, _ := client.ServerStatus("srv")
	if status.Status != StatusDisabled {
		t.Errorf("expected disabled, got %s", status.Status)
	}

	// Enable
	client.Toggle("srv", true)
	if _, ok := registry.Get("mcp__srv__my_tool"); !ok {
		t.Error("tool should be restored after enable")
	}
	status, _ = client.ServerStatus("srv")
	if status.Status != StatusConnected {
		t.Errorf("expected connected, got %s", status.Status)
	}

	// Tool should work after re-enable
	result, err := client.CallTool(context.Background(), "srv", "my_tool", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) == 0 || result.Content[0].Text != "works" {
		t.Error("tool should work after re-enable")
	}
}

// TestIntegration_ErrorToolResult tests handling of isError=true from tool calls.
func TestIntegration_ErrorToolResult(t *testing.T) {
	registry := tools.NewRegistry()
	client := NewClient(registry)

	mock := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
		withTools([]ToolInfo{{Name: "fail"}}).
		withToolCall(ToolResult{
			Content: []ContentBlock{{Type: "text", Text: "something went wrong"}},
			IsError: true,
		})

	connectWithMock(t, client, "srv", mock)

	result, err := client.CallTool(context.Background(), "srv", "fail", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected isError=true")
	}
	if result.Content[0].Text != "something went wrong" {
		t.Errorf("unexpected content: %q", result.Content[0].Text)
	}

	// Verify via registry tool execution
	tool, _ := registry.Get("mcp__srv__fail")
	output, _ := tool.(tools.Tool).Execute(context.Background(), map[string]any{})
	if !output.IsError {
		t.Error("registry tool should propagate isError")
	}
}

// TestIntegration_MultiContentBlocks tests tool results with multiple content blocks.
func TestIntegration_MultiContentBlocks(t *testing.T) {
	registry := tools.NewRegistry()
	client := NewClient(registry)

	mock := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
		withTools([]ToolInfo{{Name: "multi"}}).
		withToolCall(ToolResult{
			Content: []ContentBlock{
				{Type: "text", Text: "line 1"},
				{Type: "text", Text: "line 2"},
				{Type: "image", MimeType: "image/png", Data: "base64data"},
			},
		})

	connectWithMock(t, client, "srv", mock)

	// Via client — all blocks returned
	result, _ := client.CallTool(context.Background(), "srv", "multi", nil)
	if len(result.Content) != 3 {
		t.Fatalf("expected 3 content blocks, got %d", len(result.Content))
	}

	// Via registry tool — text blocks concatenated
	tool, _ := registry.Get("mcp__srv__multi")
	output, _ := tool.(tools.Tool).Execute(context.Background(), nil)
	if output.Content != "line 1\nline 2" {
		t.Errorf("expected concatenated text, got %q", output.Content)
	}
}
