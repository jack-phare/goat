# System Prompt Assembly (Spec 05) Implementation Plan

## Overview
Implement the `pkg/prompt/` package that replaces `StaticPromptAssembler` with a full conditional prompt assembly system. The assembler composes system prompts from ~113 embedded markdown files sourced from the Piebald-AI/claude-code-system-prompts repository (v2.1.37), conditionally including parts based on enabled tools, environment, configuration, and agent context.

## Current State
- `SystemPromptAssembler` interface at `pkg/agent/interfaces.go:11-13` — `Assemble(config *AgentConfig) string`
- `StaticPromptAssembler` stub at `pkg/agent/stubs.go:11-17` — returns fixed string
- `types.SystemPromptConfig` at `pkg/types/options.go:93-98` — has `Raw`/`Preset`/`Append` but unused
- `AgentConfig` at `pkg/agent/config.go:10-38` — has `SystemPrompt`, `CWD`, `ToolRegistry`, `Model` fields
- `tools.Registry` exposes `Names()`, `IsDisabled()`, `Get()` for tool presence checks
- Piebald-AI repo cloned at `/tmp/piebald-prompts/` with 113 markdown files
- No `pkg/prompt/` directory exists

## Desired End State
- New `pkg/prompt/` package with `Assembler` struct implementing `SystemPromptAssembler`
- All 113 Piebald-AI prompt files embedded via `//go:embed`
- Conditional assembly based on: enabled tools, OS/shell, sessions/memory, MCP servers, slash commands, git status, plan mode, CLAUDE.md, output style
- CLAUDE.md loading from directory hierarchy (project + parent directories)
- Subagent prompt assembly for Explore, Plan, Task, and other agent types
- Token budget estimation and validation
- Extended `AgentConfig` with new fields needed by the assembler
- Template variable interpolation (Go `text/template`)
- All existing 199 tests continue to pass

## Phases

### Phase 1: Foundation — Embed Prompt Files + Config Extension
**Goal**: Copy all Piebald-AI prompt files into the repo, set up `//go:embed`, and extend `AgentConfig` with fields the assembler needs.

**Changes**:

1. **Create prompt file directory structure**:
   - `pkg/prompt/prompts/agents/` — 29 agent prompt files
   - `pkg/prompt/prompts/system/` — 28 system prompt files
   - `pkg/prompt/prompts/reminders/` — 37 system reminder files
   - `pkg/prompt/prompts/tools/` — 22 tool description files
   - `pkg/prompt/prompts/data/` — 3 data files
   - `pkg/prompt/prompts/skills/` — 3 skill files

2. **Copy all 113 markdown files** from `/tmp/piebald-prompts/system-prompts/` into the corresponding subdirectories, stripping the HTML comment metadata headers (the `<!-- ... -->` blocks).

3. **Create `pkg/prompt/embed.go`** — `//go:embed` directives for all prompt files:
   ```go
   //go:embed prompts/system/*.md
   var systemPrompts embed.FS

   //go:embed prompts/agents/*.md
   var agentPrompts embed.FS

   //go:embed prompts/reminders/*.md
   var reminderPrompts embed.FS

   //go:embed prompts/tools/*.md
   var toolPrompts embed.FS

   //go:embed prompts/data/*.md
   var dataPrompts embed.FS

   //go:embed prompts/skills/*.md
   var skillPrompts embed.FS
   ```
   Plus a `func loadPrompt(fs embed.FS, name string) string` helper that reads and caches.

4. **Extend `AgentConfig`** in `pkg/agent/config.go` — add new fields:
   ```go
   // Environment
   OS    string
   Shell string

   // Feature toggles
   SessionsEnabled bool
   MemoryEnabled   bool
   LearningMode    bool
   ScratchpadDir   string

   // Content
   ClaudeMDContent string   // pre-loaded CLAUDE.md content
   OutputStyle     string   // output style config (empty = default)
   SlashCommands   []string // registered slash command names

   // Git state (snapshot at session start)
   GitBranch        string
   GitMainBranch    string
   GitStatus        string
   GitRecentCommits string

   // MCP
   MCPServers map[string]types.McpServerConfig

   // Agent identity
   AgentType string // "" for main agent, "explore", "plan", "task", etc.

   // Prompt version
   PromptVersion string // e.g., "2.1.37"
   ```

5. **Update `DefaultConfig()`** to set sensible defaults for new fields (OS from `runtime.GOOS`, Shell from `$SHELL`).

**Success Criteria**:
- Automated:
  - [x] `go build ./pkg/prompt/...` succeeds
  - [x] `go vet ./pkg/prompt/...` clean
  - [x] All existing 199 tests pass (`go test -race ./...`)
  - [x] Embedded files load correctly (unit test reads each prompt file)

### Phase 2: Core Assembly Engine
**Goal**: Implement the `Assembler` struct with the main conditional assembly algorithm matching the spec's ordering.

**Changes**:

1. **Create `pkg/prompt/assembler.go`** — the main `Assembler` type:
   ```go
   type Assembler struct{}

   func (a *Assembler) Assemble(config *agent.AgentConfig) string
   ```

   Assembly ordering (matching Piebald-AI catalog and spec Section 4):
   1. Main system prompt (always) — template interpolation for `OUTPUT_STYLE_CONFIG`, `SECURITY_POLICY`, version vars
   2. Censoring policy (always)
   3. Doing tasks (always) — with `TOOL_USAGE_HINTS_ARRAY` interpolation
   4. Executing actions with care (always)
   5. Tool usage policy (always) — with tool name interpolation
   6. Tool permission mode (always)
   7. Tone and style (always) — with `BASH_TOOL_NAME` interpolation
   8. Task management (conditional: TodoWrite enabled)
   9. Parallel tool call note (always)
   10. Accessing past sessions (conditional: `SessionsEnabled`)
   11. Agent memory instructions (conditional: `MemoryEnabled`)
   12. Learning mode (conditional: `LearningMode`)
   13. Git status (conditional: git info available)
   14. MCP CLI (conditional: MCP servers configured)
   15. Scratchpad directory (conditional: `ScratchpadDir != ""`)
   16. Hooks configuration (conditional: hooks configured)
   17. Teammate communication (conditional: swarm mode — defer)
   18. Chrome browser MCP tools (conditional: chrome MCP enabled — defer)
   19. CLAUDE.md instructions (conditional: `ClaudeMDContent != ""`)
   20. Output style (conditional: `OutputStyle != ""`)
   21. User append (from `SystemPromptConfig.Append`)

2. **Create `pkg/prompt/interpolate.go`** — template variable resolution:
   - `renderTemplate(tmpl string, vars PromptVars) string`
   - `PromptVars` struct holding all interpolation variables
   - Uses `strings.ReplaceAll` for simple `${VAR}` substitution (matching the JS interpolation style)
   - Special handling for conditional expressions like `${X !== null ? A : B}`

3. **Create `pkg/prompt/assembler_test.go`** — comprehensive tests:
   - Test assembly with all features enabled
   - Test assembly with minimal config (only "always" parts)
   - Test custom override (`SystemPromptConfig.Raw` bypasses assembly)
   - Test preset + append
   - Test each conditional part is included/excluded correctly
   - Test template interpolation
   - Test tool name substitution

**Success Criteria**:
- Automated:
  - [x] `go test -race ./pkg/prompt/...` all pass
  - [x] Assembler output contains expected sections for each config
  - [x] Custom override returns raw string verbatim
  - [x] All 199+ existing tests pass

### Phase 3: CLAUDE.md Loading
**Goal**: Implement CLAUDE.md file discovery and loading from the directory hierarchy.

**Changes**:

1. **Create `pkg/prompt/claudemd.go`** — CLAUDE.md file loader:
   ```go
   func LoadClaudeMD(cwd string) string
   ```

   Loading order (per spec Section 6):
   - `{cwd}/CLAUDE.md`
   - `{cwd}/.claude/CLAUDE.md`
   - `{cwd}/CLAUDE.local.md`
   - Walk up parent directories for `CLAUDE.md` files
   - Join all found files with `\n\n---\n\n` separator
   - Priority: local > project > parent

2. **Create `pkg/prompt/claudemd_test.go`** — tests:
   - Test with CLAUDE.md at CWD
   - Test with .claude/CLAUDE.md
   - Test with CLAUDE.local.md
   - Test parent directory walking
   - Test multiple files merged correctly
   - Test no CLAUDE.md files found (returns "")
   - Uses `t.TempDir()` for filesystem isolation

**Success Criteria**:
- Automated:
  - [x] `go test -race ./pkg/prompt/...` all pass
  - [x] CLAUDE.md files loaded in correct priority order
  - [x] Parent walking stops at filesystem root
  - [x] Empty result when no files exist

### Phase 4: Subagent Prompt Assembly
**Goal**: Implement prompt assembly for subagents (Explore, Plan, Task, WebFetch summarizer, etc.).

**Changes**:

1. **Create `pkg/prompt/subagent.go`** — subagent prompt builder:
   ```go
   func AssembleSubagentPrompt(agentDef types.AgentDefinition, parentConfig *agent.AgentConfig) string
   ```

   Subagent assembly (per spec Section 7):
   - Agent's custom `Prompt` field as base
   - Append environment details (CWD, OS, Shell)
   - Append skills content if specified
   - Do NOT include parent CLAUDE.md by default

   Built-in agent prompts (loaded from embedded files):
   - `ExplorePrompt()` → `agent-prompt-explore.md`
   - `PlanPrompt()` → `agent-prompt-plan-mode-enhanced.md`
   - `TaskPrompt()` → `agent-prompt-task-tool.md` + `agent-prompt-task-tool-extra-notes.md`
   - `WebFetchSummarizerPrompt()` → `agent-prompt-webfetch-summarizer.md`
   - `ClaudeMDCreationPrompt()` → `agent-prompt-claudemd-creation.md`
   - `AgentCreationPrompt()` → `agent-prompt-agent-creation-architect.md`
   - `StatusLineSetupPrompt()` → `agent-prompt-status-line-setup.md`
   - `RememberSkillPrompt()` → `agent-prompt-remember-skill.md`
   - `SessionSearchPrompt()` → `agent-prompt-session-search-assistant.md`
   - `SessionMemoryUpdatePrompt()` → `agent-prompt-session-memory-update-instructions.md`
   - `SessionTitlePrompt()` → `agent-prompt-session-title-and-branch-generation.md`
   - `ConversationSummarizationPrompt()` → `agent-prompt-conversation-summarization.md`
   - `SecurityReviewPrompt()` → `agent-prompt-security-review-slash-command.md`
   - `PRCommentsPrompt()` → `agent-prompt-pr-comments-slash-command.md`
   - `ReviewPRPrompt()` → `agent-prompt-review-pr-slash-command.md`
   - `UserSentimentPrompt()` → `agent-prompt-user-sentiment-analysis.md`
   - `UpdateMagicDocsPrompt()` → `agent-prompt-update-magic-docs.md`

2. **Create `pkg/prompt/subagent_test.go`** — tests:
   - Test each built-in agent prompt loads correctly
   - Test custom AgentDefinition assembly
   - Test environment details appended
   - Test CLAUDE.md NOT included by default

**Success Criteria**:
- Automated:
  - [x] `go test -race ./pkg/prompt/...` all pass
  - [x] All 17+ built-in agent prompts load without error
  - [x] Custom agent prompts assemble correctly

### Phase 5: System Reminders
**Goal**: Implement the system reminder catalog for runtime injection.

**Changes**:

1. **Create `pkg/prompt/reminders.go`** — reminder registry:
   ```go
   type ReminderID string

   const (
       ReminderFileModified       ReminderID = "file_modified"
       ReminderFileTruncated      ReminderID = "file_truncated"
       ReminderFileEmpty          ReminderID = "file_empty"
       ReminderFileShorterThanOffset ReminderID = "file_shorter_than_offset"
       ReminderOutputTokenLimit   ReminderID = "output_token_limit"
       ReminderPlanModeActive5Phase  ReminderID = "plan_mode_active_5phase"
       ReminderPlanModeActiveIterative ReminderID = "plan_mode_active_iterative"
       ReminderPlanModeActiveSubagent  ReminderID = "plan_mode_active_subagent"
       ReminderPlanModeReEntry    ReminderID = "plan_mode_re_entry"
       ReminderExitedPlanMode     ReminderID = "exited_plan_mode"
       ReminderTodoWriteReminder  ReminderID = "todowrite_reminder"
       ReminderTodoListChanged    ReminderID = "todo_list_changed"
       ReminderTodoListEmpty      ReminderID = "todo_list_empty"
       ReminderTaskToolsReminder  ReminderID = "task_tools_reminder"
       ReminderTaskStatus         ReminderID = "task_status"
       ReminderTokenUsage         ReminderID = "token_usage"
       ReminderUSDBudget          ReminderID = "usd_budget"
       ReminderMalwareAnalysis    ReminderID = "malware_analysis"
       ReminderOutputStyleActive  ReminderID = "output_style_active"
       ReminderSessionContinuation ReminderID = "session_continuation"
       ReminderVerifyPlan         ReminderID = "verify_plan"
       ReminderHookSuccess        ReminderID = "hook_success"
       ReminderHookBlockingError  ReminderID = "hook_blocking_error"
       ReminderHookStopped        ReminderID = "hook_stopped"
       ReminderHookContext        ReminderID = "hook_context"
       ReminderNewDiagnostics     ReminderID = "new_diagnostics"
       ReminderMemoryFile         ReminderID = "memory_file"
       ReminderNestedMemory       ReminderID = "nested_memory"
       ReminderCompactFileRef     ReminderID = "compact_file_ref"
       ReminderInvokedSkills      ReminderID = "invoked_skills"
       ReminderFileOpenedInIDE    ReminderID = "file_opened_in_ide"
       ReminderLinesSelectedInIDE ReminderID = "lines_selected_in_ide"
       ReminderAgentMention       ReminderID = "agent_mention"
       ReminderBtwSideQuestion    ReminderID = "btw_side_question"
       ReminderMCPResourceNoContent    ReminderID = "mcp_resource_no_content"
       ReminderMCPResourceNoDisplay    ReminderID = "mcp_resource_no_displayable"
       ReminderPlanFileRef        ReminderID = "plan_file_ref"
       // Swarm-specific (defer for now)
       // ReminderDelegateMode, ReminderExitedDelegateMode, ReminderTeamCoordination, ReminderTeamShutdown
   )

   func GetReminder(id ReminderID, vars map[string]string) string
   ```

   Each reminder is loaded from its embedded file and has simple variable substitution.

2. **Create `pkg/prompt/reminders_test.go`** — tests:
   - Test each reminder loads correctly
   - Test variable substitution in reminders
   - Test unknown reminder ID returns ""

**Success Criteria**:
- Automated:
  - [x] `go test -race ./pkg/prompt/...` all pass
  - [x] All ~37 reminders load without error
  - [x] Variable substitution works correctly

### Phase 6: Token Budget + Integration
**Goal**: Add token estimation, wire the assembler into `DefaultConfig`, and ensure backward compatibility.

**Changes**:

1. **Create `pkg/prompt/tokens.go`** — token estimation:
   ```go
   func EstimateTokens(text string) int    // len(text) / 4 heuristic
   func (a *Assembler) Validate(config *agent.AgentConfig, maxTokens int) error
   ```

2. **Update `pkg/agent/config.go`** — change `DefaultConfig()`:
   - Replace `StaticPromptAssembler` with `&prompt.Assembler{}`
   - Import `pkg/prompt`
   - Keep `StaticPromptAssembler` available for tests and simple usage

3. **Update `pkg/agent/agent.go`** — add option functions for new config fields:
   - `WithOS(os string) Option`
   - `WithShell(shell string) Option`
   - `WithClaudeMD(content string) Option`
   - `WithOutputStyle(style string) Option`
   - `WithMCPServers(servers map[string]types.McpServerConfig) Option`
   - `WithSessionsEnabled(enabled bool) Option`
   - `WithMemoryEnabled(enabled bool) Option`

4. **Create `pkg/prompt/tokens_test.go`** — tests:
   - Test estimation heuristic
   - Test validation passes for normal prompts
   - Test validation fails for oversized prompts

5. **Integration test** — `pkg/prompt/integration_test.go`:
   - Assemble full prompt with realistic config
   - Verify all expected sections present
   - Verify token estimate within expected range (~5000-8000 tokens for typical config)

**Success Criteria**:
- Automated:
  - [x] `go test -race ./...` — all tests pass (existing 199 + 52 new prompt tests = 251 total)
  - [x] `go vet ./...` clean
  - [x] `go build ./...` succeeds
  - [x] Integration test verifies full assembly
- Manual:
  - [ ] Run agentic loop with new assembler and verify system prompt sent to LLM is well-formed

## Out of Scope
- Real MCP client implementation (Spec 10) — we use the MCPClient interface/stub
- Permission system implementation (Spec 06) — interface only
- Hook system implementation (Spec 07) — interface only
- Context compaction (Spec 09) — interface only
- Swarm/teammate features (teammate communication, delegate mode, team coordination reminders)
- Chrome browser automation prompts
- Insights/analytics prompts (insights-at-a-glance, friction-analysis, etc.)
- Skillify prompt
- Session persistence/loading
- Real search provider (WebSearch)

## Open Questions
None — all resolved during planning.

## References
- Spec: `thoughts/specs/05-SYSTEM-PROMPT.md`
- Piebald-AI source: `/tmp/piebald-prompts/` (cloned from https://github.com/Piebald-AI/claude-code-system-prompts v2.1.37)
- Existing interface: `pkg/agent/interfaces.go:11-13`
- Existing config: `pkg/agent/config.go`
- Existing stubs: `pkg/agent/stubs.go`
- Tool registry: `pkg/tools/registry.go`
