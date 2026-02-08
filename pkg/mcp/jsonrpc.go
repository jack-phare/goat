package mcp

import "encoding/json"

// JSONRPCRequest is a JSON-RPC 2.0 request message.
type JSONRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int   `json:"id,omitempty"` // nil for notifications
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// JSONRPCResponse is a JSON-RPC 2.0 response message.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError is the error object in a JSON-RPC 2.0 response.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *JSONRPCError) Error() string { return e.Message }

// newRequest creates a JSON-RPC 2.0 request with the given ID, method, and params.
func newRequest(id int, method string, params any) JSONRPCRequest {
	return JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  method,
		Params:  params,
	}
}

// newNotification creates a JSON-RPC 2.0 notification (no ID, no response expected).
func newNotification(method string, params any) JSONRPCRequest {
	return JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
}
