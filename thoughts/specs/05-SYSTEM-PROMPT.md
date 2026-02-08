# Spec 05: System Prompt Assembly

**Go Package**: `pkg/prompt/`
**Source References**:
- `sdk.d.ts:776-800` — `systemPrompt` option (string | preset with append)
- Piebald-AI/claude-code-system-prompts — Complete catalog of ~60 prompt parts with token counts
- Piebald-AI README: "Claude Code doesn't just have one single string for its system prompt... Large portions conditionally added depending on the environment and various configs."

---

## 1. Purpose

Claude Code's system prompt is not a single string. It is assembled from multiple parts, conditionally included based on configuration, available tools, environment, and agent context. The prompt assembler must:

1. Compose the base system prompt from ordered parts
2. Inject tool descriptions for enabled tools
3. Append CLAUDE.md project instructions
4. Include agent-specific prompts for subagents
5. Apply user customizations (append text, output style)
6. Respect the system prompt option override

---

## 2. System Prompt Configuration (from `sdk.d.ts:776-800`)

```go
// SystemPromptConfig matches the SDK's systemPrompt option.
type SystemPromptConfig struct {
    // Mode 1: Full custom replacement
    Custom string

    // Mode 2: Preset with optional append
    Preset string // "claude_code"
    Append string // additional instructions appended to default
}

// Determines prompt assembly strategy:
// - If Custom != "": use Custom verbatim (no assembly)
// - If Preset == "claude_code": assemble full prompt, then append Append
// - If neither: assemble full prompt (default behavior)
```

---

## 3. Prompt Part Catalog (from Piebald-AI)

### 3.1 Main System Prompt Parts (assembled in order)

| Order | Part | Tokens | Condition |
|-------|------|--------|-----------|
| 1 | Main system prompt | 269 | Always |
| 2 | Accessing past sessions | 352 | Sessions enabled |
| 3 | Agent memory instructions | 337 | Memory enabled |
| 4 | Censoring malicious activities | 98 | Always (security policy) |
| 5 | CLAUDE.md instructions | 295 | CLAUDE.md exists |
| 6 | Conversational style | 236 | Always |
| 7 | Custom slash commands info | 155 | Commands registered |
| 8 | Do not litter output | 81 | Always |
| 9 | Environment details | 387 | Always (OS, shell, CWD) |
| 10 | File read limits | 34 | Always |
| 11 | Glob documentation | 205 | Glob tool enabled |
| 12 | Grep documentation | 235 | Grep tool enabled |
| 13 | Large file handling | 187 | Always |
| 14 | MCP connected servers | ~varies | MCP servers configured |
| 15 | Proactive suggestions | 179 | Always |
| 16 | Respecting conventions | 98 | Always |
| 17 | Search/replace blocks | 112 | FileEdit tool enabled |
| 18 | Todo tool | 247 | TodoWrite tool enabled |
| 19 | Tool use formatting | 53 | Always |
| 20 | Use tools to verify | 85 | Always |

### 3.2 Tool Descriptions (injected into tools array, not system prompt)

Tool descriptions are sent as the `description` field in each tool definition, not concatenated into the system prompt. The Piebald catalog shows these separately:

| Tool | Description Tokens |
|------|-------------------|
| Bash | 489 |
| FileRead (Read) | 374 |
| FileWrite (Write) | 277 |
| FileEdit (Edit) | 536 |
| Glob | 179 |
| Grep | 548 |
| Agent (Task) | 294 |
| WebSearch | 138 |
| WebFetch | 196 |
| TodoWrite | 152 |
| NotebookEdit | 231 |
| AskUserQuestion | 186 |
| ListMcpResources | 72 |
| ReadMcpResource | 81 |
| Config | 94 |

### 3.3 Agent/Subagent Prompts

| Agent | Tokens | Used When |
|-------|--------|-----------|
| Explore | 516 | /explore command |
| Plan mode (enhanced) | 633 | plan permission mode |
| Task tool | 294 | Task/Agent tool spawns subagent |
| WebFetch summarizer | 185 | WebFetch tool processes results |
| Agent creation architect | 1110 | /agent-create command |
| CLAUDE.md creation | 384 | /init command |
| Status line setup | 1460 | Status line configuration |
| Remember skill | 1048 | /remember command |
| Session Search Assistant | 439 | Session search |
| Session memory update | 756 | Memory updates |
| Session title/branch | 307 | Auto-title generation |
| Update Magic Docs | 718 | Magic docs refresh |
| User sentiment analysis | 205 | Sentiment monitoring |
| /pr-comments | 402 | PR comment review |
| /review-pr | 243 | PR review |
| /security-review | 2610 | Security audit |
| Agent Summary Generation | 184 | Agent summary |

### 3.4 System Reminders (~40 reminders, conditionally triggered)

Reminders are injected by the runtime based on classifier triggers or conversation conditions. They are appended to the user's message, not the system prompt.

---

## 4. Assembly Algorithm

```go
// AssembleSystemPrompt builds the complete system prompt from parts.
func AssembleSystemPrompt(config PromptAssemblyConfig) string {
    // Check for custom override
    if config.SystemPrompt.Custom != "" {
        return config.SystemPrompt.Custom
    }

    var parts []string

    // 1. Core identity
    parts = append(parts, renderMainPrompt(config))

    // 2. Security policy (always included)
    parts = append(parts, renderSecurityPolicy())
    parts = append(parts, renderCensoringPolicy())

    // 3. Environment details
    parts = append(parts, renderEnvironmentDetails(config.CWD, config.OS, config.Shell))

    // 4. Session/memory instructions (conditional)
    if config.SessionsEnabled {
        parts = append(parts, renderPastSessionsInstructions())
    }
    if config.MemoryEnabled {
        parts = append(parts, renderAgentMemoryInstructions())
    }

    // 5. Tool-specific documentation (conditional on enabled tools)
    if config.ToolEnabled("Glob") {
        parts = append(parts, renderGlobDocs())
    }
    if config.ToolEnabled("Grep") {
        parts = append(parts, renderGrepDocs())
    }
    if config.ToolEnabled("FileEdit") {
        parts = append(parts, renderSearchReplaceDocs())
    }
    if config.ToolEnabled("TodoWrite") {
        parts = append(parts, renderTodoDocs())
    }

    // 6. MCP server documentation
    if len(config.MCPServers) > 0 {
        parts = append(parts, renderMCPServerDocs(config.MCPServers))
    }

    // 7. Custom slash commands
    if len(config.SlashCommands) > 0 {
        parts = append(parts, renderSlashCommandDocs(config.SlashCommands))
    }

    // 8. Behavioral instructions (always)
    parts = append(parts, renderConversationalStyle())
    parts = append(parts, renderProactiveSuggestions())
    parts = append(parts, renderRespectConventions())
    parts = append(parts, renderToolUseFormatting())
    parts = append(parts, renderUseToolsToVerify())
    parts = append(parts, renderDoNotLitterOutput())
    parts = append(parts, renderLargeFileHandling())
    parts = append(parts, renderFileReadLimits())

    // 9. CLAUDE.md project instructions
    if config.ClaudeMDContent != "" {
        parts = append(parts, renderClaudeMDInstructions(config.ClaudeMDContent))
    }

    // 10. Output style (if configured)
    if config.OutputStyle != "" {
        parts = append(parts, renderOutputStyle(config.OutputStyle))
    }

    // 11. User append (from systemPrompt preset config)
    if config.SystemPrompt.Append != "" {
        parts = append(parts, config.SystemPrompt.Append)
    }

    return strings.Join(parts, "\n\n")
}
```

---

## 5. Template Interpolation

The main system prompt contains interpolation variables (from Piebald extraction):

```
You are an interactive CLI tool that helps users
${OUTPUT_STYLE_CONFIG !== null ? 'according to your "Output Style" below...' : "with software engineering tasks."}
Use the instructions below and the tools available to you to assist the user.
${SECURITY_POLICY}
```

Go implementation:

```go
type PromptVars struct {
    OutputStyleConfig *string // nil if no output style
    SecurityPolicy    string
    IssuesExplainer   string
    PackageURL        string
    ReadmeURL         string
    Version           string
    FeedbackChannel   string
    BuildTime         string
}

func renderMainPrompt(config PromptAssemblyConfig) string {
    tmpl := `You are an interactive CLI tool that helps users {{if .OutputStyleConfig}}according to your "Output Style" below, which describes how you should respond to user queries.{{else}}with software engineering tasks.{{end}} Use the instructions below and the tools available to you to assist the user.

{{.SecurityPolicy}}

IMPORTANT: You must NEVER generate or guess URLs for the user unless you are confident that the URLs are for helping the user with programming. You may use URLs provided by the user in their messages or local files.`

    return executeTemplate(tmpl, config.Vars)
}
```

---

## 6. CLAUDE.md Loading

CLAUDE.md files are loaded from the project directory hierarchy:

```go
func loadClaudeMDFiles(cwd string) string {
    var files []string

    // Walk up from CWD to find CLAUDE.md files
    // Priority: project root > subdirectory
    // Also check: .claude/CLAUDE.md, CLAUDE.local.md

    paths := []string{
        filepath.Join(cwd, "CLAUDE.md"),
        filepath.Join(cwd, ".claude", "CLAUDE.md"),
        filepath.Join(cwd, "CLAUDE.local.md"),
    }

    for _, p := range paths {
        if content, err := os.ReadFile(p); err == nil {
            files = append(files, string(content))
        }
    }

    // Walk up to find parent CLAUDE.md files
    parent := filepath.Dir(cwd)
    for parent != cwd {
        p := filepath.Join(parent, "CLAUDE.md")
        if content, err := os.ReadFile(p); err == nil {
            files = append(files, string(content))
        }
        cwd = parent
        parent = filepath.Dir(parent)
    }

    return strings.Join(files, "\n\n---\n\n")
}
```

---

## 7. Subagent Prompt Assembly

Subagents use different prompt assembly. The AgentDefinition's `prompt` field replaces the main system prompt:

```go
func AssembleSubagentPrompt(agent AgentDefinition, parentConfig PromptAssemblyConfig) string {
    var parts []string

    // 1. Agent's custom prompt
    parts = append(parts, agent.Prompt)

    // 2. Critical reminder (experimental)
    if agent.CriticalSystemReminder != "" {
        parts = append(parts, "CRITICAL REMINDER: " + agent.CriticalSystemReminder)
    }

    // 3. Skills (preloaded context)
    for _, skill := range agent.Skills {
        if content, ok := loadSkill(skill); ok {
            parts = append(parts, content)
        }
    }

    // 4. Subset of parent environment info
    parts = append(parts, renderEnvironmentDetails(parentConfig.CWD, parentConfig.OS, parentConfig.Shell))

    return strings.Join(parts, "\n\n")
}
```

---

## 8. Token Budget Awareness

The assembled prompt counts toward the context window. Track approximate token counts:

```go
// EstimateTokens returns approximate token count for a string.
// Uses the ~4 chars per token heuristic for Claude models.
func EstimateTokens(text string) int {
    return len(text) / 4
}

// Warn if system prompt exceeds budget
func (a *Assembler) Validate() error {
    prompt := a.Assemble()
    tokens := EstimateTokens(prompt)
    if tokens > a.config.MaxSystemPromptTokens {
        return fmt.Errorf("system prompt %d tokens exceeds budget %d",
            tokens, a.config.MaxSystemPromptTokens)
    }
    return nil
}
```

---

## 9. Prompt Storage

Prompts are stored as Go embedded files for versioning and fast access:

```go
//go:embed prompts/main_system_prompt.txt
var mainSystemPrompt string

//go:embed prompts/security_policy.txt
var securityPolicy string

//go:embed prompts/censoring_policy.txt
var censoringPolicy string

// ... etc for all prompt parts
```

This allows:
- Version-pinned prompts matching a specific Claude Code version
- No runtime file I/O for prompt assembly
- Easy diffing between versions

---

## 10. Verification Checklist

- [ ] **Part ordering**: Assembled prompt part order matches Piebald-AI catalog ordering
- [ ] **Conditional inclusion**: Tool docs only included when tool is enabled
- [ ] **Interpolation**: All template variables resolve correctly
- [ ] **CLAUDE.md hierarchy**: Files loaded in correct priority (local > project > parent)
- [ ] **Token budget**: Assembled prompt stays within context budget
- [ ] **Custom override**: `systemPrompt: "custom"` completely replaces assembled prompt
- [ ] **Preset + append**: `{preset: "claude_code", append: "..."}` appends correctly
- [ ] **Subagent isolation**: Subagent prompts don't include parent's CLAUDE.md unless specified
- [ ] **MCP docs**: Connected MCP server tools appear in system prompt documentation
- [ ] **Output style**: OutputStyle config injected into main prompt template
- [ ] **Version pinning**: Prompt text matches specific Claude Code version (v2.1.37)
