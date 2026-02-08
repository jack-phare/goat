# Remaining Tools + Background Task Manager Implementation Plan

## Overview
Implement all 13 remaining tools from Spec 04, plus a background task manager to support `run_in_background` for Bash and the Agent/Task tool. This completes the full tool catalog of 19 tools.

## Current State
- **6 core tools** implemented and tested (39 tests pass): Bash, FileRead, FileWrite, FileEdit, Glob, Grep
- **Tool registry** fully working with Register/Get/Disable/Allow, OpenAI-format schema generation, and LLM adapter
- **Agent loop** integrates with tools via `executeTools()` in `pkg/agent/tools.go`
- **Existing patterns**: CWD injection, `map[string]any` input, `ToolOutput{Content, IsError}` return, `t.TempDir()` in tests
- **Missing**: All tools listed in Spec 04 §2.1 rows 1, 3, 4, 10-19

## Desired End State
- All 19 tools from Spec 04 are registered and functional
- Background task manager supports `run_in_background` for Bash (and future Agent tool)
- `TaskOutput` and `TaskStop` tools work with the task manager
- WebFetch does real HTTP GET + HTML-to-text extraction
- WebSearch uses a configurable `SearchProvider` interface with a default stub
- NotebookEdit supports replace/insert/delete of Jupyter cells
- Agent/Task tool calls a `SubagentSpawner` interface (stubbed — real implementation deferred to Spec 08)
- MCP tools (`ListMcpResources`, `ReadMcpResource`, `mcp__*`) call an `MCPClient` interface (stubbed — real implementation deferred to Spec 10)
- `DefaultRegistry()` registers all 19 tools
- All new tools have comprehensive test suites
- `go test -race ./pkg/tools/...` and `go vet ./pkg/tools/...` pass

---

## Phases

### Phase 1: Background Task Manager
**Goal**: Build the infrastructure for running tools in the background and retrieving their output. This is a prerequisite for Bash `run_in_background`, Agent tool, TaskOutput, and TaskStop.

**Changes**:
- `pkg/tools/taskmanager.go` — Background task lifecycle manager:
  ```go
  // TaskManager tracks background tasks (bash commands, subagent loops).
  type TaskManager struct {
      mu     sync.RWMutex
      tasks  map[string]*BackgroundTask
  }

  type BackgroundTask struct {
      ID        string
      Status    TaskStatus // Running, Completed, Failed, Stopped
      Output    *TaskOutput  // thread-safe accumulated output
      Cancel    context.CancelFunc
      Done      chan struct{}
      StartedAt time.Time
      Error     error
  }

  type TaskStatus int
  const (
      TaskRunning   TaskStatus = iota
      TaskCompleted
      TaskFailed
      TaskStopped
  )

  type TaskOutput struct {
      mu      sync.Mutex
      content strings.Builder
  }

  func NewTaskManager() *TaskManager
  func (tm *TaskManager) Launch(ctx context.Context, id string, fn func(ctx context.Context) (string, error)) *BackgroundTask
  func (tm *TaskManager) Get(id string) (*BackgroundTask, bool)
  func (tm *TaskManager) GetOutput(id string, block bool, timeout time.Duration) (string, error)
  func (tm *TaskManager) Stop(id string) error
  ```
- `pkg/tools/taskmanager_test.go`:
  - Test launch + immediate completion
  - Test blocking GetOutput waits for task
  - Test non-blocking GetOutput returns partial
  - Test Stop cancels running task
  - Test timeout on GetOutput
  - Test concurrent access (with `-race`)

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/tools/... -run TestTaskManager` passes
  - [x] `go test -race ./pkg/tools/... -run TestTaskManager` passes
  - [x] `go vet ./pkg/tools/...` passes

---

### Phase 2: Enhance Bash with Background Support
**Goal**: Wire `run_in_background` into the existing Bash tool using the TaskManager.

**Changes**:
- `pkg/tools/bash.go`:
  - Add `TaskManager *TaskManager` field to `BashTool`
  - When `run_in_background: true` in input:
    1. Generate task ID (UUID or short random)
    2. Launch command via `tm.Launch()`
    3. Return immediately with task ID and instructions to use `TaskOutput` to check results
  - When `run_in_background` is false/absent: existing behavior unchanged
- `pkg/tools/bash_test.go`:
  - Test `run_in_background=true` returns task ID immediately
  - Test background task output can be retrieved via TaskManager

**Success Criteria**:
- Automated:
  - [x] All existing Bash tests still pass (no regression)
  - [x] `go test ./pkg/tools/... -run TestBash` passes (including new background tests)

---

### Phase 3: TaskOutput & TaskStop Tools
**Goal**: Implement the tools that interact with the background task manager.

**Changes**:
- `pkg/tools/taskoutput.go` — TaskOutput tool:
  ```
  Name: "TaskOutput"
  Input: task_id (required), block (optional, default true), timeout (optional, default 30000ms)
  Execution: Calls TaskManager.GetOutput(id, block, timeout)
  SideEffect: SideEffectNone
  ```
- `pkg/tools/taskoutput_test.go`:
  - Test retrieves completed task output
  - Test blocks until task completes
  - Test timeout returns partial + error
  - Test unknown task_id returns error
- `pkg/tools/taskstop.go` — TaskStop tool:
  ```
  Name: "TaskStop"
  Input: task_id (required)
  Execution: Calls TaskManager.Stop(id)
  SideEffect: SideEffectMutating (state change, but naming follows spec)
  ```
- `pkg/tools/taskstop_test.go`:
  - Test stops running task
  - Test stopping already-completed task returns error or no-op
  - Test unknown task_id returns error

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/tools/... -run TestTaskOutput` passes
  - [x] `go test ./pkg/tools/... -run TestTaskStop` passes

---

### Phase 4: TodoWrite Tool
**Goal**: Implement the structured todo list tool.

**Changes**:
- `pkg/tools/todowrite.go`:
  ```
  Name: "TodoWrite"
  Input: todos (required array of {content, status, activeForm})
  Execution: Stores the todo list in-memory. Returns formatted list.
  SideEffect: SideEffectNone (state change, but internal)
  ```
  - Holds a `Todos []TodoItem` slice (mutex-protected for concurrent access)
  - `Execute` replaces the full list with the input (the SDK's TodoWrite is a full overwrite, not a patch)
  - Returns formatted markdown of the current todo list
- `pkg/tools/todowrite_test.go`:
  - Test create initial list
  - Test overwrite existing list
  - Test validates status enum (pending/in_progress/completed)
  - Test empty todos array is valid (clears the list)

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/tools/... -run TestTodoWrite` passes

---

### Phase 5: AskUserQuestion Tool
**Goal**: Implement the tool that blocks for user input, delegating to a callback interface.

**Changes**:
- `pkg/tools/askuser.go`:
  ```
  Name: "AskUserQuestion"
  Input: questions (required, 1-4 items, each with question/header/options/multiSelect)
  Execution: Calls UserInputHandler.AskQuestions(ctx, questions) which blocks for user response
  SideEffect: SideEffectBlocking
  ```
  - Define `UserInputHandler` interface:
    ```go
    type UserInputHandler interface {
        AskQuestions(ctx context.Context, questions []QuestionSpec) (map[string]string, error)
    }
    ```
  - `AskUserQuestionTool` holds a `Handler UserInputHandler` field
  - If Handler is nil, return error "user input not available in this context"
  - Validates: 1-4 questions, 2-4 options per question, header max 12 chars
- `pkg/tools/askuser_test.go`:
  - Test with mock handler returns formatted answer
  - Test nil handler returns appropriate error
  - Test validation: too many questions, too many options, missing required fields

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/tools/... -run TestAskUser` passes

---

### Phase 6: Config Tool
**Goal**: Implement the runtime config get/set tool.

**Changes**:
- `pkg/tools/config.go`:
  ```
  Name: "Config"
  Input: setting (required string), value (optional — nil means read)
  Execution: Calls ConfigStore.Get/Set
  SideEffect: SideEffectNone (state change, but internal)
  ```
  - Define `ConfigStore` interface:
    ```go
    type ConfigStore interface {
        Get(key string) (any, bool)
        Set(key string, value any) error
    }
    ```
  - Default implementation: `InMemoryConfigStore` backed by `sync.Map`
  - Tool reads config value if `value` is nil, sets it otherwise
  - Returns "key = value" on get, "key set to value" on set
- `pkg/tools/config_test.go`:
  - Test get non-existent key → error
  - Test set then get → returns value
  - Test set string, bool, number types

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/tools/... -run TestConfig` passes

---

### Phase 7: ExitPlanMode Tool
**Goal**: Implement the plan mode exit tool that signals state change.

**Changes**:
- `pkg/tools/exitplanmode.go`:
  ```
  Name: "ExitPlanMode"
  Input: allowedPrompts (optional array of {tool, prompt})
  Execution: Returns a marker response indicating plan mode should be exited.
             The agent loop checks for this in the tool result.
  SideEffect: SideEffectNone
  ```
  - The tool simply validates the input and returns a structured response
  - The agent loop (or a future plan mode handler) reads the response to transition state
  - For now, returns "Exiting plan mode. Allowed prompts: [list]" as content
- `pkg/tools/exitplanmode_test.go`:
  - Test with no prompts
  - Test with prompts array
  - Test validates prompt structure (tool must be "Bash")

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/tools/... -run TestExitPlanMode` passes

---

### Phase 8: WebFetch Tool
**Goal**: Implement real HTTP GET with HTML-to-text extraction.

**Changes**:
- `pkg/tools/webfetch.go`:
  ```
  Name: "WebFetch"
  Input: url (required), prompt (required)
  Execution:
    1. HTTP GET with 30s timeout, User-Agent header
    2. Extract text from HTML (strip tags, decode entities)
    3. Truncate to reasonable size (e.g. 50,000 chars)
    4. Return extracted text + prompt context
  SideEffect: SideEffectNetwork
  ```
  - Uses `net/http` for fetching
  - HTML-to-text: strip tags using a simple state machine or `golang.org/x/net/html` tokenizer
  - Auto-upgrade HTTP to HTTPS
  - Handle redirects (follow up to 10)
  - Return: "Fetched content from {url}:\n\n{extracted_text}"
  - Note: The SDK runs a summarizer subagent with the prompt; we return the raw extracted text + prompt for now. The caller (LLM) gets the text and can reason about the prompt itself.
  - Add dependency: `golang.org/x/net` for HTML tokenizer
- `pkg/tools/webfetch_test.go`:
  - Test with httptest server returning HTML → extracts text
  - Test HTTP→HTTPS upgrade
  - Test timeout handling
  - Test invalid URL returns error
  - Test non-HTML content (plain text) returned as-is
  - Test large content truncation

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/tools/... -run TestWebFetch` passes
  - [x] No external network calls in tests (all httptest)

---

### Phase 9: WebSearch Tool
**Goal**: Implement web search with a configurable provider interface.

**Changes**:
- `pkg/tools/websearch.go`:
  ```
  Name: "WebSearch"
  Input: query (required), allowed_domains (optional), blocked_domains (optional)
  Execution: Calls SearchProvider.Search(ctx, query, opts)
  SideEffect: SideEffectNetwork
  ```
  - Define `SearchProvider` interface:
    ```go
    type SearchProvider interface {
        Search(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error)
    }
    type SearchOptions struct {
        AllowedDomains []string
        BlockedDomains []string
    }
    type SearchResult struct {
        Title   string
        URL     string
        Snippet string
    }
    ```
  - Default: `StubSearchProvider` that returns "Web search not configured. Set a SearchProvider on the WebSearchTool."
  - Format results as markdown list with title, URL, snippet
- `pkg/tools/websearch_test.go`:
  - Test with mock provider returning results → formatted output
  - Test stub provider returns helpful error message
  - Test with allowed_domains filtering (passed to provider)
  - Test with blocked_domains filtering
  - Test empty results → "No results found"

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/tools/... -run TestWebSearch` passes

---

### Phase 10: NotebookEdit Tool
**Goal**: Full Jupyter notebook cell editing (replace, insert, delete).

**Changes**:
- `pkg/tools/notebookedit.go`:
  ```
  Name: "NotebookEdit"
  Input: notebook_path (required), new_source (required for replace/insert),
         cell_number (optional 0-indexed), cell_id (optional),
         cell_type (optional: "code"|"markdown"), edit_mode (optional: "replace"|"insert"|"delete")
  Execution:
    1. Read .ipynb file, parse as JSON
    2. Validate cell_number or cell_id
    3. Perform operation:
       - replace: overwrite cell source with new_source
       - insert: add new cell after cell_number (or at beginning)
       - delete: remove cell at cell_number
    4. Write back to file, preserving formatting
  SideEffect: SideEffectMutating
  ```
  - Jupyter .ipynb format: JSON with `cells` array, each cell has `cell_type`, `source` (array of strings), `metadata`, `outputs` (for code cells)
  - Parse with `encoding/json` into `map[string]any` to preserve unknown fields
  - Validate: notebook_path must be absolute, must end in `.ipynb`
  - For insert: create new cell with appropriate structure (empty outputs, empty metadata)
- `pkg/tools/notebookedit_test.go`:
  - Test replace cell content
  - Test insert new cell at position
  - Test insert at beginning (no cell_id)
  - Test delete cell
  - Test invalid cell number → error
  - Test non-.ipynb file → error
  - Test preserves notebook metadata and other cells

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/tools/... -run TestNotebookEdit` passes

---

### Phase 11: Agent Tool (Interface + Stub)
**Goal**: Implement the Agent/Task tool that delegates to a `SubagentSpawner` interface. Real subagent manager deferred to Spec 08.

**Changes**:
- `pkg/tools/agenttool.go`:
  ```
  Name: "Agent"
  Input: description (required), prompt (required), subagent_type (required),
         model (optional), resume (optional), run_in_background (optional),
         max_turns (optional)
  Execution: Calls SubagentSpawner.Spawn(ctx, input)
  SideEffect: SideEffectSpawns (new side effect type)
  ```
  - Define `SubagentSpawner` interface:
    ```go
    type SubagentSpawner interface {
        Spawn(ctx context.Context, input AgentInput) (AgentResult, error)
    }
    type AgentInput struct {
        Description     string
        Prompt          string
        SubagentType    string
        Model           *string
        Resume          *string
        RunInBackground *bool
        MaxTurns        *int
    }
    type AgentResult struct {
        AgentID string
        Output  string // final output (empty if background)
    }
    ```
  - Default: `StubSubagentSpawner` that returns "Subagent spawning not yet configured"
  - Add `SideEffectSpawns` to `SideEffectType` enum in `tool.go`
- `pkg/tools/agenttool_test.go`:
  - Test with mock spawner → returns output
  - Test stub spawner → returns not-configured message
  - Test run_in_background → returns agent ID
  - Test validates required fields

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/tools/... -run TestAgent` passes

---

### Phase 12: MCP Tools (Interface + Stub)
**Goal**: Implement ListMcpResources, ReadMcpResource, and the dynamic `mcp__*` tool pattern. Real MCP client deferred to Spec 10.

**Changes**:
- `pkg/tools/mcp.go` — MCP tool implementations:
  - Define `MCPClient` interface:
    ```go
    type MCPClient interface {
        ListResources(ctx context.Context, serverName string) ([]MCPResource, error)
        ReadResource(ctx context.Context, serverName, uri string) (string, error)
        CallTool(ctx context.Context, serverName, toolName string, args map[string]any) (string, error)
    }
    type MCPResource struct {
        URI         string
        Name        string
        Description string
        MimeType    string
    }
    ```
  - `ListMcpResourcesTool`:
    ```
    Name: "ListMcpResources"
    Input: server_name (optional — lists all if empty)
    Execution: Calls MCPClient.ListResources
    SideEffect: SideEffectNone
    ```
  - `ReadMcpResourceTool`:
    ```
    Name: "ReadMcpResource"
    Input: server_name (required), uri (required)
    Execution: Calls MCPClient.ReadResource
    SideEffect: SideEffectNone
    ```
  - Default: `StubMCPClient` that returns "MCP not configured"
- `pkg/tools/mcptool.go` — Dynamic MCP tool:
  - `MCPTool` struct that represents a single MCP server tool
  - Registered dynamically via `Registry.RegisterMCPTool(serverName, toolName, description, schema)`
  - Name format: `mcp__{serverName}__{toolName}`
  - Execution delegates to `MCPClient.CallTool(serverName, toolName, args)`
- Add to `Registry`:
  - `RegisterMCPTool(serverName, toolName, description string, schema map[string]any)`
  - `UnregisterMCPTools(serverName string)` — removes all tools for a server
- `pkg/tools/mcp_test.go`:
  - Test ListMcpResources with mock client
  - Test ReadMcpResource with mock client
  - Test stub client returns helpful message
- `pkg/tools/mcptool_test.go`:
  - Test RegisterMCPTool adds tool to registry
  - Test UnregisterMCPTools removes tools
  - Test MCPTool execution delegates to client
  - Test tool name format `mcp__server__tool`

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/tools/... -run TestMCP` passes
  - [x] `go test ./pkg/tools/... -run TestMCPTool` passes

---

### Phase 13: Registry Updates & DefaultRegistry
**Goal**: Wire all new tools into the registry and update `DefaultRegistry()`.

**Changes**:
- `pkg/agent/defaults.go` — Update `DefaultRegistry()` to register all 19 tools:
  ```go
  func DefaultRegistry(cwd string) *tools.Registry {
      tm := tools.NewTaskManager()

      registry := tools.NewRegistry(
          tools.WithAllowed("Read", "Glob", "Grep", "ListMcpResources", "ReadMcpResource"),
      )

      // Core 6 (existing)
      registry.Register(&tools.BashTool{CWD: cwd, TaskManager: tm})
      registry.Register(&tools.FileReadTool{})
      registry.Register(&tools.FileWriteTool{})
      registry.Register(&tools.FileEditTool{})
      registry.Register(&tools.GlobTool{CWD: cwd})
      registry.Register(&tools.GrepTool{CWD: cwd})

      // Background task tools
      registry.Register(&tools.TaskOutputTool{TaskManager: tm})
      registry.Register(&tools.TaskStopTool{TaskManager: tm})

      // State management tools
      registry.Register(&tools.TodoWriteTool{})
      registry.Register(&tools.ConfigTool{Store: tools.NewInMemoryConfigStore()})
      registry.Register(&tools.ExitPlanModeTool{})
      registry.Register(&tools.AskUserQuestionTool{}) // Handler set by host app

      // Network tools
      registry.Register(&tools.WebFetchTool{})
      registry.Register(&tools.WebSearchTool{}) // Provider set by host app

      // Subagent
      registry.Register(&tools.AgentTool{}) // Spawner set by host app

      // MCP (stub until Spec 10)
      registry.Register(&tools.ListMcpResourcesTool{})
      registry.Register(&tools.ReadMcpResourceTool{})
      // Dynamic mcp__* tools registered at runtime via RegisterMCPTool

      // NotebookEdit
      registry.Register(&tools.NotebookEditTool{})

      return registry
  }
  ```
- Update `pkg/agent/config.go` — No changes needed; tools are injected via registry
- Verify all existing agent tests still pass with expanded registry

**Success Criteria**:
- Automated:
  - [x] `go test ./...` passes (entire project)
  - [x] `go test -race ./...` passes
  - [x] `go vet ./...` passes
  - [x] 18 static tools appear in `DefaultRegistry().Names()` (mcp__* tools are dynamic, registered at runtime)
- Manual:
  - [ ] `ToolDefinitions()` returns valid OpenAI-format definitions for all 18 static tools

---

### Phase 14: Integration Tests
**Goal**: Verify the new tools work end-to-end with the agent loop.

**Changes**:
- `pkg/agent/loop_test.go` — Add test cases:
  - `TestLoop_BackgroundBash` — Bash with `run_in_background: true` → returns task ID → TaskOutput retrieves result
  - `TestLoop_TodoWrite` — LLM calls TodoWrite → updates in-memory list → returns confirmation
  - `TestLoop_WebFetch` — LLM calls WebFetch → httptest serves HTML → extracts text → returns to LLM
  - `TestLoop_UnknownTool_MCPPrefix` — LLM calls `mcp__foo__bar` when no MCP tools registered → appropriate error
- Add dependency: `golang.org/x/net` (for HTML tokenizer in WebFetch)

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/agent/... -run TestLoop` passes (all existing + new cases)
  - [x] `go test -race ./...` passes
  - [x] `go vet ./...` passes

---

## File Layout Summary

```
pkg/tools/
├── adapter.go            (existing, no changes)
├── agenttool.go          (NEW — Agent/Task tool with SubagentSpawner interface)
├── agenttool_test.go     (NEW)
├── askuser.go            (NEW — AskUserQuestion with UserInputHandler interface)
├── askuser_test.go       (NEW)
├── bash.go               (MODIFIED — add TaskManager + run_in_background)
├── bash_test.go          (MODIFIED — add background tests)
├── config.go             (NEW — Config tool with ConfigStore interface)
├── config_test.go        (NEW)
├── exitplanmode.go       (NEW — ExitPlanMode tool)
├── exitplanmode_test.go  (NEW)
├── fileedit.go           (existing, no changes)
├── fileedit_test.go      (existing, no changes)
├── fileread.go           (existing, no changes)
├── fileread_test.go      (existing, no changes)
├── filewrite.go          (existing, no changes)
├── filewrite_test.go     (existing, no changes)
├── glob.go               (existing, no changes)
├── glob_test.go          (existing, no changes)
├── grep.go               (existing, no changes)
├── grep_test.go          (existing, no changes)
├── mcp.go                (NEW — ListMcpResources, ReadMcpResource, MCPClient interface)
├── mcp_test.go           (NEW)
├── mcptool.go            (NEW — Dynamic mcp__* tool, RegisterMCPTool on Registry)
├── mcptool_test.go       (NEW)
├── notebookedit.go       (NEW — NotebookEdit tool)
├── notebookedit_test.go  (NEW)
├── registry.go           (MODIFIED — add RegisterMCPTool, UnregisterMCPTools)
├── registry_test.go      (MODIFIED — add MCP registration tests)
├── taskmanager.go        (NEW — Background task lifecycle manager)
├── taskmanager_test.go   (NEW)
├── taskoutput.go         (NEW — TaskOutput tool)
├── taskoutput_test.go    (NEW)
├── taskstop.go           (NEW — TaskStop tool)
├── taskstop_test.go      (NEW)
├── todowrite.go          (NEW — TodoWrite tool)
├── todowrite_test.go     (NEW)
├── tool.go               (MODIFIED — add SideEffectSpawns)
├── webfetch.go           (NEW — WebFetch with real HTTP + HTML extraction)
├── webfetch_test.go      (NEW)
├── websearch.go          (NEW — WebSearch with SearchProvider interface)
└── websearch_test.go     (NEW)

pkg/agent/
├── defaults.go           (MODIFIED — register all 19 tools)
└── loop_test.go          (MODIFIED — add integration test cases)
```

## Dependencies
- `golang.org/x/net` — HTML tokenizer for WebFetch tool (add to go.mod)
- No other new external dependencies

## Out of Scope
- Full subagent manager implementation (Spec 08) — interface + stub only for Agent tool
- Full MCP client implementation (Spec 10) — interface + stub only for MCP tools
- Real search provider (Brave, SearXNG, etc.) — interface only, stub default
- WebFetch summarizer subagent — returns raw extracted text; LLM reasons about prompt
- PDF support in FileRead — separate enhancement
- Go regexp fallback for Grep — ripgrep still required
- Plan mode state machine — ExitPlanMode returns a marker; loop handling deferred
- Session persistence of background tasks — tasks are in-memory only
- Concurrent tool execution (tools still run serially in the agent loop)

## Open Questions
*None — all resolved.*

## References
- Spec 04: `thoughts/specs/04-TOOL-REGISTRY.md`
- Spec 08: `thoughts/specs/08-SUBAGENT-MANAGER.md` (Agent tool interface reference)
- Spec 10: `thoughts/specs/10-MCP-INTEGRATION.md` (MCP tool interface reference)
- Previous plan: `thoughts/plans/2026-02-08-agentic-loop.md`
- Existing code: `pkg/tools/`, `pkg/agent/`
