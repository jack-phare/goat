package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jg-phare/goat/pkg/tools"
	"github.com/jg-phare/goat/pkg/types"
)

// Client manages MCP server connections and implements tools.MCPClient.
type Client struct {
	mu       sync.RWMutex
	servers  map[string]*ServerConnection
	registry *tools.Registry
}

// NewClient creates a new MCP client that will register discovered tools in the given registry.
func NewClient(registry *tools.Registry) *Client {
	return &Client{
		servers:  make(map[string]*ServerConnection),
		registry: registry,
	}
}

// Connect establishes a connection to an MCP server and registers its tools.
func (c *Client) Connect(ctx context.Context, name string, config types.McpServerConfig) error {
	conn := newServerConnection(name, config)

	if err := conn.connect(ctx); err != nil {
		c.mu.Lock()
		c.servers[name] = conn // store even failed connections for status reporting
		c.mu.Unlock()
		return err
	}

	c.mu.Lock()
	c.servers[name] = conn
	c.mu.Unlock()

	// Wire up notification handler for tool list changes
	conn.mu.Lock()
	if conn.Transport != nil {
		serverName := name
		conn.Transport.SetNotificationHandler(func(method string, params json.RawMessage) {
			if method == "notifications/tools/list_changed" {
				c.handleToolListChanged(serverName)
			}
		})
	}
	conn.mu.Unlock()

	// Register tools in the registry
	c.registerTools(name, conn.Tools)

	return nil
}

// Disconnect removes a server connection and unregisters its tools.
func (c *Client) Disconnect(name string) error {
	c.mu.Lock()
	conn, ok := c.servers[name]
	if !ok {
		c.mu.Unlock()
		return fmt.Errorf("unknown server: %q", name)
	}
	delete(c.servers, name)
	c.mu.Unlock()

	c.registry.UnregisterMCPTools(name)
	return conn.disconnect()
}

// Reconnect disconnects and reconnects a server.
func (c *Client) Reconnect(ctx context.Context, name string) error {
	c.mu.RLock()
	conn, ok := c.servers[name]
	c.mu.RUnlock()

	if !ok {
		return fmt.Errorf("unknown server: %q", name)
	}

	config := conn.Config
	c.registry.UnregisterMCPTools(name)
	conn.disconnect()

	// Reconnect
	return c.Connect(ctx, name, config)
}

// Toggle enables or disables a server. Disabled servers have their tools unregistered.
func (c *Client) Toggle(name string, enabled bool) error {
	c.mu.RLock()
	conn, ok := c.servers[name]
	c.mu.RUnlock()

	if !ok {
		return fmt.Errorf("unknown server: %q", name)
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()

	if conn.Enabled == enabled {
		return nil // no-op
	}
	conn.Enabled = enabled

	if !enabled {
		c.registry.UnregisterMCPTools(name)
		conn.Status = StatusDisabled
	} else {
		conn.Status = StatusConnected
		// Re-register tools
		c.registerTools(name, conn.Tools)
	}

	return nil
}

// SetServers performs a bulk update: adds new servers, removes old ones, keeps unchanged.
func (c *Client) SetServers(ctx context.Context, servers map[string]types.McpServerConfig) *SetServersResult {
	result := &SetServersResult{
		Errors: make(map[string]string),
	}

	c.mu.RLock()
	existing := make(map[string]bool)
	for name := range c.servers {
		existing[name] = true
	}
	c.mu.RUnlock()

	// Determine what to add and remove
	desired := make(map[string]bool)
	for name := range servers {
		desired[name] = true
	}

	// Remove servers not in desired set
	for name := range existing {
		if !desired[name] {
			if err := c.Disconnect(name); err != nil {
				result.Errors[name] = err.Error()
			} else {
				result.Removed = append(result.Removed, name)
			}
		}
	}

	// Add servers not in existing set
	for name, config := range servers {
		if !existing[name] {
			if err := c.Connect(ctx, name, config); err != nil {
				result.Errors[name] = err.Error()
			} else {
				result.Added = append(result.Added, name)
			}
		}
	}

	// Check for config changes on existing servers that are still in desired set
	for name, newConfig := range servers {
		if existing[name] {
			c.mu.RLock()
			conn := c.servers[name]
			c.mu.RUnlock()
			if conn != nil && !configEqual(conn.Config, newConfig) {
				// Config changed — reconnect with new config
				if err := c.Disconnect(name); err != nil {
					result.Errors[name] = err.Error()
					continue
				}
				if err := c.Connect(ctx, name, newConfig); err != nil {
					result.Errors[name] = err.Error()
				} else {
					result.Updated = append(result.Updated, name)
				}
			}
		}
	}

	return result
}

// Status returns the status of all server connections.
func (c *Client) Status() []ServerStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	statuses := make([]ServerStatus, 0, len(c.servers))
	for _, conn := range c.servers {
		statuses = append(statuses, conn.status())
	}
	return statuses
}

// ServerStatus returns the status of a specific server.
func (c *Client) ServerStatus(name string) (*ServerStatus, error) {
	c.mu.RLock()
	conn, ok := c.servers[name]
	c.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unknown server: %q", name)
	}

	s := conn.status()
	return &s, nil
}

// ListResources implements tools.MCPClient.
func (c *Client) ListResources(ctx context.Context, serverName string) ([]tools.MCPResource, error) {
	c.mu.RLock()
	conn, ok := c.servers[serverName]
	c.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unknown server: %q", serverName)
	}

	conn.mu.Lock()
	resources := conn.Resources
	conn.mu.Unlock()

	result := make([]tools.MCPResource, len(resources))
	for i, r := range resources {
		result[i] = tools.MCPResource{
			URI:         r.URI,
			Name:        r.Name,
			Description: r.Description,
			MimeType:    r.MimeType,
		}
	}
	return result, nil
}

// ReadResource implements tools.MCPClient.
func (c *Client) ReadResource(ctx context.Context, serverName, uri string) (tools.MCPResourceContent, error) {
	c.mu.RLock()
	conn, ok := c.servers[serverName]
	c.mu.RUnlock()

	if !ok {
		return tools.MCPResourceContent{}, fmt.Errorf("unknown server: %q", serverName)
	}

	result, err := conn.readResource(ctx, uri)
	if err != nil {
		return tools.MCPResourceContent{}, err
	}

	if len(result.Contents) == 0 {
		return tools.MCPResourceContent{URI: uri}, nil
	}

	rc := result.Contents[0]
	return tools.MCPResourceContent{
		URI:      rc.URI,
		MimeType: rc.MimeType,
		Text:     rc.Text,
		Blob:     rc.Blob,
	}, nil
}

// CallTool implements tools.MCPClient.
// If the transport reports a connection error, CallTool attempts auto-reconnection
// with exponential backoff before retrying the call once.
func (c *Client) CallTool(ctx context.Context, serverName, toolName string, args map[string]any) (tools.MCPToolCallResult, error) {
	c.mu.RLock()
	conn, ok := c.servers[serverName]
	c.mu.RUnlock()

	if !ok {
		return tools.MCPToolCallResult{}, fmt.Errorf("unknown server: %q", serverName)
	}

	result, err := conn.callTool(ctx, toolName, args)
	if err != nil {
		// Check if this is a transport-level failure worth reconnecting for
		if isTransportError(err) {
			if reconnErr := c.reconnectWithBackoff(ctx, serverName, 3); reconnErr == nil {
				// Retry once after successful reconnect
				result, err = conn.callTool(ctx, toolName, args)
				if err != nil {
					return tools.MCPToolCallResult{}, err
				}
			} else {
				return tools.MCPToolCallResult{}, fmt.Errorf("tool call failed and reconnect failed: %w", err)
			}
		} else {
			return tools.MCPToolCallResult{}, err
		}
	}

	blocks := make([]tools.MCPContentBlock, len(result.Content))
	for i, cb := range result.Content {
		blocks[i] = tools.MCPContentBlock{
			Type:     cb.Type,
			Text:     cb.Text,
			MimeType: cb.MimeType,
			Data:     cb.Data,
			URI:      cb.URI,
		}
	}
	return tools.MCPToolCallResult{
		Content: blocks,
		IsError: result.IsError,
	}, nil
}

// Ping sends a health check ping to a connected MCP server.
// Returns nil if the server responds, or an error if unreachable.
func (c *Client) Ping(ctx context.Context, name string) error {
	c.mu.RLock()
	conn, ok := c.servers[name]
	c.mu.RUnlock()
	if !ok {
		return fmt.Errorf("unknown server: %q", name)
	}

	conn.mu.Lock()
	transport := conn.Transport
	conn.mu.Unlock()
	if transport == nil {
		return fmt.Errorf("not connected")
	}

	_, err := transport.Send(ctx, newRequest(conn.nextRequestID(), "ping", nil))
	return err
}

// Close disconnects all servers.
func (c *Client) Close() error {
	c.mu.Lock()
	names := make([]string, 0, len(c.servers))
	for name := range c.servers {
		names = append(names, name)
	}
	c.mu.Unlock()

	var errs []string
	for _, name := range names {
		if err := c.Disconnect(name); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// reconnectWithBackoff attempts to reconnect to a server with exponential backoff.
func (c *Client) reconnectWithBackoff(ctx context.Context, name string, maxAttempts int) error {
	backoff := 1 * time.Second
	for attempt := 0; attempt < maxAttempts; attempt++ {
		err := c.Reconnect(ctx, name)
		if err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
	}
	return fmt.Errorf("reconnect failed after %d attempts", maxAttempts)
}

// configEqual compares two McpServerConfig values for equality.
func configEqual(a, b types.McpServerConfig) bool {
	if a.Type != b.Type || a.Command != b.Command || a.URL != b.URL {
		return false
	}
	if len(a.Args) != len(b.Args) {
		return false
	}
	for i := range a.Args {
		if a.Args[i] != b.Args[i] {
			return false
		}
	}
	if len(a.Env) != len(b.Env) {
		return false
	}
	for k, v := range a.Env {
		if b.Env[k] != v {
			return false
		}
	}
	if len(a.Headers) != len(b.Headers) {
		return false
	}
	for k, v := range a.Headers {
		if b.Headers[k] != v {
			return false
		}
	}
	return true
}

// isTransportError checks if an error indicates a transport-level failure
// (disconnection, write error, etc.) as opposed to an application-level error.
func isTransportError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "not connected") ||
		strings.Contains(msg, "transport closed") ||
		strings.Contains(msg, "write to stdin") ||
		strings.Contains(msg, "connection lost") ||
		strings.Contains(msg, "broken pipe")
}

// handleToolListChanged re-fetches and re-registers tools when a server
// sends a notifications/tools/list_changed notification.
func (c *Client) handleToolListChanged(name string) {
	c.mu.RLock()
	conn, ok := c.servers[name]
	c.mu.RUnlock()
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn.mu.Lock()
	transport := conn.Transport
	conn.mu.Unlock()
	if transport == nil {
		return
	}

	tools, err := conn.listTools(ctx)
	if err != nil {
		return // keep old tools
	}

	// Re-register tools atomically
	c.registry.UnregisterMCPTools(name)
	conn.mu.Lock()
	conn.Tools = tools
	conn.mu.Unlock()
	c.registerTools(name, tools)
}

// registerTools registers MCP tools in the tool registry.
func (c *Client) registerTools(serverName string, mcpTools []ToolInfo) {
	for _, t := range mcpTools {
		var schema map[string]any
		if t.InputSchema != nil {
			json.Unmarshal(t.InputSchema, &schema)
		}
		var annotations *tools.MCPToolAnnotations
		if t.Annotations != nil {
			annotations = &tools.MCPToolAnnotations{
				ReadOnly:    t.Annotations.ReadOnly,
				Destructive: t.Annotations.Destructive,
				OpenWorld:   t.Annotations.OpenWorld,
			}
		}
		c.registry.RegisterMCPTool(serverName, t.Name, t.Description, schema, c, annotations)
	}
}
