package mcp

import (
	"encoding/json"
	"testing"
)

func TestNewRequest(t *testing.T) {
	req := newRequest(1, "tools/list", nil)
	if req.JSONRPC != "2.0" {
		t.Errorf("expected jsonrpc 2.0, got %q", req.JSONRPC)
	}
	if req.ID == nil || *req.ID != 1 {
		t.Errorf("expected id 1, got %v", req.ID)
	}
	if req.Method != "tools/list" {
		t.Errorf("expected method tools/list, got %q", req.Method)
	}
	if req.Params != nil {
		t.Errorf("expected nil params, got %v", req.Params)
	}
}

func TestNewNotification(t *testing.T) {
	n := newNotification("notifications/initialized", nil)
	if n.ID != nil {
		t.Errorf("notification should have nil ID, got %v", n.ID)
	}
	if n.Method != "notifications/initialized" {
		t.Errorf("expected notifications/initialized, got %q", n.Method)
	}
}

func TestRequestMarshalRoundTrip(t *testing.T) {
	req := newRequest(42, "tools/call", ToolCallParams{
		Name:      "search",
		Arguments: map[string]any{"query": "test"},
	})
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}

	var decoded JSONRPCRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.JSONRPC != "2.0" {
		t.Errorf("jsonrpc: got %q", decoded.JSONRPC)
	}
	if decoded.ID == nil || *decoded.ID != 42 {
		t.Errorf("id: got %v", decoded.ID)
	}
	if decoded.Method != "tools/call" {
		t.Errorf("method: got %q", decoded.Method)
	}
}

func TestNotificationMarshalOmitsID(t *testing.T) {
	n := newNotification("notifications/initialized", nil)
	data, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if _, hasID := raw["id"]; hasID {
		t.Error("notification should not have 'id' field in JSON")
	}
}

func TestResponseUnmarshalSuccess(t *testing.T) {
	raw := `{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"search"}]}}`
	var resp JSONRPCResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.ID != 1 {
		t.Errorf("expected id 1, got %d", resp.ID)
	}
	if resp.Error != nil {
		t.Error("expected no error")
	}
	if resp.Result == nil {
		t.Fatal("expected result")
	}

	var toolsList ToolsListResult
	if err := json.Unmarshal(resp.Result, &toolsList); err != nil {
		t.Fatal(err)
	}
	if len(toolsList.Tools) != 1 || toolsList.Tools[0].Name != "search" {
		t.Errorf("unexpected tools: %+v", toolsList)
	}
}

func TestResponseUnmarshalError(t *testing.T) {
	raw := `{"jsonrpc":"2.0","id":5,"error":{"code":-32601,"message":"Method not found"}}`
	var resp JSONRPCResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.ID != 5 {
		t.Errorf("expected id 5, got %d", resp.ID)
	}
	if resp.Error == nil {
		t.Fatal("expected error")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("expected code -32601, got %d", resp.Error.Code)
	}
	if resp.Error.Message != "Method not found" {
		t.Errorf("expected 'Method not found', got %q", resp.Error.Message)
	}
	if resp.Error.Error() != "Method not found" {
		t.Errorf("Error() should return message, got %q", resp.Error.Error())
	}
}

func TestResponseUnmarshalErrorWithData(t *testing.T) {
	raw := `{"jsonrpc":"2.0","id":3,"error":{"code":-32602,"message":"Invalid params","data":"details here"}}`
	var resp JSONRPCResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil {
		t.Fatal("expected error")
	}
	if resp.Error.Data == nil {
		t.Error("expected error data")
	}
}

func TestRequestWithNilParams(t *testing.T) {
	req := newRequest(1, "tools/list", nil)
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if _, hasParams := raw["params"]; hasParams {
		t.Error("nil params should be omitted from JSON")
	}
}

func TestRequestWithParams(t *testing.T) {
	req := newRequest(2, "tools/call", map[string]string{"name": "test"})
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if _, hasParams := raw["params"]; !hasParams {
		t.Error("expected params in JSON")
	}
}
