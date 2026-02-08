# MCP Integration Implementation Plan (Spec 10)

## Overview

Implement the MCP (Model Context Protocol) client in `pkg/mcp/` that manages connections to external MCP servers, handles JSON-RPC 2.0 communication, and bridges to the existing tool registry. Two transport types: **Stdio** (process spawn with stdin/stdout pipes) and **Streamable HTTP** (HTTP POST with optional SSE responses). Server notifications deferred to follow-up.

## Current State

- **`pkg/tools/mcp.go:17-37`** — `MCPClient` interface (3 string-returning methods) + `StubMCPClient`
- **`pkg/tools/mcptool.go:9-73`** — `MCPTool` dynamic tool + `RegisterMCPTool`/`UnregisterMCPTools` on Registry
- **`pkg/types/mcp.go:1-21`** — `McpServerConfig` union (stdio/sse/http/sdk/claudeai-proxy fields)
- **`pkg/agent/config.go:55`** — `MCPServers map[string]types.McpServerConfig` on AgentConfig
- **`pkg/agent/defaults.go:43-46`** — ListMcpResourcesTool + ReadMcpResourceTool registered with nil Client
- **`pkg/permission/risk.go:17-18,41-47`** — ListMcpResources/ReadMcpResource → RiskLow; mcp__* → RiskHigh
- **`pkg/prompt/assembler.go:73-75`** — Conditional MCP system prompt inclusion
- **13 existing MCP tool tests** — All passing

## Desired End State

- `pkg/mcp/` package with `Client` implementing `tools.MCPClient`
- JSON-RPC 2.0 protocol layer with request ID correlation
- Stdio transport: process lifecycle (spawn, pipe, kill)
- HTTP transport: Streamable HTTP (POST JSON-RPC, response may be JSON or SSE)
- Full connection lifecycle: connect → initialize → capabilities → list tools → register → ready
- Server capability gating: only call tools/list, resources/list if server declares support
- `SetServers` for bulk add/remove with diffing
- Toggle, reconnect, disconnect operations
- Structured return types (`MCPToolResult`, `MCPResourceContent`) replacing string returns
- ~80+ tests covering protocol, transports, lifecycle, and integration

## Import Graph

```
pkg/mcp/ imports:
  - pkg/tools/ (MCPClient interface, MCPResource, Registry for dynamic tool registration)
  - pkg/types/ (McpServerConfig)
  - Standard library: context, encoding/json, sync, net/http, os/exec, bufio, io, fmt, strings
```

No import cycles: `mcp` depends on `tools` and `types` only. The agent loop wires `mcp.Client` into tools at startup.

---

## Phases

### Phase 1: JSON-RPC 2.0 Protocol Layer + Types

**Goal**: Build the JSON-RPC message types, ID-correlated request/response handling, and all MCP-specific types needed by both transports.

**New Files**:

1. **`pkg/mcp/jsonrpc.go`** — JSON-RPC 2.0 types and helpers:
   ```go
   type JSONRPCRequest struct {
       JSONRPC string `json:"jsonrpc"`
       ID      int    `json:"id"`
       Method  string `json:"method"`
       Params  any    `json:"params,omitempty"`
   }

   type JSONRPCResponse struct {
       JSONRPC string          `json:"jsonrpc"`
       ID      int             `json:"id"`
       Result  json.RawMessage `json:"result,omitempty"`
       Error   *JSONRPCError   `json:"error,omitempty"`
   }

   type JSONRPCError struct {
       Code    int    `json:"code"`
       Message string `json:"message"`
       Data    any    `json:"data,omitempty"`
   }
   ```
   Helper: `func newRequest(id int, method string, params any) JSONRPCRequest`

2. **`pkg/mcp/types.go`** — MCP protocol types:
   ```go
   // Transport types
   const (
       TransportStdio = "stdio"
       TransportHTTP  = "http"
   )

   // Connection statuses
   type ConnectionStatus string
   const (
       StatusConnected ConnectionStatus = "connected"
       StatusFailed    ConnectionStatus = "failed"
       StatusNeedsAuth ConnectionStatus = "needs-auth"
       StatusPending   ConnectionStatus = "pending"
       StatusDisabled  ConnectionStatus = "disabled"
   )

   // Server info from initialize handshake
   type ServerInfo struct {
       Name    string `json:"name"`
       Version string `json:"version"`
   }

   // Capabilities exchanged during initialize
   type ClientCapabilities struct {
       // What the client supports (we send this)
   }
   type ServerCapabilities struct {
       Tools     *ToolsCapability     `json:"tools,omitempty"`
       Resources *ResourcesCapability `json:"resources,omitempty"`
       Prompts   *PromptsCapability   `json:"prompts,omitempty"`
   }
   type ToolsCapability struct {
       ListChanged bool `json:"listChanged,omitempty"`
   }
   type ResourcesCapability struct {
       Subscribe   bool `json:"subscribe,omitempty"`
       ListChanged bool `json:"listChanged,omitempty"`
   }
   type PromptsCapability struct {
       ListChanged bool `json:"listChanged,omitempty"`
   }

   // Initialize request/response
   type InitializeParams struct {
       ProtocolVersion string             `json:"protocolVersion"`
       Capabilities    ClientCapabilities `json:"capabilities"`
       ClientInfo      ClientInfo         `json:"clientInfo"`
   }
   type ClientInfo struct {
       Name    string `json:"name"`
       Version string `json:"version"`
   }
   type InitializeResult struct {
       ProtocolVersion  string             `json:"protocolVersion"`
       Capabilities     ServerCapabilities `json:"capabilities"`
       ServerInfo       ServerInfo         `json:"serverInfo"`
   }

   // Tool types
   type ToolInfo struct {
       Name        string          `json:"name"`
       Description string          `json:"description,omitempty"`
       InputSchema json.RawMessage `json:"inputSchema,omitempty"`
       Annotations *ToolAnnotations `json:"annotations,omitempty"`
   }
   type ToolAnnotations struct {
       ReadOnly    *bool `json:"readOnly,omitempty"`
       Destructive *bool `json:"destructive,omitempty"`
       OpenWorld   *bool `json:"openWorld,omitempty"`
   }
   type ToolsListResult struct {
       Tools []ToolInfo `json:"tools"`
   }
   type ToolCallParams struct {
       Name      string         `json:"name"`
       Arguments map[string]any `json:"arguments,omitempty"`
   }
   type ToolResult struct {
       Content []ContentBlock `json:"content"`
       IsError bool           `json:"isError,omitempty"`
   }
   type ContentBlock struct {
       Type     string `json:"type"` // "text", "image", "resource"
       Text     string `json:"text,omitempty"`
       MimeType string `json:"mimeType,omitempty"`
       Data     string `json:"data,omitempty"` // base64 for image
       URI      string `json:"uri,omitempty"`  // for resource
   }

   // Resource types
   type Resource struct {
       URI         string `json:"uri"`
       Name        string `json:"name"`
       Description string `json:"description,omitempty"`
       MimeType    string `json:"mimeType,omitempty"`
   }
   type ResourcesListResult struct {
       Resources []Resource `json:"resources"`
   }
   type ResourceReadParams struct {
       URI string `json:"uri"`
   }
   type ResourceReadResult struct {
       Contents []ResourceContent `json:"contents"`
   }
   type ResourceContent struct {
       URI      string `json:"uri"`
       MimeType string `json:"mimeType,omitempty"`
       Text     string `json:"text,omitempty"`
       Blob     string `json:"blob,omitempty"` // base64
   }

   // MCP method constants
   const (
       MethodInitialize    = "initialize"
       MethodInitialized   = "notifications/initialized" // notification after init
       MethodToolsList     = "tools/list"
       MethodToolsCall     = "tools/call"
       MethodResourcesList = "resources/list"
       MethodResourcesRead = "resources/read"
   )
   ```

3. **`pkg/mcp/jsonrpc_test.go`** — Tests:
   - Marshaling/unmarshaling of requests and responses
   - Error response handling
   - ID correlation helper
   - Nil params handling

4. **`pkg/mcp/types_test.go`** — Tests:
   - InitializeResult unmarshaling with various capability combos
   - ToolInfo schema parsing
   - ContentBlock type switching

**Success Criteria**:
- Automated:
  - [x] `go vet ./pkg/mcp/...` passes
  - [x] `go test ./pkg/mcp/... -race` passes
  - [x] `go build ./...` passes

---

### Phase 2: Transport Interface + Stdio Transport

**Goal**: Define the `Transport` interface and implement the Stdio transport (process spawn, stdin/stdout JSON-RPC, process lifecycle).

**New Files**:

1. **`pkg/mcp/transport.go`** — Transport interface:
   ```go
   // Transport abstracts bidirectional JSON-RPC communication with an MCP server.
   type Transport interface {
       // Send sends a JSON-RPC request and returns the correlated response.
       Send(ctx context.Context, req JSONRPCRequest) (JSONRPCResponse, error)
       // Notify sends a JSON-RPC notification (no response expected).
       Notify(ctx context.Context, method string, params any) error
       // Close terminates the transport connection.
       Close() error
   }
   ```

2. **`pkg/mcp/stdio.go`** — Stdio transport:
   ```go
   type StdioTransport struct {
       cmd    *exec.Cmd
       stdin  io.WriteCloser
       stdout *bufio.Scanner
       mu     sync.Mutex    // serializes writes to stdin
       nextID int
       pending map[int]chan JSONRPCResponse // ID → response channel
       pendMu  sync.Mutex
       done   chan struct{}
   }

   func NewStdioTransport(command string, args []string, env map[string]string) (*StdioTransport, error)
   ```
   Key behaviors:
   - `exec.Command` with `StdinPipe()` and `StdoutPipe()`; stderr captured to buffer for error reporting
   - Inherits parent env + user-specified env vars (`cmd.Env = append(os.Environ(), ...)`  )
   - Background goroutine reads stdout line-by-line, dispatches responses to `pending[id]` channels
   - `Send()` writes JSON + `\n` to stdin, creates a pending channel, waits for response (with context timeout)
   - `Notify()` writes JSON + `\n` (no ID field, no pending channel)
   - `Close()` closes stdin, sends SIGTERM, waits with timeout, then SIGKILL
   - All operations respect context cancellation

3. **`pkg/mcp/stdio_test.go`** — Tests:
   - Start/stop lifecycle (mock command via `echo` or small Go test helper)
   - Send request, receive response (use a test helper script that echoes JSON-RPC)
   - Concurrent sends with different IDs
   - Timeout handling
   - Process crash recovery (early EOF)
   - Close terminates process
   - Environment variable passing
   - Large message handling (beyond default scanner buffer)

**Implementation Notes**:
- Follow the `hooks/shell.go` pattern for `exec.CommandContext`
- Follow the `llm/sse.go` pattern for the background reader goroutine (channel dispatch)
- Use `bufio.NewScanner(stdout)` with enlarged buffer (`scanner.Buffer(buf, 1024*1024)`) for large JSON payloads
- Notification JSON-RPC messages have no `id` field — use `json.RawMessage` or omit via pointer

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/mcp/... -race -run TestStdio` passes
  - [x] Process cleanup verified (no leaked processes)
  - [x] Concurrent send safety verified with -race

---

### Phase 3: HTTP Transport (Streamable HTTP)

**Goal**: Implement the Streamable HTTP transport per the MCP 2025-03-26 spec. Each JSON-RPC request is sent as HTTP POST; the response may be immediate JSON or an SSE stream.

**New Files**:

1. **`pkg/mcp/http.go`** — HTTP transport:
   ```go
   type HTTPTransport struct {
       url     string
       headers map[string]string
       client  *http.Client
       mu      sync.Mutex
   }

   func NewHTTPTransport(url string, headers map[string]string) *HTTPTransport
   ```
   Key behaviors:
   - `Send()`: POST JSON-RPC to URL with `Content-Type: application/json`, `Accept: application/json, text/event-stream`
   - If response `Content-Type` is `application/json`: unmarshal single JSONRPCResponse
   - If response `Content-Type` is `text/event-stream`: parse SSE stream, extract JSON-RPC response from `data:` lines, match by ID
   - `Notify()`: POST with no response body expected
   - `Close()`: no-op (HTTP is stateless per-request)
   - Custom headers (auth tokens, etc.) passed on every request
   - Session ID tracking via `Mcp-Session-Id` response header (stored and sent on subsequent requests)

2. **`pkg/mcp/http_test.go`** — Tests using `httptest.NewServer`:
   - Simple JSON response
   - SSE streamed response
   - Session ID tracking
   - Auth header passing
   - HTTP error codes (401, 500)
   - Context cancellation during SSE read
   - Large response handling

**Implementation Notes**:
- Reuse the SSE line-by-line parsing pattern from `llm/sse.go` but adapted for JSON-RPC (no `[DONE]` sentinel; stream ends on connection close or final response)
- `http.Client` injected or use `http.DefaultClient` with reasonable timeout
- No retry logic in transport layer (retry handled by Client if needed)

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/mcp/... -race -run TestHTTP` passes
  - [x] httptest-based tests (no external network)
  - [x] Both JSON and SSE response paths covered

---

### Phase 4: Server Connection + Client

**Goal**: Build the `ServerConnection` (per-server state) and `Client` (connection manager that implements `tools.MCPClient`).

**New Files**:

1. **`pkg/mcp/connection.go`** — Per-server connection:
   ```go
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
       mu           sync.Mutex
   }
   ```
   Methods:
   - `connect(ctx context.Context) error` — create transport based on config.Type, run initialize handshake, send `notifications/initialized`, list tools (if capability), list resources (if capability)
   - `disconnect() error` — close transport, clear state
   - `callTool(ctx, name string, args map[string]any) (*ToolResult, error)` — delegate to transport
   - `listResources(ctx) ([]Resource, error)` — delegate to transport
   - `readResource(ctx, uri string) (*ResourceReadResult, error)` — delegate to transport

2. **`pkg/mcp/client.go`** — Client (implements `tools.MCPClient`):
   ```go
   type Client struct {
       mu       sync.RWMutex
       servers  map[string]*ServerConnection
       registry *tools.Registry
   }

   func NewClient(registry *tools.Registry) *Client
   ```
   Methods:
   - `Connect(ctx, name, config) error` — create ServerConnection, connect, register tools in registry
   - `Disconnect(name) error` — unregister tools, disconnect
   - `Reconnect(ctx, name) error` — disconnect + connect
   - `Toggle(name, enabled bool) error` — set enabled flag; if disabling, unregister tools; if enabling, register tools
   - `SetServers(ctx, servers map) (*SetServersResult, error)` — diff current vs desired, add new, remove old
   - `Status() []ServerStatus` — snapshot of all connections
   - `ServerStatus(name) (*ServerStatus, error)` — single server status
   - Implement `tools.MCPClient` interface:
     - `ListResources(ctx, serverName) ([]tools.MCPResource, error)` — delegate to connection, convert types
     - `ReadResource(ctx, serverName, uri) (MCPResourceContent, error)` — delegate
     - `CallTool(ctx, serverName, toolName, args) (MCPToolCallResult, error)` — delegate
   - `Close() error` — disconnect all servers

3. **`pkg/mcp/status.go`** — Status types for external queries:
   ```go
   type ServerStatus struct {
       Name       string
       Status     ConnectionStatus
       ServerInfo *ServerInfo
       Error      string
       Tools      []ToolInfo
       Scope      string
   }

   type SetServersResult struct {
       Added   []string
       Removed []string
       Errors  map[string]string
   }
   ```

4. **`pkg/mcp/connection_test.go`** — Tests:
   - Initialize handshake with mock transport
   - Capability gating: server without tools capability → skip tools/list
   - Server without resources capability → skip resources/list
   - Tool call delegation
   - Resource operations
   - Disconnect cleanup
   - Error during initialize → status=failed

5. **`pkg/mcp/client_test.go`** — Tests:
   - Connect registers tools in registry
   - Disconnect unregisters tools
   - Reconnect re-registers
   - Toggle off → tools removed, toggle on → tools added
   - SetServers diff logic (add 2, remove 1, keep 1)
   - CallTool routes to correct server
   - ListResources routes to correct server
   - Unknown server → error
   - Concurrent access safety

**Implementation Notes**:
- Use a `mockTransport` in tests that returns pre-programmed responses
- Tool registration: for each tool from the server, call `registry.RegisterMCPTool(serverName, tool.Name, tool.Description, schemaMap, client)`
- The `client` (which implements `tools.MCPClient`) passes itself as the MCPClient to RegisterMCPTool, creating the circular reference needed for tool execution to route back through the client

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/mcp/... -race` passes (all tests)
  - [x] `go vet ./pkg/mcp/...` passes
  - [x] Connection lifecycle fully covered
  - [x] Capability gating tested (tools present vs absent, resources present vs absent)

---

### Phase 5: Update MCPClient Interface + Existing Tools

**Goal**: Update `tools.MCPClient` interface to use structured types instead of strings, update all consumers.

**Changes**:

1. **`pkg/tools/mcp.go:9-37`** — Update interface and types:
   ```go
   // MCPToolCallResult is the structured result of calling an MCP tool.
   type MCPToolCallResult struct {
       Content []MCPContentBlock
       IsError bool
   }

   type MCPContentBlock struct {
       Type     string // "text", "image", "resource"
       Text     string
       MimeType string
       Data     string // base64 for images
       URI      string // for embedded resources
   }

   // MCPResourceContent is the structured result of reading an MCP resource.
   type MCPResourceContent struct {
       URI      string
       MimeType string
       Text     string
       Blob     string // base64 for binary
   }

   // MCPClient communicates with MCP servers.
   type MCPClient interface {
       ListResources(ctx context.Context, serverName string) ([]MCPResource, error)
       ReadResource(ctx context.Context, serverName, uri string) (MCPResourceContent, error)
       CallTool(ctx context.Context, serverName, toolName string, args map[string]any) (MCPToolCallResult, error)
   }
   ```

2. **`pkg/tools/mcp.go:24-37`** — Update `StubMCPClient` to return new types
3. **`pkg/tools/mcptool.go:36-51`** — Update `MCPTool.Execute` to handle structured result:
   - Concatenate text content blocks into a single string
   - Set `IsError` from `MCPToolCallResult.IsError`
4. **`pkg/tools/mcp.go:64-91`** — Update `ListMcpResourcesTool.Execute` (no change needed, MCPResource stays)
5. **`pkg/tools/mcp.go:123-148`** — Update `ReadMcpResourceTool.Execute` to return structured content
6. **`pkg/tools/mcp_test.go`** — Update mock and tests for new types
7. **`pkg/tools/mcptool_test.go`** — Update mock and tests for new types
8. **`pkg/mcp/client.go`** — Ensure Client implements updated interface (type conversions from internal MCP types to tools types)

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/tools/... -race` passes (all existing + updated tests)
  - [x] `go test ./pkg/mcp/... -race` passes
  - [x] `go vet ./...` passes
  - [x] `go build ./...` passes (no compilation errors from changed interface)

---

### Phase 6: Integration Wiring + End-to-End Tests

**Goal**: Wire the MCP client into the agent startup, update `DefaultRegistry`, and add integration tests.

**Changes**:

1. **`pkg/agent/defaults.go`** — Update default registry construction:
   - Add a `DefaultRegistryWithMCP(cwd string, mcpClient tools.MCPClient) *tools.Registry` function (or update existing)
   - Pass `mcpClient` to `ListMcpResourcesTool{Client: mcpClient}` and `ReadMcpResourceTool{Client: mcpClient}`
   - Comment that `mcp.Client.Connect()` will register dynamic tools after startup

2. **`pkg/agent/config.go`** — No structural changes (MCPServers map already present)

3. **`pkg/mcp/integration_test.go`** — End-to-end integration tests:
   - Spin up a mock MCP stdio server (small Go binary or test helper) that responds to initialize, tools/list, tools/call
   - Create an mcp.Client, connect to mock server, verify tools registered in registry
   - Call a tool via the registry, verify result routes through MCP client to mock server and back
   - Disconnect, verify tools removed
   - SetServers with multiple servers

4. **`pkg/mcp/testhelper_test.go`** — Test helpers:
   - `mockTransport` implementing Transport interface
   - Pre-programmed response sequences
   - Optional helper for creating a real stdio process from a test binary

**Success Criteria**:
- Automated:
  - [x] `go test ./... -race` passes (671 tests)
  - [x] `go vet ./...` passes
  - [x] Integration test demonstrates full lifecycle: connect → tools registered → call tool → result → disconnect → tools removed
- Manual:
  - [ ] Connect to a real MCP server (e.g., `npx @modelcontextprotocol/server-filesystem`) and verify tool listing works

---

## Out of Scope

- **SSE transport** (legacy, deprecated in favor of Streamable HTTP)
- **SDK transport** (in-process Go plugins — different architecture)
- **claudeai-proxy transport** (Anthropic internal)
- **Server-initiated notifications** (tools/list_changed, resources/list_changed) — deferred
- **MCP prompts** (prompts/list, prompts/get) — not used by the agent currently
- **MCP sampling** (sampling/createMessage) — server requests LLM call from client
- **MCP roots** (roots/list) — filesystem root declarations
- **Authentication flows** (OAuth, needs-auth status handling) — deferred
- **Session persistence** of MCP connections (reconnect on resume) — depends on Spec 11

## Dependencies

- No new external dependencies required
- Uses: `os/exec`, `bufio`, `encoding/json`, `net/http`, `sync`, `context`, `io`, `fmt`, `strings`
- Existing: `pkg/tools`, `pkg/types`

## Open Questions

*None — all resolved during planning.*

## References

- Spec: `thoughts/specs/10-MCP-INTEGRATION.md`
- MCP Protocol: https://modelcontextprotocol.io/specification
- Existing stubs: `pkg/tools/mcp.go`, `pkg/tools/mcptool.go`
- Pattern reference: `pkg/hooks/shell.go` (stdin/stdout JSON), `pkg/llm/sse.go` (SSE parsing)
- Agent config: `pkg/agent/config.go:55` (MCPServers field)
