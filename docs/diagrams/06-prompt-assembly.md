# Prompt Assembly — System Prompt Construction

> `pkg/prompt/` — Assembles the system prompt from 133 embedded prompt files,
> conditional sections, CLAUDE.md files, and variable interpolation.

## How System Prompt Assembly Works

```
 ┌─────────────────────────────────────────────────────────────────────┐
 │                    Assembler.Assemble(config)                       │
 │                                                                     │
 │  Input: AgentConfig with all context fields                        │
 │  Output: Single string — the complete system prompt                │
 │                                                                     │
 │  ┌─── Step 1: Load base system prompt ──────────────────────────┐  │
 │  │  //go:embed prompts/system/system-prompt.md                   │  │
 │  │  (the core "You are Claude Code" prompt)                      │  │
 │  └───────────────────────────────────────────────────────────────┘  │
 │                                                                     │
 │  ┌─── Step 2: Conditional sections ─────────────────────────────┐  │
 │  │                                                               │  │
 │  │  if config.ToolRegistry has tools:                            │  │
 │  │    append tool usage instructions                             │  │
 │  │                                                               │  │
 │  │  if config.SessionsEnabled:                                   │  │
 │  │    append session management instructions                     │  │
 │  │                                                               │  │
 │  │  if config.MemoryEnabled:                                     │  │
 │  │    append memory instructions                                 │  │
 │  │                                                               │  │
 │  │  if config.GitBranch != "":                                   │  │
 │  │    append git context (branch, status, commits)               │  │
 │  │                                                               │  │
 │  │  if config.MCPServers != nil:                                 │  │
 │  │    append MCP server instructions                             │  │
 │  │                                                               │  │
 │  │  if config.ClaudeMDContent != "":                             │  │
 │  │    append CLAUDE.md content                                   │  │
 │  │                                                               │  │
 │  │  if config.OutputStyle != "":                                 │  │
 │  │    append output style instructions                           │  │
 │  └───────────────────────────────────────────────────────────────┘  │
 │                                                                     │
 │  ┌─── Step 3: Variable interpolation ───────────────────────────┐  │
 │  │  Replace ${VAR} patterns from original JS prompt files:       │  │
 │  │    ${CWD}          → config.CWD                               │  │
 │  │    ${OS}           → config.OS (runtime.GOOS)                │  │
 │  │    ${SHELL}        → config.Shell                             │  │
 │  │    ${MODEL}        → config.Model                             │  │
 │  │    ${GIT_BRANCH}   → config.GitBranch                        │  │
 │  │    ${DATE}         → current date                             │  │
 │  │    ${VERSION}      → config.PromptVersion                    │  │
 │  └───────────────────────────────────────────────────────────────┘  │
 │                                                                     │
 │  ┌─── Step 4: Token estimation ─────────────────────────────────┐  │
 │  │  estimate = len(prompt) / 4                                   │  │
 │  │  (heuristic: ~4 chars per token)                              │  │
 │  └───────────────────────────────────────────────────────────────┘  │
 └─────────────────────────────────────────────────────────────────────┘
```

## Embedded Prompt File Organization

```
 pkg/prompt/prompts/
 ├── agents/                       // 17 subagent prompt files
 │   ├── agent-bash.md
 │   ├── agent-claude-code-guide.md
 │   ├── agent-explore.md
 │   ├── agent-general-purpose.md
 │   ├── agent-plan.md
 │   ├── agent-statusline-setup.md
 │   └── ... (12 more specialty agents)
 │
 ├── system/                       // Core system prompt sections
 │   ├── system-prompt.md          // Main "You are Claude Code" prompt
 │   ├── system-prompt-doing-tasks.md
 │   ├── system-prompt-executing-actions.md
 │   ├── system-prompt-using-tools.md
 │   ├── system-prompt-tone-style.md
 │   ├── system-prompt-auto-memory.md
 │   ├── system-prompt-git-commit.md
 │   ├── system-prompt-git-pr.md
 │   ├── system-prompt-file-read-limits.md
 │   ├── system-prompt-large-file-handling.md
 │   ├── system-prompt-use-tools-to-verify.md
 │   └── ... (more sections)
 │
 ├── reminders/                    // 37 system reminder templates
 │   ├── reminder-auto-compact.md
 │   ├── reminder-permission-denied.md
 │   ├── reminder-tool-error.md
 │   └── ...
 │
 ├── tools/                        // Per-tool usage instructions
 │   ├── tool-bash.md
 │   ├── tool-read.md
 │   ├── tool-write.md
 │   └── ...
 │
 ├── data/                         // Reference data
 │   └── pricing.json
 │
 └── skills/                       // Skill-specific prompts
     └── ...

 Total: 133 files, compiled in via //go:embed
 Version: v2.1.37 (Piebald-AI prompt set)
```

## CLAUDE.md Loading — Directory Hierarchy Walk

```
 Given CWD = /Users/dev/myproject/src/pkg/

 Walk UP the directory tree:
 ┌──────────────────────────────────────────────────────────────────┐
 │                                                                  │
 │  /Users/dev/myproject/src/pkg/CLAUDE.md      ← checked first   │
 │  /Users/dev/myproject/src/pkg/.claude/CLAUDE.md                 │
 │  /Users/dev/myproject/src/CLAUDE.md                             │
 │  /Users/dev/myproject/src/.claude/CLAUDE.md                     │
 │  /Users/dev/myproject/CLAUDE.md              ← project root    │
 │  /Users/dev/myproject/.claude/CLAUDE.md                         │
 │  /Users/dev/CLAUDE.md                                           │
 │  /Users/dev/.claude/CLAUDE.md                                   │
 │  /Users/CLAUDE.md                                               │
 │  ...up to filesystem root                                       │
 │                                                                  │
 │  All found files are concatenated (child dirs first)            │
 │  Result → config.ClaudeMDContent                                │
 └──────────────────────────────────────────────────────────────────┘
```

## Subagent Prompt Accessors

17 built-in agent prompts accessible via typed functions:

```
 ┌──────────────────────────────────────────────────────────────────┐
 │  Agent Name             │ Model   │ Prompt File                  │
 ├─────────────────────────┼─────────┼──────────────────────────────┤
 │  general-purpose        │ default │ agent-general-purpose.md     │
 │  Explore                │ haiku   │ agent-explore.md             │
 │  Plan                   │ default │ agent-plan.md                │
 │  Bash                   │ default │ agent-bash.md                │
 │  statusline-setup       │ sonnet  │ agent-statusline-setup.md    │
 │  claude-code-guide      │ haiku   │ agent-claude-code-guide.md   │
 │  code-simplifier        │ default │ agent-code-simplifier.md     │
 │  codebase-analyzer      │ default │ agent-codebase-analyzer.md   │
 │  codebase-locator       │ default │ agent-codebase-locator.md    │
 │  codebase-pattern-finder│ default │ agent-codebase-pattern.md    │
 │  thoughts-analyzer      │ default │ agent-thoughts-analyzer.md   │
 │  thoughts-locator       │ default │ agent-thoughts-locator.md    │
 │  web-search-researcher  │ default │ agent-web-search.md          │
 │  python-simplifier      │ default │ agent-python-simplifier.md   │
 │  ... (3 more)           │         │                              │
 └──────────────────────────┴─────────┴──────────────────────────────┘
```

## System Reminders — Dynamic Context Injection

37 reminder IDs with variable substitution, injected as needed:

```
 Reminder examples:
   "auto-compact"     → "Context was automatically compacted..."
   "permission-denied" → "Permission denied for ${TOOL_NAME}..."
   "tool-error"       → "Tool ${TOOL_NAME} returned error..."

 Used by the agent loop to add context-sensitive notes
 to the conversation without modifying the base system prompt.
```

## Go vs TS: Prompt Assembly

```
 ┌────────────────────────────┬──────────────────────────────────────┐
 │ Claude Code TS              │ Goat Go                              │
 ├────────────────────────────┼──────────────────────────────────────┤
 │ require() at runtime        │ //go:embed at compile time          │
 │ Dynamic file reads          │ Embedded in binary (no FS needed)   │
 │ Template literals           │ strings.ReplaceAll for ${VAR}       │
 │ Class-based assembler       │ Interface-based (Assemble(config))  │
 │ Prompt version from package │ Explicit config.PromptVersion       │
 │                              │                                      │
 │ Both use the SAME prompt    │ 133 files copied from Claude Code   │
 │ files (v2.1.37)             │ v2.1.37 source                      │
 └────────────────────────────┴──────────────────────────────────────┘

 Go advantage: Prompts compiled into binary. No filesystem dependency,
 no accidental modification, instant access. Single binary deployment.

 Tradeoff: Updating prompts requires recompilation. Claude Code TS
 can hot-reload prompts by restarting the Node.js process.
```

## Import Direction (Cycle Avoidance)

```
 prompt imports agent (for AgentConfig type)
 agent does NOT import prompt

 Caller wires them together:
   assembler := prompt.NewAssembler()
   config := agent.New(llmClient, registry,
     agent.WithPrompter(assembler),  // inject assembler
   )

 This avoids import cycles while keeping the interface in agent/
 and the implementation in prompt/.
```
