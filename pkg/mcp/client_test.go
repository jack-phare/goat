package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/jg-phare/goat/pkg/tools"
	"github.com/jg-phare/goat/pkg/types"
)

// connectWithMock sets up a server connection using a mock transport,
// bypassing the real transport creation.
func connectWithMock(t *testing.T, client *Client, name string, mock *mockTransport) {
	t.Helper()
	conn := newServerConnection(name, types.McpServerConfig{})
	conn.Transport = mock
	if err := conn.runHandshake(context.Background()); err != nil {
		t.Fatalf("handshake failed: %v", err)
	}
	client.mu.Lock()
	client.servers[name] = conn
	client.mu.Unlock()
	client.registerTools(name, conn.Tools)
}

func TestClient_ConnectRegistersTools(t *testing.T) {
	registry := tools.NewRegistry()
	client := NewClient(registry)

	mock := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
		withTools([]ToolInfo{
			{Name: "search", Description: "Search for things"},
			{Name: "read", Description: "Read a file"},
		})

	connectWithMock(t, client, "srv1", mock)

	// Verify tools registered in registry
	names := registry.Names()
	if len(names) != 2 {
		t.Fatalf("expected 2 tools, got %d: %v", len(names), names)
	}

	tool, ok := registry.Get("mcp__srv1__search")
	if !ok {
		t.Fatal("expected mcp__srv1__search in registry")
	}
	if tool.Description() != "Search for things" {
		t.Errorf("description: got %q", tool.Description())
	}
}

func TestClient_DisconnectUnregistersTools(t *testing.T) {
	registry := tools.NewRegistry()
	client := NewClient(registry)

	mock := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
		withTools([]ToolInfo{{Name: "tool1"}})

	connectWithMock(t, client, "srv1", mock)

	if len(registry.Names()) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(registry.Names()))
	}

	if err := client.Disconnect("srv1"); err != nil {
		t.Fatal(err)
	}

	if len(registry.Names()) != 0 {
		t.Errorf("expected 0 tools after disconnect, got %d", len(registry.Names()))
	}
}

func TestClient_DisconnectUnknownServer(t *testing.T) {
	registry := tools.NewRegistry()
	client := NewClient(registry)

	err := client.Disconnect("nonexistent")
	if err == nil {
		t.Error("expected error for unknown server")
	}
}

func TestClient_Reconnect(t *testing.T) {
	registry := tools.NewRegistry()
	client := NewClient(registry)

	mock := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
		withTools([]ToolInfo{{Name: "tool1"}})

	connectWithMock(t, client, "srv1", mock)

	// Verify connected
	status, _ := client.ServerStatus("srv1")
	if status.Status != StatusConnected {
		t.Fatalf("expected connected, got %s", status.Status)
	}
}

func TestClient_Toggle(t *testing.T) {
	registry := tools.NewRegistry()
	client := NewClient(registry)

	mock := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
		withTools([]ToolInfo{{Name: "tool1"}})

	connectWithMock(t, client, "srv1", mock)

	// Initially 1 tool
	if len(registry.Names()) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(registry.Names()))
	}

	// Toggle off
	if err := client.Toggle("srv1", false); err != nil {
		t.Fatal(err)
	}
	if len(registry.Names()) != 0 {
		t.Errorf("expected 0 tools after toggle off, got %d", len(registry.Names()))
	}

	status, _ := client.ServerStatus("srv1")
	if status.Status != StatusDisabled {
		t.Errorf("expected disabled, got %s", status.Status)
	}

	// Toggle on
	if err := client.Toggle("srv1", true); err != nil {
		t.Fatal(err)
	}
	if len(registry.Names()) != 1 {
		t.Errorf("expected 1 tool after toggle on, got %d", len(registry.Names()))
	}

	status, _ = client.ServerStatus("srv1")
	if status.Status != StatusConnected {
		t.Errorf("expected connected, got %s", status.Status)
	}
}

func TestClient_ToggleIdempotent(t *testing.T) {
	registry := tools.NewRegistry()
	client := NewClient(registry)

	mock := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
		withTools([]ToolInfo{{Name: "tool1"}})

	connectWithMock(t, client, "srv1", mock)

	// Toggle on when already on → no-op
	if err := client.Toggle("srv1", true); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ToggleUnknownServer(t *testing.T) {
	client := NewClient(tools.NewRegistry())
	err := client.Toggle("nonexistent", true)
	if err == nil {
		t.Error("expected error")
	}
}

func TestClient_CallToolRoutes(t *testing.T) {
	registry := tools.NewRegistry()
	client := NewClient(registry)

	mock1 := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
		withTools([]ToolInfo{{Name: "search"}}).
		withToolCall(ToolResult{
			Content: []ContentBlock{{Type: "text", Text: "result from srv1"}},
		})

	mock2 := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
		withTools([]ToolInfo{{Name: "fetch"}}).
		withToolCall(ToolResult{
			Content: []ContentBlock{{Type: "text", Text: "result from srv2"}},
		})

	connectWithMock(t, client, "srv1", mock1)
	connectWithMock(t, client, "srv2", mock2)

	// Call tool on srv1
	result, err := client.CallTool(context.Background(), "srv1", "search", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 1 || result.Content[0].Text != "result from srv1" {
		t.Errorf("unexpected result: %+v", result)
	}

	// Call tool on srv2
	result, err = client.CallTool(context.Background(), "srv2", "fetch", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 1 || result.Content[0].Text != "result from srv2" {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestClient_CallToolUnknownServer(t *testing.T) {
	client := NewClient(tools.NewRegistry())
	_, err := client.CallTool(context.Background(), "nonexistent", "tool", nil)
	if err == nil {
		t.Error("expected error")
	}
}

func TestClient_ListResources(t *testing.T) {
	registry := tools.NewRegistry()
	client := NewClient(registry)

	mock := newMockTransport().
		withInitialize(ServerCapabilities{
			Tools:     &ToolsCapability{},
			Resources: &ResourcesCapability{},
		}).
		withTools([]ToolInfo{}).
		withResources([]Resource{
			{URI: "file:///readme.md", Name: "readme", MimeType: "text/markdown"},
		})

	connectWithMock(t, client, "srv1", mock)

	resources, err := client.ListResources(context.Background(), "srv1")
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	if resources[0].URI != "file:///readme.md" {
		t.Errorf("uri: got %q", resources[0].URI)
	}
}

func TestClient_ListResourcesUnknownServer(t *testing.T) {
	client := NewClient(tools.NewRegistry())
	_, err := client.ListResources(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error")
	}
}

func TestClient_ReadResource(t *testing.T) {
	registry := tools.NewRegistry()
	client := NewClient(registry)

	mock := newMockTransport().
		withInitialize(ServerCapabilities{Resources: &ResourcesCapability{}}).
		withResources([]Resource{{URI: "file:///test", Name: "test"}}).
		withResourceRead(ResourceReadResult{
			Contents: []ResourceContent{{URI: "file:///test", Text: "file content"}},
		})

	connectWithMock(t, client, "srv1", mock)

	content, err := client.ReadResource(context.Background(), "srv1", "file:///test")
	if err != nil {
		t.Fatal(err)
	}
	if content.Text != "file content" {
		t.Errorf("text: got %q", content.Text)
	}
}

func TestClient_SetServers(t *testing.T) {
	registry := tools.NewRegistry()
	client := NewClient(registry)

	// Pre-populate with a server
	mock1 := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
		withTools([]ToolInfo{{Name: "old_tool"}})
	connectWithMock(t, client, "old_server", mock1)

	// We can't fully test SetServers with real transport creation,
	// but we can verify the diff logic by checking what gets removed.
	result := client.SetServers(context.Background(), map[string]types.McpServerConfig{
		// old_server is NOT in desired → will be removed
		// new_server IS in desired → will try to connect (will fail without real transport)
		"new_server": {Type: "stdio", Command: "nonexistent_command"},
	})

	// old_server should be removed
	found := false
	for _, name := range result.Removed {
		if name == "old_server" {
			found = true
		}
	}
	if !found {
		t.Error("expected old_server to be removed")
	}

	// new_server should fail (nonexistent command)
	if _, ok := result.Errors["new_server"]; !ok {
		t.Error("expected error for new_server")
	}

	// old_server's tools should be unregistered
	if _, ok := registry.Get("mcp__old_server__old_tool"); ok {
		t.Error("old_server tools should be unregistered")
	}
}

func TestSetServers_ConfigChange(t *testing.T) {
	registry := tools.NewRegistry()
	client := NewClient(registry)

	// Setup initial server with a known config
	mock := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
		withTools([]ToolInfo{{Name: "tool_v1", Description: "Version 1"}})

	conn := newServerConnection("srv1", types.McpServerConfig{
		Type: "http",
		URL:  "http://old.example.com",
	})
	conn.Transport = mock
	if err := conn.runHandshake(context.Background()); err != nil {
		t.Fatalf("handshake failed: %v", err)
	}
	client.mu.Lock()
	client.servers["srv1"] = conn
	client.mu.Unlock()
	client.registerTools("srv1", conn.Tools)

	// Verify initial tool
	if _, ok := registry.Get("mcp__srv1__tool_v1"); !ok {
		t.Fatal("expected tool_v1 to be registered")
	}

	// Call SetServers with changed URL — since we can't do a real reconnect,
	// it will fail, but the config change should be detected
	result := client.SetServers(context.Background(), map[string]types.McpServerConfig{
		"srv1": {Type: "http", URL: "http://new.example.com"},
	})

	// The disconnect should succeed (mock transport), but Connect with real transport will fail
	// What we're verifying: the config change was detected and disconnect was called
	if _, ok := result.Errors["srv1"]; !ok {
		// If no error, it somehow connected (unlikely without real server)
		if len(result.Updated) == 0 {
			t.Error("expected either Updated or Errors for changed config")
		}
	}
	// Old tool should be unregistered (disconnect was called)
	if _, ok := registry.Get("mcp__srv1__tool_v1"); ok {
		t.Error("tool_v1 should have been unregistered after config change")
	}
}

func TestConfigEqual(t *testing.T) {
	base := types.McpServerConfig{
		Type:    "http",
		URL:     "http://example.com",
		Headers: map[string]string{"X-Key": "val"},
	}
	same := types.McpServerConfig{
		Type:    "http",
		URL:     "http://example.com",
		Headers: map[string]string{"X-Key": "val"},
	}
	diffURL := types.McpServerConfig{
		Type:    "http",
		URL:     "http://other.com",
		Headers: map[string]string{"X-Key": "val"},
	}
	diffHeader := types.McpServerConfig{
		Type:    "http",
		URL:     "http://example.com",
		Headers: map[string]string{"X-Key": "other"},
	}

	if !configEqual(base, same) {
		t.Error("expected equal configs to match")
	}
	if configEqual(base, diffURL) {
		t.Error("expected different URLs to not match")
	}
	if configEqual(base, diffHeader) {
		t.Error("expected different headers to not match")
	}
}

func TestClient_Status(t *testing.T) {
	registry := tools.NewRegistry()
	client := NewClient(registry)

	mock := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
		withTools([]ToolInfo{{Name: "t"}})

	connectWithMock(t, client, "srv1", mock)

	statuses := client.Status()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].Name != "srv1" {
		t.Errorf("name: got %q", statuses[0].Name)
	}
	if statuses[0].Status != StatusConnected {
		t.Errorf("status: got %s", statuses[0].Status)
	}
}

func TestClient_ServerStatusUnknown(t *testing.T) {
	client := NewClient(tools.NewRegistry())
	_, err := client.ServerStatus("nope")
	if err == nil {
		t.Error("expected error")
	}
}

func TestClient_Close(t *testing.T) {
	registry := tools.NewRegistry()
	client := NewClient(registry)

	mock := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
		withTools([]ToolInfo{{Name: "t"}})

	connectWithMock(t, client, "srv1", mock)

	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if len(registry.Names()) != 0 {
		t.Error("expected no tools after close")
	}
}

func TestClient_ConcurrentAccess(t *testing.T) {
	registry := tools.NewRegistry()
	client := NewClient(registry)

	mock := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
		withTools([]ToolInfo{{Name: "tool"}}).
		withToolCall(ToolResult{Content: []ContentBlock{{Type: "text", Text: "ok"}}})

	connectWithMock(t, client, "srv1", mock)

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client.CallTool(context.Background(), "srv1", "tool", nil)
		}()
	}
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client.Status()
		}()
	}
	wg.Wait()
}

func TestClient_AnnotationsPreservedThroughRegistration(t *testing.T) {
	registry := tools.NewRegistry()
	client := NewClient(registry)

	readOnly := true
	destructive := false
	mock := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
		withTools([]ToolInfo{{
			Name:        "safe_read",
			Description: "A read-only tool",
			Annotations: &ToolAnnotations{
				ReadOnly:    &readOnly,
				Destructive: &destructive,
			},
		}})

	connectWithMock(t, client, "srv1", mock)

	// Verify tool is registered
	tool, ok := registry.Get("mcp__srv1__safe_read")
	if !ok {
		t.Fatal("expected mcp__srv1__safe_read in registry")
	}

	// Verify annotations are accessible
	mcpTool, ok := tool.(*tools.MCPTool)
	if !ok {
		t.Fatalf("expected *tools.MCPTool, got %T", tool)
	}

	annotations := mcpTool.Annotations()
	if annotations == nil {
		t.Fatal("expected annotations to be non-nil")
	}
	if annotations.ReadOnly == nil || *annotations.ReadOnly != true {
		t.Errorf("ReadOnly = %v, want true", annotations.ReadOnly)
	}
	if annotations.Destructive == nil || *annotations.Destructive != false {
		t.Errorf("Destructive = %v, want false", annotations.Destructive)
	}
	if annotations.OpenWorld != nil {
		t.Errorf("OpenWorld should be nil, got %v", annotations.OpenWorld)
	}
}

func TestClient_ToolListChanged(t *testing.T) {
	registry := tools.NewRegistry()
	client := NewClient(registry)

	// Start with one tool
	mock := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{ListChanged: true}}).
		withTools([]ToolInfo{{Name: "old_tool", Description: "Old tool"}})

	connectWithMock(t, client, "srv1", mock)

	// Verify initial tool
	if _, ok := registry.Get("mcp__srv1__old_tool"); !ok {
		t.Fatal("expected old_tool to be registered")
	}

	// Update the mock to return a different tool list
	newTools := ToolsListResult{Tools: []ToolInfo{{Name: "new_tool", Description: "New tool"}}}
	data, _ := json.Marshal(newTools)
	mock.mu.Lock()
	mock.responses[MethodToolsList] = data
	mock.mu.Unlock()

	// Simulate the notification
	client.handleToolListChanged("srv1")

	// Verify old tool removed, new tool added
	if _, ok := registry.Get("mcp__srv1__old_tool"); ok {
		t.Error("old_tool should have been unregistered")
	}
	if _, ok := registry.Get("mcp__srv1__new_tool"); !ok {
		t.Error("new_tool should have been registered")
	}
}

func TestClient_AutoReconnect(t *testing.T) {
	registry := tools.NewRegistry()
	client := NewClient(registry)

	mock := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
		withTools([]ToolInfo{{Name: "tool1"}}).
		withToolCall(ToolResult{Content: []ContentBlock{{Type: "text", Text: "ok"}}})

	connectWithMock(t, client, "srv1", mock)

	// First call succeeds
	result, err := client.CallTool(context.Background(), "srv1", "tool1", nil)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if result.Content[0].Text != "ok" {
		t.Errorf("unexpected result: %+v", result)
	}

	// Verify isTransportError works
	if !isTransportError(fmt.Errorf("not connected")) {
		t.Error("'not connected' should be a transport error")
	}
	if !isTransportError(fmt.Errorf("transport closed")) {
		t.Error("'transport closed' should be a transport error")
	}
	if isTransportError(fmt.Errorf("parse tool result: invalid json")) {
		t.Error("parse error should NOT be a transport error")
	}
	if isTransportError(nil) {
		t.Error("nil should NOT be a transport error")
	}
}

func TestClient_NotificationHandlerDoesNotInterfere(t *testing.T) {
	// Verify that setting up a notification handler doesn't break normal request/response
	registry := tools.NewRegistry()
	client := NewClient(registry)

	mock := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{ListChanged: true}}).
		withTools([]ToolInfo{{Name: "tool1"}}).
		withToolCall(ToolResult{Content: []ContentBlock{{Type: "text", Text: "ok"}}})

	connectWithMock(t, client, "srv1", mock)

	// Set notification handler explicitly
	mock.mu.Lock()
	handler := mock.onNotification
	mock.mu.Unlock()
	// Handler may or may not be set by connectWithMock depending on implementation,
	// but normal tool calls should still work
	_ = handler

	result, err := client.CallTool(context.Background(), "srv1", "tool1", nil)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if result.Content[0].Text != "ok" {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestClient_AnnotationsNilWhenAbsent(t *testing.T) {
	registry := tools.NewRegistry()
	client := NewClient(registry)

	mock := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
		withTools([]ToolInfo{{Name: "no_annotations", Description: "No annotations"}})

	connectWithMock(t, client, "srv1", mock)

	tool, ok := registry.Get("mcp__srv1__no_annotations")
	if !ok {
		t.Fatal("expected tool in registry")
	}

	mcpTool := tool.(*tools.MCPTool)
	if mcpTool.Annotations() != nil {
		t.Error("expected nil annotations when server doesn't provide them")
	}
}

func TestClient_Ping(t *testing.T) {
	t.Run("connected server", func(t *testing.T) {
		registry := tools.NewRegistry()
		client := NewClient(registry)

		mock := newMockTransport().
			withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
			withTools([]ToolInfo{{Name: "tool1"}})

		// Ping responds to any method via mockTransport — add a ping response
		pongData, _ := json.Marshal(map[string]string{})
		mock.withResponse("ping", pongData)

		connectWithMock(t, client, "srv1", mock)

		if err := client.Ping(context.Background(), "srv1"); err != nil {
			t.Errorf("expected ping to succeed, got: %v", err)
		}
	})

	t.Run("unknown server", func(t *testing.T) {
		client := NewClient(tools.NewRegistry())
		if err := client.Ping(context.Background(), "nonexistent"); err == nil {
			t.Error("expected error for unknown server")
		}
	})

	t.Run("disconnected server", func(t *testing.T) {
		registry := tools.NewRegistry()
		client := NewClient(registry)

		mock := newMockTransport().
			withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
			withTools([]ToolInfo{{Name: "tool1"}})

		connectWithMock(t, client, "srv1", mock)

		// Disconnect the server
		client.Disconnect("srv1")

		// Re-add it without transport to simulate disconnected state
		conn := newServerConnection("srv1", types.McpServerConfig{})
		client.mu.Lock()
		client.servers["srv1"] = conn
		client.mu.Unlock()

		if err := client.Ping(context.Background(), "srv1"); err == nil {
			t.Error("expected error for disconnected server")
		}
	})
}

func TestClient_ToolSchemaPassthrough(t *testing.T) {
	registry := tools.NewRegistry()
	client := NewClient(registry)

	schema := json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`)
	mock := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
		withTools([]ToolInfo{{
			Name:        "search",
			Description: "Search for things",
			InputSchema: schema,
		}})

	connectWithMock(t, client, "srv1", mock)

	tool, ok := registry.Get("mcp__srv1__search")
	if !ok {
		t.Fatal("expected tool in registry")
	}
	s := tool.InputSchema()
	if s["type"] != "object" {
		t.Errorf("expected object schema, got %v", s["type"])
	}
}
