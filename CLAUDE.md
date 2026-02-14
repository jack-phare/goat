# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Is

Goat is a Go port of the Claude Code agentic loop. It implements the full agent lifecycle — LLM streaming, tool execution, permission checking, context compaction, session persistence, subagent spawning, team coordination, MCP integration, and prompt assembly — as a Go library with an OpenAI-compatible LLM backend (routed through LiteLLM proxy).

## Build & Test Commands

```bash
# Build all packages
go build ./...

# Run all tests (always use -race)
go test -race ./...

# Run tests for a single package
go test -race ./pkg/llm/...

# Run a single test by name
go test -race ./pkg/agent/... -run TestLoopEndTurn

# Update golden test files (when expected output changes)
go test ./pkg/llm/... -run TestGolden -update

# Vet all packages
go vet ./...

# Build the eval binary (headless benchmark runner)
go build -o example ./cmd/eval/

# Cross-compile eval binary for Linux (Modal sandbox)
bash scripts/build_eval.sh amd64
```

## Architecture

### Core Loop (`pkg/agent/`)

The agentic loop is the central orchestrator. `RunLoop()` launches a goroutine that runs a state machine:

```
INITIALIZE → LLM_CALL → PARSE_RESP → (end_turn | tool_use | max_tokens) → LLM_CALL → ...
```

It emits `types.SDKMessage` values on a channel-based `Query` type. The caller consumes messages via `query.Messages()` and waits via `query.Wait()`.

All dependencies are injected through `AgentConfig` — the loop itself never imports concrete implementations of permissions, hooks, prompts, compaction, etc. This is the central design principle: `pkg/agent/` defines interfaces, other packages implement them.

### Key Interfaces (all in `pkg/agent/interfaces.go`)

| Interface | Implementor | Purpose |
|-----------|-------------|---------|
| `SystemPromptAssembler` | `pkg/prompt.Assembler` | Builds system prompt from 133 embedded Piebald files |
| `PermissionChecker` | `pkg/permission.Checker` | Layered permission flow (mode → rules → hooks → prompter) |
| `HookRunner` | `pkg/hooks.Runner` | Pre/post tool lifecycle hooks |
| `ContextCompactor` | `pkg/context.Compactor` | LLM-powered summarization with truncation fallback |
| `SessionStore` | `pkg/session.Store` | JSONL file-based async persistence |
| `SkillProvider` | `pkg/prompt.SkillRegistry` | Skill lookup and slash command resolution |

Stubs exist for all interfaces: `AllowAllChecker`, `NoOpHookRunner`, `NoOpCompactor`, `NoOpSessionStore`.

### Import Graph (critical — no cycles allowed)

```
agent (interfaces only, no concrete deps)
  ↑
  ├── prompt      (imports agent for AgentConfig)
  ├── permission  (imports agent for PermissionChecker + HookRunner)
  ├── hooks       (imports agent for HookRunner)
  ├── context     (imports agent, llm)
  ├── session     (imports agent, llm)
  ├── subagent    (imports agent, hooks, prompt, tools, llm)
  ├── teams       (imports agent, hooks)
  └── transport   (imports agent)

tools (standalone, no agent import)
  ↑
  └── mcp (imports tools only)

llm (standalone)
types (standalone)
```

The `pkg/agent/` package defines types like `TokenBudget` and `CompactRequest` that would naturally belong in `pkg/context/`, but live in `agent` to avoid import cycles.

### LLM Client (`pkg/llm/`)

OpenAI-compatible streaming client. Speaks SSE to `/v1/chat/completions`. Key types:
- `Client` interface with `Complete()` returning a `*Stream`
- `Stream` wraps `bufio.Scanner` over SSE; `Next()`/`Chunk()` iteration
- `ToolCallAccumulator` merges sparse/interleaved tool call deltas
- `CostTracker` (mutex-protected) tracks per-model USD spend
- Model IDs get `anthropic/` prefix added for requests, stripped from responses

### Tool System (`pkg/tools/`)

`Registry` maps tool names to `Tool` interface implementations. 22 static tools + dynamic `mcp__*` tools. Tools are serial-executed in the loop; permissions checked before each invocation.

`TaskManager` (shared between Bash/TaskOutput/TaskStop) manages background processes with state tracking (Running → Completed/Failed/Stopped).

### Prompt Assembly (`pkg/prompt/`)

133 Piebald-AI v2.1.37 prompt files embedded via `//go:embed`. The `Assembler` conditionally assembles sections based on `AgentConfig` (tools, sessions, memory, git, MCP, skills). Handles JS-style `${VAR}` interpolation from original prompt files.

Skill system: `SkillLoader` scans `.claude/skills/{name}/SKILL.md` files (YAML frontmatter + markdown body). `SkillRegistry` is thread-safe. `SkillWatcher` provides fsnotify-based hot-reload with 500ms debounce.

### Subagent Manager (`pkg/subagent/`)

12-step spawn flow: limits → resolve definition → generate ID → hooks → model → tools → permissions → memory → prompt → config → registry → launch. 6 built-in agent types (general-purpose, Explore, Plan, Bash, statusline-setup, claude-code-guide). No nesting — Agent tool excluded from subagent registries.

### Entry Points

- `cmd/eval/main.go` — Headless eval binary for benchmarks. Env vars: `OPENAI_BASE_URL`, `OPENAI_API_KEY`, `EVAL_MODEL`. Flags: `-prompt`, `-cwd`, `-max-turns`, `-skills-dir`.
- `cmd/example/main.go` — Interactive example with multi-provider support (Groq, OpenAI, Anthropic, LiteLLM).

## Test Conventions

- All tests pass with `-race` flag (~800+ tests)
- Golden tests in `testdata/golden/` — regenerate with `-update` flag
- SSE test fixtures in `testdata/sse/`
- `httptest` for HTTP mocking — no external network calls in tests
- Agent loop tests use `mockLLMClient` with pre-programmed `StreamChunk` responses
- Table-driven tests are the standard pattern

## Project Conventions

- Specs live in `thoughts/specs/` (now archived in `thoughts/archive/`), plans in `thoughts/plans/`
- Go module: `github.com/jg-phare/goat`, Go 1.24.1
- Use `uv run python` for any Python tasks
- CI runs `go build -v ./...` then `go test -v ./...` on push/PR to main
