# Spec 10: MCP Integration

**Go Package**: `pkg/mcp/`
**Source References**:
- `sdk.d.ts:290-398` — McpServerConfig variants (stdio, sse, http, sdk, claudeai-proxy), McpServerStatus, McpSetServersResult
- `sdk.d.ts:634` — Options.mcpServers
- `sdk.d.ts:1032-1072` — Query methods: mcpServerStatus, reconnectMcpServer, toggleMcpServer, setMcpServers
- `sdk-tools.d.ts:280-295` — ListMcpResourcesInput, McpInput, ReadMcpResourceInput
- MCP Protocol Specification: https://modelcontextprotocol.io/specification

---

## 1. Purpose

MCP (Model Context Protocol) integration allows the agent to connect to external tool servers that expose tools, resources, and prompts. The Go port must manage MCP server connections, register their tools dynamically, and handle the MCP JSON-RPC protocol.

---

## 2. MCP Server Config Types (from `sdk.d.ts:290-330`)

```go
// MCPServerConfig is a union of all supported MCP transport types.
type MCPServerConfig struct {
    Type    MCPTransportType
    // Stdio fields
    Command string            `json:"command,omitempty"`
    Args    []string          `json:"args,omitempty"`
    Env     map[string]string `json:"env,omitempty"`
    // SSE/HTTP fields
    URL     string            `json:"url,omitempty"`
    Headers map[string]string `json:"headers,omitempty"`
    // SDK fields
    Name    string            `json:"name,omitempty"`
    // ClaudeAI proxy fields
    ID      string            `json:"id,omitempty"`
}

type MCPTransportType string

const (
    MCPStdio     MCPTransportType = "stdio"       // sdk.d.ts:396-400
    MCPSSE       MCPTransportType = "sse"          // sdk.d.ts:390-394
    MCPHTTP      MCPTransportType = "http"         // sdk.d.ts:304-308
    MCPSdk       MCPTransportType = "sdk"          // sdk.d.ts:310-313 (in-process)
    MCPClaudeAI  MCPTransportType = "claudeai-proxy" // sdk.d.ts:290-294
)
```

---

## 3. MCP Server Status (from `sdk.d.ts:331-375`)

```go
type MCPServerStatus struct {
    Name       string              `json:"name"`
    Status     MCPConnectionStatus `json:"status"`
    ServerInfo *MCPServerInfo      `json:"serverInfo,omitempty"`
    Error      string              `json:"error,omitempty"`
    Config     *MCPServerConfig    `json:"config,omitempty"`
    Scope      string              `json:"scope,omitempty"` // "project"|"user"|"local"|"claudeai"|"managed"
    Tools      []MCPToolInfo       `json:"tools,omitempty"`
}

type MCPConnectionStatus string

const (
    MCPConnected  MCPConnectionStatus = "connected"
    MCPFailed     MCPConnectionStatus = "failed"
    MCPNeedsAuth  MCPConnectionStatus = "needs-auth"
    MCPPending    MCPConnectionStatus = "pending"
    MCPDisabled   MCPConnectionStatus = "disabled"
)

type MCPServerInfo struct {
    Name    string `json:"name"`
    Version string `json:"version"`
}

type MCPToolInfo struct {
    Name        string            `json:"name"`
    Description string            `json:"description,omitempty"`
    Annotations *MCPToolAnnotations `json:"annotations,omitempty"`
}

type MCPToolAnnotations struct {
    ReadOnly    *bool `json:"readOnly,omitempty"`
    Destructive *bool `json:"destructive,omitempty"`
    OpenWorld   *bool `json:"openWorld,omitempty"`
}
```

---

## 4. Go Client Interface

```go
// Client manages connections to MCP servers.
type Client struct {
    mu        sync.RWMutex
    servers   map[string]*ServerConnection
    registry  *tools.Registry // for dynamic tool registration
}

func NewClient(registry *tools.Registry) *Client

// Lifecycle
func (c *Client) Connect(name string, config MCPServerConfig) error
func (c *Client) Disconnect(name string) error
func (c *Client) Reconnect(name string) error
func (c *Client) Toggle(name string, enabled bool) error

// Bulk operations (maps to Query.setMcpServers)
func (c *Client) SetServers(servers map[string]MCPServerConfig) (*MCPSetServersResult, error)

// Status
func (c *Client) Status() []MCPServerStatus
func (c *Client) ServerStatus(name string) (*MCPServerStatus, error)

// Tool execution (called by tool registry for mcp__* tools)
func (c *Client) CallTool(serverName, toolName string, args map[string]interface{}) (interface{}, error)

// Resource access
func (c *Client) ListResources(serverName string) ([]MCPResource, error)
func (c *Client) ReadResource(serverName string, uri string) (interface{}, error)
```

---

## 5. MCP Set Servers Result (from `sdk.d.ts:377-389`)

```go
type MCPSetServersResult struct {
    Added   []string          `json:"added"`
    Removed []string          `json:"removed"`
    Errors  map[string]string `json:"errors"`
}
```

---

## 6. Server Connection

```go
// ServerConnection wraps a single MCP server connection.
type ServerConnection struct {
    Name      string
    Config    MCPServerConfig
    Status    MCPConnectionStatus
    Info      *MCPServerInfo
    Tools     []MCPToolInfo
    Enabled   bool
    Transport MCPTransport

    mu sync.Mutex
}

// MCPTransport abstracts the communication with an MCP server.
type MCPTransport interface {
    // Initialize performs the MCP handshake.
    Initialize(ctx context.Context) (*MCPServerInfo, error)
    // ListTools returns available tools from the server.
    ListTools(ctx context.Context) ([]MCPToolInfo, error)
    // CallTool invokes a tool on the server.
    CallTool(ctx context.Context, name string, args map[string]interface{}) (*MCPToolResult, error)
    // ListResources returns available resources.
    ListResources(ctx context.Context) ([]MCPResource, error)
    // ReadResource reads a specific resource.
    ReadResource(ctx context.Context, uri string) (*MCPResourceContent, error)
    // Close terminates the connection.
    Close() error
}

type MCPToolResult struct {
    Content []MCPContent `json:"content"`
    IsError bool         `json:"isError,omitempty"`
}

type MCPContent struct {
    Type string `json:"type"` // "text", "image", "resource"
    Text string `json:"text,omitempty"`
    // Other content types...
}
```

---

## 7. Transport Implementations

### 7.1 Stdio Transport

```go
// StdioTransport communicates with an MCP server via stdin/stdout JSON-RPC.
type StdioTransport struct {
    cmd     *exec.Cmd
    stdin   io.WriteCloser
    stdout  *bufio.Scanner
    mu      sync.Mutex
    nextID  int
}

func NewStdioTransport(command string, args []string, env map[string]string) (*StdioTransport, error) {
    cmd := exec.Command(command, args...)
    for k, v := range env {
        cmd.Env = append(cmd.Env, k+"="+v)
    }
    stdin, _ := cmd.StdinPipe()
    stdout, _ := cmd.StdoutPipe()
    cmd.Start()
    return &StdioTransport{
        cmd:    cmd,
        stdin:  stdin,
        stdout: bufio.NewScanner(stdout),
    }, nil
}
```

### 7.2 SSE/HTTP Transport

```go
// HTTPTransport communicates with an MCP server via HTTP/SSE.
type HTTPTransport struct {
    url     string
    headers map[string]string
    client  *http.Client
}

func NewHTTPTransport(url string, headers map[string]string) *HTTPTransport
func NewSSETransport(url string, headers map[string]string) *SSETransport
```

---

## 8. Dynamic Tool Registration

When an MCP server connects, its tools are registered in the tool registry:

```go
func (c *Client) registerServerTools(conn *ServerConnection) error {
    tools, err := conn.Transport.ListTools(context.Background())
    if err != nil {
        return err
    }

    conn.Tools = tools

    for _, tool := range tools {
        c.registry.RegisterMCPTool(conn.Name, tool.Name, tool.Description, tool.Schema)
    }

    return nil
}

func (c *Client) unregisterServerTools(name string) {
    c.registry.UnregisterMCPTools(name)
}
```

---

## 9. MCP JSON-RPC Protocol

All MCP communication uses JSON-RPC 2.0:

```go
type JSONRPCRequest struct {
    JSONRPC string      `json:"jsonrpc"` // "2.0"
    ID      int         `json:"id"`
    Method  string      `json:"method"`
    Params  interface{} `json:"params,omitempty"`
}

type JSONRPCResponse struct {
    JSONRPC string          `json:"jsonrpc"`
    ID      int             `json:"id"`
    Result  json.RawMessage `json:"result,omitempty"`
    Error   *JSONRPCError   `json:"error,omitempty"`
}

type JSONRPCError struct {
    Code    int         `json:"code"`
    Message string      `json:"message"`
    Data    interface{} `json:"data,omitempty"`
}

// MCP methods
const (
    MethodInitialize    = "initialize"
    MethodToolsList     = "tools/list"
    MethodToolsCall     = "tools/call"
    MethodResourcesList = "resources/list"
    MethodResourcesRead = "resources/read"
)
```

---

## 10. Connection Lifecycle

```
Connect(name, config)
    │
    ▼
[Spawn process / open HTTP connection]
    │
    ▼
Initialize (JSON-RPC "initialize")
    │ ◄── returns server info
    ▼
ListTools (JSON-RPC "tools/list")
    │ ◄── returns tool definitions
    ▼
Register tools in Registry (mcp__{name}__{tool})
    │
    ▼
Status: "connected"
    │
    │ (on tool call from agent)
    ▼
CallTool (JSON-RPC "tools/call")
    │ ◄── returns tool result
    ▼
Return to agent loop
```

---

## 11. Verification Checklist

- [ ] **Config types**: All 5 MCP transport types (stdio, sse, http, sdk, claudeai-proxy) supported
- [ ] **Status values**: All 5 connection statuses tracked correctly
- [ ] **Tool registration**: MCP tools appear in tool registry as `mcp__{server}__{tool}`
- [ ] **Tool unregistration**: Disconnecting a server removes its tools
- [ ] **SetServers**: Correctly diffs old vs new, adding/removing as needed
- [ ] **JSON-RPC**: Correct 2.0 protocol with ID correlation
- [ ] **Stdio lifecycle**: Process spawned on connect, killed on disconnect
- [ ] **HTTP/SSE**: Correct header passing and connection management
- [ ] **Reconnect**: Tears down and re-establishes connection
- [ ] **Toggle**: Disabled servers don't expose tools but maintain connection
- [ ] **Resource access**: ListMcpResources and ReadMcpResource tools work end-to-end
- [ ] **Error propagation**: MCP server errors returned as tool_result errors
- [ ] **Annotations**: Tool annotations (readOnly, destructive) inform permission system
