# MCP Client — Model Context Protocol Integration

> `pkg/mcp/` — JSON-RPC 2.0 client for MCP servers. Supports stdio and HTTP
> transports, capability-gated tool/resource discovery, and dynamic tool registration.

## MCP Connection Lifecycle

```
 ┌───────────────────────────────────────────────────────────────────┐
 │  mcp.Client.SetServers(serverConfigs)                             │
 │                                                                   │
 │  For each new server config:                                      │
 │                                                                   │
 │  ┌─── connect() ──────────────────────────────────────────────┐  │
 │  │                                                             │  │
 │  │  1. Create transport:                                       │  │
 │  │     stdio → StdioTransport (exec.Command, stdin/stdout)    │  │
 │  │     http  → HTTPTransport (POST JSON-RPC, SSE responses)  │  │
 │  │                                                             │  │
 │  │  2. Start transport                                         │  │
 │  │     stdio: launch process, pipe stdin/stdout                │  │
 │  │     http: validate endpoint URL                             │  │
 │  │                                                             │  │
 │  └─────────────────────────────────────────────────────────────┘  │
 │                                                                   │
 │  ┌─── runHandshake() ─────────────────────────────────────────┐  │
 │  │                                                             │  │
 │  │  3. Send "initialize" request                               │  │
 │  │     {method:"initialize", params:{                          │  │
 │  │       protocolVersion: "2024-11-05",                        │  │
 │  │       capabilities: {roots:{listChanged:true}},             │  │
 │  │       clientInfo: {name:"goat", version:"0.1.0"}           │  │
 │  │     }}                                                      │  │
 │  │                                                             │  │
 │  │  4. Receive initialize response                             │  │
 │  │     {result:{                                               │  │
 │  │       capabilities: {tools:{}, resources:{}},               │  │
 │  │       serverInfo: {name:"...", version:"..."}               │  │
 │  │     }}                                                      │  │
 │  │                                                             │  │
 │  │  5. Send "notifications/initialized" notification           │  │
 │  │     (no ID — fire and forget)                               │  │
 │  │                                                             │  │
 │  │  6. Capability-gated discovery:                             │  │
 │  │     if capabilities.tools:                                  │  │
 │  │       Send "tools/list" → receive tool definitions          │  │
 │  │       Register as mcp__server__toolname in Registry         │  │
 │  │                                                             │  │
 │  │     if capabilities.resources:                              │  │
 │  │       Send "resources/list" → receive resource list         │  │
 │  │       Store for ListMcpResources/ReadMcpResource tools      │  │
 │  │                                                             │  │
 │  │  7. State = READY                                           │  │
 │  │                                                             │  │
 │  └─────────────────────────────────────────────────────────────┘  │
 └───────────────────────────────────────────────────────────────────┘
```

## JSON-RPC 2.0 Protocol

```
 ┌───────────────────────────────────────────────────────────────────┐
 │                                                                   │
 │  REQUEST (has ID):                                                │
 │  ──────────────────────▶                                         │
 │  {"jsonrpc":"2.0", "id":1,                                      │
 │   "method":"tools/call",                                         │
 │   "params":{"name":"search","arguments":{"query":"test"}}}       │
 │                                                                   │
 │  RESPONSE (correlated by ID):                                     │
 │  ◀──────────────────────                                         │
 │  {"jsonrpc":"2.0", "id":1,                                      │
 │   "result":{"content":[{"type":"text","text":"results..."}]}}   │
 │                                                                   │
 │  NOTIFICATION (no ID):                                            │
 │  ──────────────────────▶                                         │
 │  {"jsonrpc":"2.0",                                               │
 │   "method":"notifications/initialized"}                          │
 │  (no response expected)                                           │
 │                                                                   │
 │  ERROR RESPONSE:                                                  │
 │  ◀──────────────────────                                         │
 │  {"jsonrpc":"2.0", "id":1,                                      │
 │   "error":{"code":-32602,"message":"Invalid params"}}            │
 │                                                                   │
 └───────────────────────────────────────────────────────────────────┘
```

## Two Transport Implementations

```
 ┌─── StdioTransport ─────────────────────────────────────────────────┐
 │                                                                     │
 │  exec.CommandContext("path/to/server")                              │
 │  ┌──────────┐     ┌──────────────────┐     ┌──────────┐           │
 │  │  Client   │────▶│  stdin (pipe)    │────▶│  Server  │           │
 │  │          │     │  (write JSON)    │     │ Process  │           │
 │  │          │◀────│  stdout (pipe)   │◀────│          │           │
 │  │          │     │  (read JSON)     │     │          │           │
 │  └──────────┘     └──────────────────┘     └──────────┘           │
 │                                                                     │
 │  Background reader goroutine:                                      │
 │    bufio.Scanner reads lines from stdout                           │
 │    JSON unmarshal → dispatch to pending requests by ID             │
 │                                                                     │
 │  Lifecycle: process started on connect, killed on close            │
 └─────────────────────────────────────────────────────────────────────┘

 ┌─── HTTPTransport ──────────────────────────────────────────────────┐
 │                                                                     │
 │  Streamable HTTP (POST JSON-RPC):                                  │
 │  ┌──────────┐     ┌──────────────────┐     ┌──────────┐           │
 │  │  Client   │────▶│  POST /rpc       │────▶│  HTTP    │           │
 │  │          │     │  Content-Type:   │     │  Server  │           │
 │  │          │     │  application/json│     │          │           │
 │  │          │◀────│  Response:       │◀────│          │           │
 │  │          │     │  JSON or SSE     │     │          │           │
 │  └──────────┘     └──────────────────┘     └──────────┘           │
 │                                                                     │
 │  Response types:                                                    │
 │    application/json → single JSON-RPC response                     │
 │    text/event-stream → SSE with multiple responses                 │
 │                                                                     │
 └─────────────────────────────────────────────────────────────────────┘
```

## Dynamic Tool Registration

```
 MCP Server declares tools:
   tools/list → [{name:"search", inputSchema:{...}, description:"..."}]
         │
         ▼
 For each MCP tool:
   registry.RegisterMCPTool(
     name:        "mcp__context7__search",    ← prefix + server + tool
     schema:      {type:"object", properties:{query:{type:"string"}}},
     description: "Search documentation",
     annotations: {readOnly: true},           ← from server capabilities
     client:      mcpClient,                  ← circular ref for execution
   )
         │
         ▼
 Tool available in LLM tool list as "mcp__context7__search"
 LLM can call it like any other tool
         │
         ▼
 On execution:
   registry.Get("mcp__context7__search")
   tool.Execute(ctx, input)
     → mcpClient.CallTool("context7", "search", input)
       → JSON-RPC: {method:"tools/call", params:{name:"search", arguments:input}}
       → Response: {content:[{type:"text", text:"..."}]}
     → ToolOutput{Content: "..."}
```

## SetServers — Bulk Server Management

```
 ┌───────────────────────────────────────────────────────────────────┐
 │  SetServers(newConfigs):                                          │
 │                                                                   │
 │  1. Diff existing vs new:                                         │
 │     ┌─────────────────────────────────────────────────────────┐  │
 │     │  existing = {context7, playwright}                      │  │
 │     │  new      = {context7, github}                          │  │
 │     │                                                         │  │
 │     │  keep:   context7 (same config? keep connection)        │  │
 │     │  remove: playwright (not in new set)                    │  │
 │     │  add:    github (not in existing set)                   │  │
 │     └─────────────────────────────────────────────────────────┘  │
 │                                                                   │
 │  2. Remove old:                                                   │
 │     Close playwright connection                                   │
 │     registry.UnregisterMCPTools("mcp__playwright__")             │
 │                                                                   │
 │  3. Add new:                                                      │
 │     connect("github") → handshake → register tools               │
 │                                                                   │
 │  4. Keep existing:                                                │
 │     context7 stays connected, tools remain registered            │
 └───────────────────────────────────────────────────────────────────┘
```

## Toggle — Enable/Disable Server

```
 Toggle(serverName, enabled):
   enabled=false:
     registry.UnregisterMCPTools("mcp__serverName__")
     server.disabled = true
     (connection stays open, just tools removed)

   enabled=true:
     Re-register all server tools
     server.disabled = false
```

## Comparison with Claude Code TS MCP

```
 ┌────────────────────────────┬──────────────────────────────────────┐
 │ Claude Code TS              │ Goat Go                              │
 ├────────────────────────────┼──────────────────────────────────────┤
 │ @modelcontextprotocol/sdk  │ Custom JSON-RPC implementation       │
 │ stdio transport only       │ stdio + HTTP transports              │
 │ Dynamic import              │ Interface-based MCPClient            │
 │ Error as JS exceptions     │ Error as (result, error) tuples      │
 │ Single-threaded             │ Concurrent-safe with mutexes         │
 │ Process management: spawn  │ exec.CommandContext with context      │
 │                              │   cancellation                       │
 └────────────────────────────┴──────────────────────────────────────┘

 Go advantage: Context cancellation propagates through the entire
 MCP call chain. If the agent loop is interrupted, all pending MCP
 calls are automatically cancelled via ctx.Done().
```
