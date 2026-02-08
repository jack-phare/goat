package mcp

import (
	"encoding/json"
	"testing"
)

func TestInitializeResultUnmarshal_FullCapabilities(t *testing.T) {
	raw := `{
		"protocolVersion": "2024-11-05",
		"capabilities": {
			"tools": {"listChanged": true},
			"resources": {"subscribe": true, "listChanged": true},
			"prompts": {"listChanged": false}
		},
		"serverInfo": {"name": "test-server", "version": "1.0.0"}
	}`
	var result InitializeResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	if result.ProtocolVersion != "2024-11-05" {
		t.Errorf("protocolVersion: got %q", result.ProtocolVersion)
	}
	if result.ServerInfo.Name != "test-server" {
		t.Errorf("serverInfo.name: got %q", result.ServerInfo.Name)
	}
	if result.Capabilities.Tools == nil {
		t.Fatal("expected tools capability")
	}
	if !result.Capabilities.Tools.ListChanged {
		t.Error("expected tools.listChanged to be true")
	}
	if result.Capabilities.Resources == nil {
		t.Fatal("expected resources capability")
	}
	if !result.Capabilities.Resources.Subscribe {
		t.Error("expected resources.subscribe to be true")
	}
}

func TestInitializeResultUnmarshal_ToolsOnly(t *testing.T) {
	raw := `{
		"protocolVersion": "2024-11-05",
		"capabilities": {"tools": {}},
		"serverInfo": {"name": "minimal", "version": "0.1"}
	}`
	var result InitializeResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	if result.Capabilities.Tools == nil {
		t.Fatal("expected tools capability")
	}
	if result.Capabilities.Resources != nil {
		t.Error("expected nil resources capability")
	}
	if result.Capabilities.Prompts != nil {
		t.Error("expected nil prompts capability")
	}
}

func TestInitializeResultUnmarshal_EmptyCapabilities(t *testing.T) {
	raw := `{
		"protocolVersion": "2024-11-05",
		"capabilities": {},
		"serverInfo": {"name": "empty", "version": "0.0"}
	}`
	var result InitializeResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	if result.Capabilities.Tools != nil {
		t.Error("expected nil tools")
	}
	if result.Capabilities.Resources != nil {
		t.Error("expected nil resources")
	}
}

func TestToolInfoUnmarshal(t *testing.T) {
	raw := `{
		"name": "search",
		"description": "Search for things",
		"inputSchema": {"type": "object", "properties": {"query": {"type": "string"}}},
		"annotations": {"readOnly": true, "destructive": false}
	}`
	var info ToolInfo
	if err := json.Unmarshal([]byte(raw), &info); err != nil {
		t.Fatal(err)
	}
	if info.Name != "search" {
		t.Errorf("name: got %q", info.Name)
	}
	if info.Description != "Search for things" {
		t.Errorf("description: got %q", info.Description)
	}
	if info.InputSchema == nil {
		t.Fatal("expected inputSchema")
	}
	if info.Annotations == nil {
		t.Fatal("expected annotations")
	}
	if info.Annotations.ReadOnly == nil || !*info.Annotations.ReadOnly {
		t.Error("expected readOnly=true")
	}
	if info.Annotations.Destructive == nil || *info.Annotations.Destructive {
		t.Error("expected destructive=false")
	}
}

func TestToolInfoUnmarshal_MinimalFields(t *testing.T) {
	raw := `{"name": "ping"}`
	var info ToolInfo
	if err := json.Unmarshal([]byte(raw), &info); err != nil {
		t.Fatal(err)
	}
	if info.Name != "ping" {
		t.Errorf("name: got %q", info.Name)
	}
	if info.InputSchema != nil {
		t.Error("expected nil inputSchema")
	}
	if info.Annotations != nil {
		t.Error("expected nil annotations")
	}
}

func TestContentBlockTypes(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		wantType string
	}{
		{"text", `{"type":"text","text":"hello"}`, "text"},
		{"image", `{"type":"image","mimeType":"image/png","data":"base64data"}`, "image"},
		{"resource", `{"type":"resource","uri":"file:///test"}`, "resource"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cb ContentBlock
			if err := json.Unmarshal([]byte(tt.json), &cb); err != nil {
				t.Fatal(err)
			}
			if cb.Type != tt.wantType {
				t.Errorf("type: got %q, want %q", cb.Type, tt.wantType)
			}
		})
	}
}

func TestToolResultUnmarshal(t *testing.T) {
	raw := `{
		"content": [
			{"type": "text", "text": "result 1"},
			{"type": "text", "text": "result 2"}
		],
		"isError": false
	}`
	var result ToolResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(result.Content))
	}
	if result.IsError {
		t.Error("expected isError=false")
	}
}

func TestToolResultUnmarshal_Error(t *testing.T) {
	raw := `{"content": [{"type": "text", "text": "something went wrong"}], "isError": true}`
	var result ToolResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected isError=true")
	}
}

func TestResourceUnmarshal(t *testing.T) {
	raw := `{
		"uri": "file:///project/readme.md",
		"name": "readme",
		"description": "Project readme",
		"mimeType": "text/markdown"
	}`
	var res Resource
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		t.Fatal(err)
	}
	if res.URI != "file:///project/readme.md" {
		t.Errorf("uri: got %q", res.URI)
	}
	if res.MimeType != "text/markdown" {
		t.Errorf("mimeType: got %q", res.MimeType)
	}
}

func TestResourceContentUnmarshal(t *testing.T) {
	raw := `{"uri": "file:///test", "mimeType": "text/plain", "text": "content here"}`
	var rc ResourceContent
	if err := json.Unmarshal([]byte(raw), &rc); err != nil {
		t.Fatal(err)
	}
	if rc.Text != "content here" {
		t.Errorf("text: got %q", rc.Text)
	}
	if rc.Blob != "" {
		t.Error("expected empty blob")
	}
}

func TestResourceContentUnmarshal_Binary(t *testing.T) {
	raw := `{"uri": "file:///image.png", "mimeType": "image/png", "blob": "aGVsbG8="}`
	var rc ResourceContent
	if err := json.Unmarshal([]byte(raw), &rc); err != nil {
		t.Fatal(err)
	}
	if rc.Blob != "aGVsbG8=" {
		t.Errorf("blob: got %q", rc.Blob)
	}
	if rc.Text != "" {
		t.Error("expected empty text for binary resource")
	}
}

func TestToolsListResultUnmarshal(t *testing.T) {
	raw := `{"tools": [{"name": "a"}, {"name": "b", "description": "does b"}]}`
	var result ToolsListResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(result.Tools))
	}
	if result.Tools[0].Name != "a" {
		t.Errorf("first tool name: got %q", result.Tools[0].Name)
	}
}

func TestResourcesListResultUnmarshal(t *testing.T) {
	raw := `{"resources": [{"uri": "file:///a", "name": "a"}]}`
	var result ResourcesListResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(result.Resources))
	}
}

func TestResourceReadResultUnmarshal(t *testing.T) {
	raw := `{"contents": [{"uri": "file:///test", "text": "hello"}]}`
	var result ResourceReadResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result.Contents))
	}
	if result.Contents[0].Text != "hello" {
		t.Errorf("text: got %q", result.Contents[0].Text)
	}
}

func TestMethodConstants(t *testing.T) {
	if MethodInitialize != "initialize" {
		t.Error("MethodInitialize mismatch")
	}
	if MethodInitialized != "notifications/initialized" {
		t.Error("MethodInitialized mismatch")
	}
	if MethodToolsList != "tools/list" {
		t.Error("MethodToolsList mismatch")
	}
	if MethodToolsCall != "tools/call" {
		t.Error("MethodToolsCall mismatch")
	}
	if MethodResourcesList != "resources/list" {
		t.Error("MethodResourcesList mismatch")
	}
	if MethodResourcesRead != "resources/read" {
		t.Error("MethodResourcesRead mismatch")
	}
}

func TestConnectionStatusValues(t *testing.T) {
	statuses := []ConnectionStatus{
		StatusConnected, StatusFailed, StatusNeedsAuth, StatusPending, StatusDisabled,
	}
	for _, s := range statuses {
		if s == "" {
			t.Error("status should not be empty")
		}
	}
}
