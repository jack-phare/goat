package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jg-phare/goat/pkg/types"
)

func TestConnection_InitializeHandshake(t *testing.T) {
	mock := newMockTransport().
		withInitialize(ServerCapabilities{
			Tools: &ToolsCapability{},
		}).
		withTools([]ToolInfo{{Name: "search", Description: "Search things"}})

	conn := newServerConnection("test", types.McpServerConfig{Type: "stdio", Command: "echo"})
	conn.Transport = mock

	// Run the handshake manually (bypass createTransport)
	err := conn.runHandshake(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if conn.Status != StatusConnected {
		t.Errorf("expected connected, got %s", conn.Status)
	}
	if conn.Info == nil || conn.Info.Name != "mock-server" {
		t.Error("expected server info")
	}
	if len(conn.Tools) != 1 || conn.Tools[0].Name != "search" {
		t.Errorf("expected 1 tool 'search', got %+v", conn.Tools)
	}
}

func TestConnection_CapabilityGating_NoTools(t *testing.T) {
	mock := newMockTransport().
		withInitialize(ServerCapabilities{
			Resources: &ResourcesCapability{},
		}).
		withResources([]Resource{{URI: "file:///test", Name: "test"}})

	conn := newServerConnection("test", types.McpServerConfig{})
	conn.Transport = mock

	err := conn.runHandshake(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// No tools capability → no tools listed
	if len(conn.Tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(conn.Tools))
	}
	// Resources capability → resources listed
	if len(conn.Resources) != 1 {
		t.Errorf("expected 1 resource, got %d", len(conn.Resources))
	}
}

func TestConnection_CapabilityGating_NoResources(t *testing.T) {
	mock := newMockTransport().
		withInitialize(ServerCapabilities{
			Tools: &ToolsCapability{},
		}).
		withTools([]ToolInfo{{Name: "a"}, {Name: "b"}})

	conn := newServerConnection("test", types.McpServerConfig{})
	conn.Transport = mock

	err := conn.runHandshake(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(conn.Tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(conn.Tools))
	}
	if len(conn.Resources) != 0 {
		t.Errorf("expected 0 resources, got %d", len(conn.Resources))
	}
}

func TestConnection_InitializeError(t *testing.T) {
	mock := newMockTransport() // no initialize response configured → method not found

	conn := newServerConnection("test", types.McpServerConfig{})
	conn.Transport = mock

	err := conn.runHandshake(context.Background())
	if err == nil {
		t.Error("expected error from initialize")
	}
	if conn.Status != StatusFailed {
		t.Errorf("expected status failed, got %s", conn.Status)
	}
}

func TestConnection_CallTool(t *testing.T) {
	mock := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
		withTools([]ToolInfo{{Name: "echo"}}).
		withToolCall(ToolResult{
			Content: []ContentBlock{{Type: "text", Text: "hello"}},
		})

	conn := newServerConnection("test", types.McpServerConfig{})
	conn.Transport = mock
	conn.runHandshake(context.Background())

	result, err := conn.callTool(context.Background(), "echo", map[string]any{"input": "test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 1 || result.Content[0].Text != "hello" {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestConnection_CallToolNotConnected(t *testing.T) {
	conn := newServerConnection("test", types.McpServerConfig{})
	_, err := conn.callTool(context.Background(), "echo", nil)
	if err == nil {
		t.Error("expected error when not connected")
	}
}

func TestConnection_ReadResource(t *testing.T) {
	mock := newMockTransport().
		withInitialize(ServerCapabilities{Resources: &ResourcesCapability{}}).
		withResources([]Resource{{URI: "file:///test", Name: "test"}}).
		withResourceRead(ResourceReadResult{
			Contents: []ResourceContent{{URI: "file:///test", Text: "content here"}},
		})

	conn := newServerConnection("test", types.McpServerConfig{})
	conn.Transport = mock
	conn.runHandshake(context.Background())

	result, err := conn.readResource(context.Background(), "file:///test")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Contents) != 1 || result.Contents[0].Text != "content here" {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestConnection_Disconnect(t *testing.T) {
	mock := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
		withTools([]ToolInfo{{Name: "test"}})

	conn := newServerConnection("test", types.McpServerConfig{})
	conn.Transport = mock
	conn.runHandshake(context.Background())

	if conn.Status != StatusConnected {
		t.Fatalf("expected connected, got %s", conn.Status)
	}

	err := conn.disconnect()
	if err != nil {
		t.Fatal(err)
	}
	if conn.Transport != nil {
		t.Error("expected nil transport after disconnect")
	}
	if len(conn.Tools) != 0 {
		t.Error("expected tools cleared")
	}
	if conn.Status != StatusPending {
		t.Errorf("expected pending status, got %s", conn.Status)
	}
}

func TestConnection_Status(t *testing.T) {
	mock := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
		withTools([]ToolInfo{{Name: "tool1"}})

	conn := newServerConnection("myserver", types.McpServerConfig{})
	conn.Transport = mock
	conn.runHandshake(context.Background())

	s := conn.status()
	if s.Name != "myserver" {
		t.Errorf("expected name 'myserver', got %q", s.Name)
	}
	if s.Status != StatusConnected {
		t.Errorf("expected connected, got %s", s.Status)
	}
	if s.ServerInfo == nil || s.ServerInfo.Name != "mock-server" {
		t.Error("expected server info")
	}
	if len(s.Tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(s.Tools))
	}
}

func TestConnection_ToolCallError(t *testing.T) {
	errResult, _ := json.Marshal(nil)
	_ = errResult

	mock := newMockTransport().
		withInitialize(ServerCapabilities{Tools: &ToolsCapability{}}).
		withTools([]ToolInfo{{Name: "fail"}})

	// Don't configure tools/call response → will get "method not found"
	conn := newServerConnection("test", types.McpServerConfig{})
	conn.Transport = mock
	conn.runHandshake(context.Background())

	_, err := conn.callTool(context.Background(), "fail", nil)
	if err == nil {
		t.Error("expected error from unconfigured tool call")
	}
}
