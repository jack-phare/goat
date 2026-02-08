package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/jg-phare/goat/pkg/types"
)

// ServerConnection manages the lifecycle and state of a single MCP server.
type ServerConnection struct {
	Name         string
	Config       types.McpServerConfig
	Status       ConnectionStatus
	Info         *ServerInfo
	Capabilities *ServerCapabilities
	Tools        []ToolInfo
	Resources    []Resource
	Enabled      bool
	Transport    Transport
	ErrorMsg     string

	mu    sync.Mutex
	nextID atomic.Int32
}

// newServerConnection creates a new connection in pending state.
func newServerConnection(name string, config types.McpServerConfig) *ServerConnection {
	return &ServerConnection{
		Name:    name,
		Config:  config,
		Status:  StatusPending,
		Enabled: true,
	}
}

// connect creates the transport and runs the MCP initialization handshake.
func (sc *ServerConnection) connect(ctx context.Context) error {
	sc.mu.Lock()
	transport, err := sc.createTransport()
	if err != nil {
		sc.Status = StatusFailed
		sc.ErrorMsg = err.Error()
		sc.mu.Unlock()
		return fmt.Errorf("create transport: %w", err)
	}
	sc.Transport = transport
	sc.mu.Unlock()

	return sc.runHandshake(ctx)
}

// runHandshake performs the MCP initialization handshake on an already-connected transport.
// This is separated from connect() to allow testing with injected mock transports.
func (sc *ServerConnection) runHandshake(ctx context.Context) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	transport := sc.Transport

	// 1. Initialize handshake
	initParams := InitializeParams{
		ProtocolVersion: "2024-11-05",
		Capabilities:    ClientCapabilities{},
		ClientInfo:      ClientInfo{Name: "goat", Version: "0.1.0"},
	}
	resp, err := transport.Send(ctx, newRequest(sc.nextRequestID(), MethodInitialize, initParams))
	if err != nil {
		sc.Status = StatusFailed
		sc.ErrorMsg = err.Error()
		transport.Close()
		sc.Transport = nil
		return fmt.Errorf("initialize: %w", err)
	}
	if resp.Error != nil {
		sc.Status = StatusFailed
		sc.ErrorMsg = resp.Error.Message
		transport.Close()
		sc.Transport = nil
		return fmt.Errorf("initialize error: %s", resp.Error.Message)
	}

	var initResult InitializeResult
	if err := json.Unmarshal(resp.Result, &initResult); err != nil {
		sc.Status = StatusFailed
		sc.ErrorMsg = err.Error()
		transport.Close()
		sc.Transport = nil
		return fmt.Errorf("parse initialize result: %w", err)
	}

	sc.Info = &initResult.ServerInfo
	sc.Capabilities = &initResult.Capabilities

	// 2. Send initialized notification
	if err := transport.Notify(ctx, MethodInitialized, nil); err != nil {
		sc.Status = StatusFailed
		sc.ErrorMsg = err.Error()
		transport.Close()
		sc.Transport = nil
		return fmt.Errorf("send initialized: %w", err)
	}

	// 3. List tools if server supports them
	if sc.Capabilities.Tools != nil {
		tools, err := sc.listTools(ctx)
		if err != nil {
			sc.Status = StatusFailed
			sc.ErrorMsg = err.Error()
			transport.Close()
			sc.Transport = nil
			return fmt.Errorf("list tools: %w", err)
		}
		sc.Tools = tools
	}

	// 4. List resources if server supports them
	if sc.Capabilities.Resources != nil {
		resources, err := sc.listResources(ctx)
		if err != nil {
			// Non-fatal: tools may still work
			sc.Resources = nil
		} else {
			sc.Resources = resources
		}
	}

	sc.Status = StatusConnected
	sc.ErrorMsg = ""
	return nil
}

// disconnect closes the transport and resets state.
func (sc *ServerConnection) disconnect() error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.Transport != nil {
		err := sc.Transport.Close()
		sc.Transport = nil
		sc.Tools = nil
		sc.Resources = nil
		sc.Info = nil
		sc.Capabilities = nil
		sc.Status = StatusPending
		sc.ErrorMsg = ""
		return err
	}
	return nil
}

// callTool executes a tool call via the transport.
func (sc *ServerConnection) callTool(ctx context.Context, name string, args map[string]any) (*ToolResult, error) {
	sc.mu.Lock()
	transport := sc.Transport
	sc.mu.Unlock()

	if transport == nil {
		return nil, fmt.Errorf("not connected")
	}

	resp, err := transport.Send(ctx, newRequest(sc.nextRequestID(), MethodToolsCall, ToolCallParams{
		Name:      name,
		Arguments: args,
	}))
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, resp.Error
	}

	var result ToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse tool result: %w", err)
	}
	return &result, nil
}

// readResource reads a resource via the transport.
func (sc *ServerConnection) readResource(ctx context.Context, uri string) (*ResourceReadResult, error) {
	sc.mu.Lock()
	transport := sc.Transport
	sc.mu.Unlock()

	if transport == nil {
		return nil, fmt.Errorf("not connected")
	}

	resp, err := transport.Send(ctx, newRequest(sc.nextRequestID(), MethodResourcesRead, ResourceReadParams{URI: uri}))
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, resp.Error
	}

	var result ResourceReadResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse resource result: %w", err)
	}
	return &result, nil
}

func (sc *ServerConnection) listTools(ctx context.Context) ([]ToolInfo, error) {
	resp, err := sc.Transport.Send(ctx, newRequest(sc.nextRequestID(), MethodToolsList, nil))
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, resp.Error
	}

	var result ToolsListResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, err
	}
	return result.Tools, nil
}

func (sc *ServerConnection) listResources(ctx context.Context) ([]Resource, error) {
	resp, err := sc.Transport.Send(ctx, newRequest(sc.nextRequestID(), MethodResourcesList, nil))
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, resp.Error
	}

	var result ResourcesListResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, err
	}
	return result.Resources, nil
}

func (sc *ServerConnection) createTransport() (Transport, error) {
	switch sc.Config.Type {
	case TransportStdio, "":
		if sc.Config.Command == "" {
			return nil, fmt.Errorf("stdio transport requires a command")
		}
		return NewStdioTransport(sc.Config.Command, sc.Config.Args, sc.Config.Env)
	case TransportHTTP, "sse":
		if sc.Config.URL == "" {
			return nil, fmt.Errorf("http transport requires a URL")
		}
		return NewHTTPTransport(sc.Config.URL, sc.Config.Headers), nil
	default:
		return nil, fmt.Errorf("unsupported transport type: %q", sc.Config.Type)
	}
}

func (sc *ServerConnection) nextRequestID() int {
	return int(sc.nextID.Add(1))
}

func (sc *ServerConnection) status() ServerStatus {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	return ServerStatus{
		Name:       sc.Name,
		Status:     sc.Status,
		ServerInfo: sc.Info,
		Error:      sc.ErrorMsg,
		Tools:      sc.Tools,
	}
}
