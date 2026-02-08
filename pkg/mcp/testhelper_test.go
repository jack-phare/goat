package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// mockTransport implements Transport with pre-programmed responses.
type mockTransport struct {
	mu        sync.Mutex
	responses map[string]json.RawMessage // method → result JSON
	closed    bool
	notified  []string // methods that were notified
}

func newMockTransport() *mockTransport {
	return &mockTransport{
		responses: make(map[string]json.RawMessage),
	}
}

// withInitialize configures the mock to respond to initialize with the given capabilities.
func (m *mockTransport) withInitialize(caps ServerCapabilities) *mockTransport {
	result := InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities:    caps,
		ServerInfo:      ServerInfo{Name: "mock-server", Version: "1.0"},
	}
	data, _ := json.Marshal(result)
	m.responses[MethodInitialize] = data
	return m
}

// withTools configures the mock to respond to tools/list with the given tools.
func (m *mockTransport) withTools(tools []ToolInfo) *mockTransport {
	result := ToolsListResult{Tools: tools}
	data, _ := json.Marshal(result)
	m.responses[MethodToolsList] = data
	return m
}

// withResources configures the mock to respond to resources/list with the given resources.
func (m *mockTransport) withResources(resources []Resource) *mockTransport {
	result := ResourcesListResult{Resources: resources}
	data, _ := json.Marshal(result)
	m.responses[MethodResourcesList] = data
	return m
}

// withToolCall configures the mock to respond to tools/call with the given result.
func (m *mockTransport) withToolCall(toolResult ToolResult) *mockTransport {
	data, _ := json.Marshal(toolResult)
	m.responses[MethodToolsCall] = data
	return m
}

// withResourceRead configures the mock to respond to resources/read.
func (m *mockTransport) withResourceRead(result ResourceReadResult) *mockTransport {
	data, _ := json.Marshal(result)
	m.responses[MethodResourcesRead] = data
	return m
}

// withResponse configures a raw response for any method.
func (m *mockTransport) withResponse(method string, result json.RawMessage) *mockTransport {
	m.responses[method] = result
	return m
}

func (m *mockTransport) Send(_ context.Context, req JSONRPCRequest) (JSONRPCResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return JSONRPCResponse{}, fmt.Errorf("transport closed")
	}

	result, ok := m.responses[req.Method]
	if !ok {
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      *req.ID,
			Error:   &JSONRPCError{Code: -32601, Message: "Method not found: " + req.Method},
		}, nil
	}

	id := 0
	if req.ID != nil {
		id = *req.ID
	}

	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}, nil
}

func (m *mockTransport) Notify(_ context.Context, method string, _ any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return fmt.Errorf("transport closed")
	}
	m.notified = append(m.notified, method)
	return nil
}

func (m *mockTransport) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.closed = true
	return nil
}
