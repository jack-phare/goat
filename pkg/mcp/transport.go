package mcp

import "context"

// Transport abstracts bidirectional JSON-RPC communication with an MCP server.
type Transport interface {
	// Send sends a JSON-RPC request and returns the correlated response.
	Send(ctx context.Context, req JSONRPCRequest) (JSONRPCResponse, error)
	// Notify sends a JSON-RPC notification (no response expected).
	Notify(ctx context.Context, method string, params any) error
	// Close terminates the transport connection.
	Close() error
}
