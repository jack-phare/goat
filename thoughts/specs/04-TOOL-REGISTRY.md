# Spec 04: Tool Registry & Execution

**Go Package**: `pkg/tools/`
**Source References**:
- `sdk-tools.d.ts:1-1570` — All 19 tool input schema types (ToolInputSchemas union)
- `sdk.d.ts:536-542` — Tools option (string[] | preset)
- `sdk.d.ts:529` — DisallowedTools
- `sdk.d.ts:504` — AllowedTools (auto-allowed)
- Piebald-AI: 18 builtin tool descriptions with token counts

---

## 1. Purpose

The tool registry manages the catalog of available tools, their JSON schemas (for the LLM's `tools` parameter), input validation, and execution. Each tool has:
1. A name (e.g., `"Bash"`, `"FileRead"`)
2. A description (injected into the system prompt / tools array)
3. An input JSON schema (from `sdk-tools.d.ts`)
4. An executor function

---

## 2. Tool Catalog

### 2.1 Complete Tool List (from `sdk-tools.d.ts` ToolInputSchemas union)

| # | Tool Name | Input Type | Side Effects | Description |
|---|-----------|-----------|--------------|-------------|
| 1 | `Agent` (Task) | `AgentInput` | Spawns subagent | Spawn a subagent for delegated tasks |
| 2 | `Bash` | `BashInput` | Mutating | Execute shell commands |
| 3 | `TaskOutput` | `TaskOutputInput` | None | Read output from background task |
| 4 | `ExitPlanMode` | `ExitPlanModeInput` | State change | Exit plan mode with permissions |
| 5 | `FileEdit` | `FileEditInput` | Mutating | Find-and-replace in files |
| 6 | `FileRead` (Read) | `FileReadInput` | None | Read file contents |
| 7 | `FileWrite` (Write) | `FileWriteInput` | Mutating | Create/overwrite files |
| 8 | `Glob` | `GlobInput` | None | Find files by pattern |
| 9 | `Grep` | `GrepInput` | None | Search file contents |
| 10 | `TaskStop` | `TaskStopInput` | State change | Stop a background task |
| 11 | `ListMcpResources` | `ListMcpResourcesInput` | None | List MCP server resources |
| 12 | `mcp__*` | `McpInput` | Varies | Dynamic MCP tool calls |
| 13 | `NotebookEdit` | `NotebookEditInput` | Mutating | Edit Jupyter notebooks |
| 14 | `ReadMcpResource` | `ReadMcpResourceInput` | None | Read MCP resource by URI |
| 15 | `TodoWrite` | `TodoWriteInput` | State change | Update task/todo list |
| 16 | `WebFetch` | `WebFetchInput` | Network | Fetch URL content |
| 17 | `WebSearch` | `WebSearchInput` | Network | Web search |
| 18 | `AskUserQuestion` | `AskUserQuestionInput` | Blocks | Ask user multiple-choice questions |
| 19 | `Config` | `ConfigInput` | State change | Get/set configuration values |

---

## 3. Go Types

### 3.1 Tool Interface

```go
// Tool defines the contract every tool must implement.
type Tool interface {
    Name() string
    Description() string
    InputSchema() json.RawMessage         // JSON Schema for the tools array
    SideEffect() SideEffectType
    Execute(ctx context.Context, input json.RawMessage) (ToolOutput, error)
}

type SideEffectType int
const (
    SideEffectNone     SideEffectType = iota
    SideEffectReadOnly
    SideEffectMutating
    SideEffectNetwork
    SideEffectBlocking
    SideEffectSpawns
)

// ToolOutput is the result sent back as tool_result content.
type ToolOutput struct {
    Content string // text content for the tool_result
    IsError bool   // maps to is_error in tool_result content block
}
```

### 3.2 Registry

```go
// Registry holds available tools and resolves them by name.
type Registry struct {
    tools     map[string]Tool
    allowed   map[string]bool // auto-allowed tools (no permission prompt)
    disabled  map[string]bool // explicitly disallowed
}

func NewRegistry(config ToolRegistryConfig) *Registry
func (r *Registry) Get(name string) (Tool, bool)
func (r *Registry) ToolDefinitions() []ToolDefinition  // for LLM tools param
func (r *Registry) IsAllowed(name string) bool
func (r *Registry) IsDisabled(name string) bool

// ToolDefinition is the OpenAI-format tool definition for LiteLLM.
type ToolDefinition struct {
    Type     string             `json:"type"`     // always "function"
    Function ToolFunctionDef    `json:"function"`
}

type ToolFunctionDef struct {
    Name        string          `json:"name"`
    Description string          `json:"description"`
    Parameters  json.RawMessage `json:"parameters"` // JSON Schema
}
```

---

## 4. Tool Input Schemas (from `sdk-tools.d.ts`)

### 4.1 BashInput (`sdk-tools.d.ts:79-113`)

```go
type BashInput struct {
    Command     string  `json:"command" validate:"required"`
    Timeout     *int    `json:"timeout,omitempty"`     // max 600000ms
    Description *string `json:"description,omitempty"` // human-readable
    RunInBackground *bool `json:"run_in_background,omitempty"`
}
```

**Execution**: `exec.CommandContext` with configurable timeout (default 120s, max 600s). Captures stdout+stderr combined. Returns truncated output if exceeds token limit.

### 4.2 FileReadInput (`sdk-tools.d.ts:166-185`)

```go
type FileReadInput struct {
    FilePath string  `json:"file_path" validate:"required,filepath"`
    Offset   *int    `json:"offset,omitempty"`  // start line (1-indexed)
    Limit    *int    `json:"limit,omitempty"`   // max lines
    Pages    *string `json:"pages,omitempty"`   // PDF page range "1-5"
}
```

**Execution**: Read file from disk. For PDFs, extract text with page range support (max 20 pages). Returns numbered lines with offset/limit support.

### 4.3 FileWriteInput (`sdk-tools.d.ts:186-196`)

```go
type FileWriteInput struct {
    FilePath string `json:"file_path" validate:"required,filepath"`
    Content  string `json:"content" validate:"required"`
}
```

**Execution**: Write content to file, creating parent directories as needed. Returns success confirmation with line count.

### 4.4 FileEditInput (`sdk-tools.d.ts:151-165`)

```go
type FileEditInput struct {
    FilePath   string `json:"file_path" validate:"required,filepath"`
    OldString  string `json:"old_string" validate:"required"`
    NewString  string `json:"new_string" validate:"required"`
    ReplaceAll *bool  `json:"replace_all,omitempty"` // default false
}
```

**Execution**: Find `old_string` in file, replace with `new_string`. If `replace_all` is false, must match exactly once (error if 0 or >1 matches). Returns diff snippet.

### 4.5 GlobInput (`sdk-tools.d.ts:197-208`)

```go
type GlobInput struct {
    Pattern string  `json:"pattern" validate:"required"`
    Path    *string `json:"path,omitempty"` // default CWD
}
```

**Execution**: `filepath.Glob` or `doublestar` library for recursive patterns. Returns sorted file list.

### 4.6 GrepInput (`sdk-tools.d.ts:209-271`)

```go
type GrepInput struct {
    Pattern    string  `json:"pattern" validate:"required"`
    Path       *string `json:"path,omitempty"`       // default CWD
    Glob       *string `json:"glob,omitempty"`        // file filter "*.go"
    OutputMode *string `json:"output_mode,omitempty"` // "content"|"files_with_matches"|"count"
    Before     *int    `json:"-B,omitempty"`           // context lines before
    After      *int    `json:"-A,omitempty"`           // context lines after
    Context    *int    `json:"-C,omitempty"`           // context both sides
    LineNumbers *bool  `json:"-n,omitempty"`           // default true
    CaseInsensitive *bool `json:"-i,omitempty"`
    FileType   *string `json:"type,omitempty"`         // rg --type
    HeadLimit  *int    `json:"head_limit,omitempty"`   // limit output lines
    Offset     *int    `json:"offset,omitempty"`       // skip first N
    Multiline  *bool   `json:"multiline,omitempty"`
}
```

**Execution**: Shell out to `ripgrep` (`rg`) with mapped flags. Falls back to Go `regexp` if `rg` not available.

### 4.7 AgentInput (`sdk-tools.d.ts:32-73`)

```go
type AgentInput struct {
    Description     string  `json:"description" validate:"required"`
    Prompt          string  `json:"prompt" validate:"required"`
    SubagentType    string  `json:"subagent_type" validate:"required"`
    Model           *string `json:"model,omitempty"`   // "sonnet"|"opus"|"haiku"
    Resume          *string `json:"resume,omitempty"`  // agent ID to resume
    RunInBackground *bool   `json:"run_in_background,omitempty"`
    MaxTurns        *int    `json:"max_turns,omitempty"`
    Name            *string `json:"name,omitempty"`
    TeamName        *string `json:"team_name,omitempty"`
    Mode            *string `json:"mode,omitempty"`    // PermissionMode
}
```

**Execution**: Spawns a new agentic loop as a subagent (see Spec 08).

### 4.8 WebSearchInput (`sdk-tools.d.ts:346-361`)

```go
type WebSearchInput struct {
    Query          string   `json:"query" validate:"required"`
    AllowedDomains []string `json:"allowed_domains,omitempty"`
    BlockedDomains []string `json:"blocked_domains,omitempty"`
}
```

**Execution**: Delegates to a search provider (configurable). Returns formatted results.

### 4.9 WebFetchInput (`sdk-tools.d.ts:335-345`)

```go
type WebFetchInput struct {
    URL    string `json:"url" validate:"required,url"`
    Prompt string `json:"prompt" validate:"required"`
}
```

**Execution**: HTTP GET, extract text, run through a summarizer subagent with the given prompt. Piebald: "Agent Prompt: WebFetch summarizer (185 tks)".

### 4.10 TodoWriteInput (`sdk-tools.d.ts:316-334`)

```go
type TodoWriteInput struct {
    Todos []TodoItem `json:"todos" validate:"required"`
}

type TodoItem struct {
    Content    string `json:"content" validate:"required"`
    Status     string `json:"status" validate:"required,oneof=pending in_progress completed"`
    ActiveForm string `json:"activeForm" validate:"required"`
}
```

**Execution**: Writes/updates a structured todo list. Emitted as `SDKTodoMessage` to observers.

### 4.11 AskUserQuestionInput (`sdk-tools.d.ts:362-540`)

```go
type AskUserQuestionInput struct {
    Questions []QuestionSpec `json:"questions" validate:"required,min=1,max=4"`
    Answers   map[string]string `json:"answers,omitempty"` // pre-filled answers
}

type QuestionSpec struct {
    Question    string       `json:"question" validate:"required"`
    Header      string       `json:"header" validate:"required,max=12"`
    Options     []OptionSpec `json:"options" validate:"required,min=2,max=4"`
    MultiSelect bool         `json:"multiSelect"`
}

type OptionSpec struct {
    Label       string `json:"label" validate:"required"`
    Description string `json:"description" validate:"required"`
}
```

**Execution**: Blocks until the user responds via the host application's permission/input callback.

### 4.12 ConfigInput (`sdk-tools.d.ts:1560-1570`)

```go
type ConfigInput struct {
    Setting string       `json:"setting" validate:"required"`
    Value   *interface{} `json:"value,omitempty"` // string|bool|number; nil = read
}
```

**Execution**: Get or set a runtime configuration value.

---

## 5. Tool Definition Generation

For each registered tool, generate an OpenAI-format tool definition for the LiteLLM request:

```go
func (t *BashTool) ToolDefinition() ToolDefinition {
    return ToolDefinition{
        Type: "function",
        Function: ToolFunctionDef{
            Name:        "Bash",
            Description: "Execute a bash command...", // from Piebald tool descriptions
            Parameters: json.RawMessage(`{
                "type": "object",
                "properties": {
                    "command": {"type": "string", "description": "The command to execute"},
                    "timeout": {"type": "integer", "description": "Optional timeout in milliseconds (max 600000)"},
                    "description": {"type": "string"}
                },
                "required": ["command"]
            }`),
        },
    }
}
```

---

## 6. Tool Output Formatting

Tool results are returned as `tool_result` content blocks. For LiteLLM, each becomes a separate `"role": "tool"` message:

```json
{
    "role": "tool",
    "tool_call_id": "call_abc123",
    "content": "file1.go\nfile2.go\nfile3.go"
}
```

For errors:
```json
{
    "role": "tool",
    "tool_call_id": "call_abc123",
    "content": "Error: file not found: /nonexistent/path"
}
```

Note: LiteLLM does not support `is_error` on tool messages. Errors must be communicated via content text prefixed with "Error: ".

---

## 7. MCP Tool Dynamic Registration

MCP tools are registered dynamically when MCP servers connect. Their names are prefixed: `mcp__{serverName}__{toolName}`.

```go
func (r *Registry) RegisterMCPTool(serverName, toolName, description string, schema json.RawMessage) {
    fullName := fmt.Sprintf("mcp__%s__%s", serverName, toolName)
    r.tools[fullName] = &MCPTool{
        name:        fullName,
        description: description,
        schema:      schema,
        serverName:  serverName,
        toolName:    toolName,
    }
}
```

---

## 8. Input Validation

All tool inputs are validated before execution:

```go
func validateInput(toolName string, input json.RawMessage) error {
    // 1. Unmarshal into the correct struct
    // 2. Run struct validation tags
    // 3. Tool-specific validation:
    //    - BashInput: timeout <= 600000
    //    - FileReadInput: file_path is absolute
    //    - FileEditInput: old_string != new_string
    //    - GrepInput: output_mode is valid enum
    //    - AgentInput: model is valid enum if set
}
```

---

## 9. Verification Checklist

- [ ] **Schema completeness**: All 19 tool input types from `sdk-tools.d.ts` have Go struct equivalents
- [ ] **JSON round-trip**: Each input struct marshals/unmarshals identically to the TypeScript schema
- [ ] **Tool definition format**: Generated tool definitions pass LiteLLM's `/v1/chat/completions` validation
- [ ] **Bash timeout**: Enforced at max 600000ms (10 minutes)
- [ ] **FileEdit uniqueness**: Returns error when `old_string` matches 0 or >1 times (unless `replace_all`)
- [ ] **Grep flags**: All `rg` flag mappings produce identical output to Claude Code's Grep tool
- [ ] **MCP tool naming**: `mcp__{server}__{tool}` convention matches SDK
- [ ] **Error format**: Tool errors are returned as content text (not structured error), matching SDK behavior
- [ ] **PDF support**: FileRead handles PDF page ranges correctly
- [ ] **Path validation**: All file tools reject relative paths
- [ ] **Background tasks**: Bash and Agent tools support `run_in_background` with task ID tracking
