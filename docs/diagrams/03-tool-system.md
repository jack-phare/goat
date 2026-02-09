# Tool System — Registry, Execution & 21 Static Tools

> `pkg/tools/` — The tool layer. Provides a unified interface for all 21 built-in
> tools plus dynamic MCP tools, with a central Registry for lookup and execution.

## Tool Interface

Every tool in Goat implements this interface:

```go
type Tool interface {
    Name() string                                          // "Bash", "Read", etc.
    Description() string                                   // LLM-facing description
    InputSchema() map[string]any                           // JSON Schema for params
    SideEffect() SideEffectType                            // risk classification
    Execute(ctx context.Context, input map[string]any) (ToolOutput, error)
}
```

## Registry Architecture

```
 ┌────────────────────────────────────────────────────────────────────┐
 │                         tools.Registry                             │
 │                                                                    │
 │  tools      map[string]Tool         // name → tool instance       │
 │  allowed    map[string]bool         // auto-allowed tools         │
 │  order      []string                // insertion order for LLM    │
 │  mu         sync.RWMutex            // concurrent safety          │
 │                                                                    │
 │  Register(tool Tool)                // add static tool            │
 │  RegisterMCPTool(name, schema,      // add dynamic mcp__* tool   │
 │    desc, annotations, client)       //   (wraps MCPClient.Call)  │
 │  UnregisterMCPTools(prefix)         // remove all with prefix     │
 │  Get(name) (Tool, bool)             // lookup by name             │
 │  LLMTools() []LLMTool              // all tools as JSON schemas   │
 │  Names() []string                   // all registered names       │
 │                                                                    │
 │  WithAllowed(names...)              // mark as auto-allowed       │
 └────────────────────────────────────────────────────────────────────┘
```

## DefaultRegistry — The 21 Static Tools

Created by `agent.DefaultRegistry(cwd, mcpClient)`:

```
 ┌─────────────────────────────────────────────────────────────────────┐
 │                    Tool Categories                                   │
 │                                                                     │
 │  ┌─ Core 6 (Claude Code equivalent) ────────────────────────────┐  │
 │  │  Bash        exec.CommandContext, timeout, CWD, background   │  │
 │  │  Read        Line numbers, offset/limit                      │  │
 │  │  Write       Create dirs, overwrite, validate path           │  │
 │  │  Edit        Find-replace, replace_all mode                  │  │
 │  │  Glob        doublestar patterns, recursive matching         │  │
 │  │  Grep        ripgrep wrapper (exec "rg")                     │  │
 │  └──────────────────────────────────────────────────────────────┘  │
 │                                                                     │
 │  ┌─ Background Tasks ───────────────────────────────────────────┐  │
 │  │  TaskOutput   Read background task output (block/non-block)  │  │
 │  │  TaskStop     Cancel running background task                 │  │
 │  └──────────────────────────────────────────────────────────────┘  │
 │                                                                     │
 │  ┌─ State Management ───────────────────────────────────────────┐  │
 │  │  TodoWrite       In-memory todo list, full overwrite         │  │
 │  │  Config          ConfigStore interface, InMemoryConfigStore  │  │
 │  │  ExitPlanMode    Marker response for plan mode transition    │  │
 │  │  AskUserQuestion UserInputHandler interface, validates opts  │  │
 │  └──────────────────────────────────────────────────────────────┘  │
 │                                                                     │
 │  ┌─ Network ────────────────────────────────────────────────────┐  │
 │  │  WebFetch    HTTP GET + HTML-to-text (x/net/html tokenizer) │  │
 │  │  WebSearch   SearchProvider interface, StubSearchProvider    │  │
 │  └──────────────────────────────────────────────────────────────┘  │
 │                                                                     │
 │  ┌─ Subagent ───────────────────────────────────────────────────┐  │
 │  │  Agent      SubagentSpawner interface, StubSubagentSpawner  │  │
 │  └──────────────────────────────────────────────────────────────┘  │
 │                                                                     │
 │  ┌─ MCP Resources ──────────────────────────────────────────────┐  │
 │  │  ListMcpResources   MCPClient interface, list resources     │  │
 │  │  ReadMcpResource    MCPClient interface, read resource      │  │
 │  └──────────────────────────────────────────────────────────────┘  │
 │                                                                     │
 │  ┌─ Notebook ───────────────────────────────────────────────────┐  │
 │  │  NotebookEdit   Jupyter .ipynb replace/insert/delete cells  │  │
 │  └──────────────────────────────────────────────────────────────┘  │
 │                                                                     │
 │  ┌─ Teams (feature-gated: CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS) ┐  │
 │  │  TeamCreate     TeamCoordinator interface, spawn team        │  │
 │  │  SendMessage    Direct/broadcast/shutdown messaging          │  │
 │  │  TeamDelete     Cleanup active team                          │  │
 │  └──────────────────────────────────────────────────────────────┘  │
 │                                                                     │
 │  ┌─ Dynamic (registered at runtime by mcp.Client) ─────────────┐  │
 │  │  mcp__*         MCP server tools, name = mcp__server__tool  │  │
 │  └──────────────────────────────────────────────────────────────┘  │
 └─────────────────────────────────────────────────────────────────────┘
```

## Tool Execution Flow (from Agent Loop)

```
 LLM Response: tool_use content block
 {type:"tool_use", id:"call_1", name:"Bash", input:{"command":"ls"}}
         │
         ▼
 ┌───────────────────────────────────────────────────────────────────┐
 │  executeTools() — Serial execution for each tool block           │
 │                                                                   │
 │  for each tool block:                                             │
 │    ┌──────────────────────────────────────────────────────────┐  │
 │    │  1. Context check                                        │  │
 │    │     └── ctx.Done()? → error "operation cancelled"        │  │
 │    │                                                          │  │
 │    │  2. executeSingleTool(ctx, block, config, state, ch)     │  │
 │    │     │                                                    │  │
 │    │     ├── Registry lookup: config.ToolRegistry.Get(name)   │  │
 │    │     │   └── not found? → error "unknown tool"            │  │
 │    │     │                                                    │  │
 │    │     ├── Permission check                                  │  │
 │    │     │   config.Permissions.Check(ctx, name, input)       │  │
 │    │     │   ├── allow → continue                             │  │
 │    │     │   ├── deny → error + maybe interrupt               │  │
 │    │     │   └── updated input? → replace input               │  │
 │    │     │                                                    │  │
 │    │     ├── PreToolUse hook                                   │  │
 │    │     │   config.Hooks.Fire(PreToolUse, {name, id, input}) │  │
 │    │     │   ├── deny → error "denied by hook"                │  │
 │    │     │   ├── updated input → replace input                │  │
 │    │     │   └── additional context → pending                 │  │
 │    │     │                                                    │  │
 │    │     ├── Emit ToolProgress (start)                         │  │
 │    │     │                                                    │  │
 │    │     ├── tool.Execute(ctx, input)                         │  │
 │    │     │   └── returns ToolOutput or error                  │  │
 │    │     │                                                    │  │
 │    │     ├── Emit ToolProgress (end, elapsed)                  │  │
 │    │     │                                                    │  │
 │    │     ├── On error:                                         │  │
 │    │     │   Fire PostToolUseFailure hook                      │  │
 │    │     │   Return error result                               │  │
 │    │     │                                                    │  │
 │    │     ├── On success:                                       │  │
 │    │     │   Fire PostToolUse hook                              │  │
 │    │     │   Check SuppressOutput                              │  │
 │    │     │   Return tool result                                │  │
 │    │     │                                                    │  │
 │    │     └── Return ToolResult + interrupted flag              │  │
 │    └──────────────────────────────────────────────────────────┘  │
 │                                                                   │
 │    3. Check interrupted                                           │
 │       └── yes: fill remaining with errors, return interrupted    │
 └───────────────────────────────────────────────────────────────────┘
```

## SideEffect Types & Risk Mapping

```
 ┌─────────────────────┬──────────┬──────────────────────────────────┐
 │ SideEffectType      │ Risk     │ Tools                            │
 ├─────────────────────┼──────────┼──────────────────────────────────┤
 │ SideEffectNone      │ None     │ Read, Glob, Grep, TodoWrite     │
 │ SideEffectReadOnly  │ Low      │ Config, ListMcp, ReadMcp,       │
 │                     │          │ AskUser, ExitPlanMode, TaskOut  │
 │ SideEffectMutating  │ Medium   │ Write, Edit, NotebookEdit       │
 │ SideEffectNetwork   │ High     │ Bash, WebFetch, WebSearch       │
 │ SideEffectBlocking  │ Low      │ TaskStop                        │
 │ SideEffectSpawns    │ Critical │ Agent                            │
 └─────────────────────┴──────────┴──────────────────────────────────┘
```

## Background TaskManager

Shared between Bash, TaskOutput, and TaskStop tools:

```
 ┌───────────────────────────────────────────────────────────────┐
 │                      TaskManager                              │
 │                                                               │
 │  mu    sync.Mutex                                            │
 │  tasks map[string]*BackgroundTask                            │
 │                                                               │
 │  Start(ctx, cmd) → taskID                                    │
 │    ├── Create cancellable context                             │
 │    ├── Start exec.CommandContext                               │
 │    ├── Launch goroutine to read stdout                        │
 │    ├── Store in tasks map                                     │
 │    └── Return task ID                                         │
 │                                                               │
 │  GetOutput(taskID, block, timeout)                            │
 │    ├── block=true:  wait for completion or timeout            │
 │    └── block=false: return current output immediately         │
 │                                                               │
 │  Stop(taskID)                                                 │
 │    └── Cancel context → process killed                        │
 │                                                               │
 │  Task States:                                                 │
 │    Running → Completed | Failed | Stopped                     │
 └───────────────────────────────────────────────────────────────┘

 Usage in BashTool:
   if input["run_in_background"] == true:
     taskID = tm.Start(ctx, cmd)
     return ToolOutput{Content: "Task started: " + taskID}
   else:
     run synchronously with timeout
```

## Dynamic MCP Tool Registration

```
 mcp.Client discovers server tools
         │
         ▼
 registry.RegisterMCPTool(
     name:        "mcp__server_name__tool_name",
     schema:      {...from server},
     description: "...",
     annotations: {ReadOnly: true/false, Destructive: true/false},
     client:      mcpClient,
 )
         │
         ▼
 Creates internal mcpToolWrapper:
   Name() → "mcp__server_name__tool_name"
   Execute(ctx, input) →
     client.CallTool(serverName, toolName, input)
     → returns MCPToolCallResult
     → formats as ToolOutput

 Permission integration:
   "mcp__*" prefix detected by permission system
   Risk based on annotations:
     ReadOnly=true  → RiskLow
     Destructive=true → RiskCritical
     Neither → RiskHigh (default, conservative)
```

## Auto-Allowed vs Permission-Gated Tools

```
 Auto-Allowed (no permission prompt):
 ┌──────────────────────────────────────┐
 │  Read, Glob, Grep,                   │
 │  ListMcpResources, ReadMcpResource   │
 └──────────────────────────────────────┘

 Permission-Gated:
 ┌──────────────────────────────────────┐
 │  Everything else — checked by        │
 │  permission system based on risk     │
 │  level and permission mode           │
 └──────────────────────────────────────┘
```
