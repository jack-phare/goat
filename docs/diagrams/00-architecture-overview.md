# Goat Architecture Overview

> Go port of the Claude Code agentic loop — a faithful reimplementation of the TypeScript/Node.js
> CLI agent as an idiomatic Go library (`github.com/jg-phare/goat`).

## System Context: How Goat Relates to Claude Code

```
                       ┌─────────────────────────────────────┐
                       │         Claude Code (TS/Node)        │
                       │  - Anthropic's official CLI agent    │
                       │  - ~50k LOC TypeScript               │
                       │  - Terminal UI + Ink React renderer  │
                       │  - Direct Anthropic API calls        │
                       └──────────────┬──────────────────────┘
                                      │  "port of"
                                      │  (same architecture,
                                      │   same prompt files,
                                      │   same SDK message types)
                                      ▼
 ┌──────────────────────────────────────────────────────────────────────┐
 │                          Goat (Go Library)                          │
 │                                                                     │
 │  Module: github.com/jg-phare/goat                                  │
 │  Go 1.25 • ~800+ tests • 12 packages                              │
 │                                                                     │
 │  Key differences from Claude Code TS:                               │
 │  ┌───────────────────────────────────────────────────────────────┐  │
 │  │ 1. Library, not CLI — no terminal UI, embeddable             │  │
 │  │ 2. LiteLLM proxy — routes via OpenAI-compatible endpoint     │  │
 │  │ 3. Channel-based streaming — Go channels, not async iterators│  │
 │  │ 4. Compile-time safety — interfaces, not duck-typed objects  │  │
 │  │ 5. No external deps for tools — pure Go (except doublestar) │  │
 │  │ 6. Transport-agnostic — pluggable stdio/ws/sse/channel       │  │
 │  └───────────────────────────────────────────────────────────────┘  │
 └──────────────────────────────────────────────────────────────────────┘
```

## Package Dependency Graph

This is the actual import graph. Arrows point from importer to importee.
The `agent` package sits at the center — it defines interfaces, and all other
packages import it (dependency inversion).

```
                          ┌──────────────┐
                          │  pkg/types/  │  SDK message types, enums
                          │  (leaf node) │  options, content blocks
                          └──────┬───────┘
                                 │ imported by everything
                 ┌───────────────┼───────────────────────────┐
                 │               │                           │
          ┌──────▼──────┐ ┌─────▼──────┐             ┌──────▼──────┐
          │  pkg/llm/   │ │ pkg/tools/ │             │ pkg/agent/  │
          │  LLM client │ │  Registry  │             │  Loop core  │
          │  SSE parser │ │  21 tools  │             │  Interfaces │
          └──────┬──────┘ └─────┬──────┘             └──────┬──────┘
                 │              │                            │
                 │    ┌─────────┘   imported by all below    │
                 │    │         ┌────────────────────────────┘
                 │    │         │    (dependency inversion:
                 │    │         │     packages implement
                 │    │         │     agent.* interfaces)
         ┌───────┼────┼─────┬──┼──────────┬──────────────┬──────────┐
         │       │    │     │  │          │              │          │
   ┌─────▼─┐ ┌──▼────▼┐ ┌─▼──▼───┐ ┌────▼────┐  ┌─────▼───┐ ┌───▼────────┐
   │prompt/│ │context/│ │hooks/  │ │permiss/ │  │session/ │ │ subagent/ │
   │Prompt │ │Compact │ │Hook    │ │Permiss  │  │Persist  │ │  Manager  │
   │Assemb │ │  or    │ │Runner  │ │Checker  │  │  Store  │ │  Spawner  │
   └───────┘ └────────┘ └────────┘ └─────────┘  └─────────┘ └─────┬─────┘
                                                                   │
                                                        ┌──────────┴──────┐
                                                        │                 │
                                                  ┌─────▼──┐     ┌───────▼───┐
                                                  │ teams/ │     │transport/ │
                                                  │ Agent  │     │ Stdio/WS  │
                                                  │ Teams  │     │ SSE/Chan  │
                                                  └────────┘     └───────────┘
                                                        │
                                                  ┌─────▼──┐
                                                  │  mcp/  │
                                                  │  MCP   │
                                                  │ Client │
                                                  └────────┘
```

## How Goat Maps to Claude Code Components

```
┌─────────────────────────┬───────────────────────┬─────────────────────────────┐
│ Claude Code (TS)        │ Goat (Go)             │ Notes                       │
├─────────────────────────┼───────────────────────┼─────────────────────────────┤
│ AgentLoop class         │ pkg/agent/ RunLoop()  │ State machine, channels     │
│ Anthropic SDK client    │ pkg/llm/ Client       │ OpenAI-compat via LiteLLM   │
│ Tool definitions        │ pkg/tools/ Registry   │ 21 static + mcp__* dynamic  │
│ Permission system       │ pkg/permission/       │ 8-layer checker, 6 modes    │
│ Hook system             │ pkg/hooks/ Runner     │ Go callbacks + shell cmds   │
│ System prompt assembly  │ pkg/prompt/ Assembler │ 133 embedded prompt files   │
│ Context compaction      │ pkg/context/ Compactor│ LLM-powered summarization   │
│ Session persistence     │ pkg/session/ Store    │ JSONL + async writer        │
│ Subagent manager        │ pkg/subagent/ Manager │ 6 built-in agent types      │
│ Agent teams             │ pkg/teams/ Manager    │ File-based coordination     │
│ MCP client              │ pkg/mcp/ Client       │ JSON-RPC 2.0, stdio+HTTP   │
│ SDK message types       │ pkg/types/            │ 1:1 correspondence          │
│ Transport (terminal UI) │ pkg/transport/        │ 4 pluggable transports      │
└─────────────────────────┴───────────────────────┴─────────────────────────────┘
```

## Data Flow Overview

```
 User Input (string)
      │
      ▼
 ┌─────────────┐     ┌─────────────────┐
 │  Transport   │────▶│   agent.Query   │
 │ (stdio/ws/   │     │  (channels:     │
 │  sse/chan)   │     │   input, ctrl,  │
 └─────────────┘     │   messages,     │
      ▲               │   done, close) │
      │               └────────┬────────┘
      │                        │
      │               ┌────────▼────────┐
      │               │   RunLoop()     │
      │               │  goroutine      │
      │               │                 │
      │               │  ┌───────────┐  │      ┌──────────────┐
      │               │  │ Prompt    │──┼─────▶│ LLM Client   │
      │               │  │ Assembly  │  │      │ (SSE stream  │
      │               │  └───────────┘  │      │  via LiteLLM)│
      │               │                 │      └──────┬───────┘
      │               │  ┌───────────┐  │             │
      │               │  │ Permission│  │      ┌──────▼───────┐
      │               │  │  Checker  │  │      │ Accumulate() │
      │               │  └───────────┘  │      │ (stream→resp)│
      │               │                 │      └──────┬───────┘
      │               │  ┌───────────┐  │             │
      │               │  │  Hooks    │  │      ┌──────▼───────┐
      │               │  │  Runner   │  │      │ Tool Exec    │
      │               │  └───────────┘  │      │ (serial,     │
      │               │                 │      │  permission  │
      │               │  ┌───────────┐  │      │  checked)    │
      │               │  │ Compactor │  │      └──────┬───────┘
      │               │  └───────────┘  │             │
      │               │                 │      ┌──────▼───────┐
      │               │  ┌───────────┐  │      │ Append to    │
      │               │  │ Session   │  │      │ history,     │
      │               │  │  Store    │  │      │ loop back    │
      │               │  └───────────┘  │      └──────────────┘
      │               └────────┬────────┘
      │                        │
      │               SDKMessage channel
      │               (64-buffer, typed
      │                messages)
      │                        │
      └────────────────────────┘
         (transported back to consumer)
```

## Go vs TypeScript: Architectural Tradeoffs

### Where Go Wins

| Aspect | Go Advantage | Details |
|--------|-------------|---------|
| **Concurrency** | Goroutines + channels | Loop runs in goroutine, messages flow via buffered channels. No callback hell. |
| **Type safety** | Interfaces at compile time | `PermissionChecker`, `HookRunner`, `ContextCompactor` — enforced at compile time, not runtime duck typing. |
| **Embedding** | Library, not CLI | Import as `pkg/agent` — no subprocess, no IPC overhead. |
| **Testing** | Race detector | All 800+ tests pass with `-race`. TypeScript has no equivalent. |
| **Resource control** | Context cancellation | `context.Context` propagates cancellation cleanly through entire call chain. |
| **Binary distribution** | Single static binary | No Node.js runtime, no `node_modules`. |

### Where TypeScript Has Advantages

| Aspect | TS Advantage | Goat Mitigation |
|--------|-------------|-----------------|
| **JSON handling** | Native JSON types | `map[string]any` + `encoding/json` (more verbose but type-safe) |
| **Async/await** | Ergonomic async | Channels + goroutines (different paradigm, equally powerful) |
| **Dynamic tools** | Prototype-based | Interface-based registration (more code, but explicit) |
| **Terminal UI** | Ink React renderer | Transport layer abstraction (UI delegated to consumer) |
| **Prompt files** | Dynamic `require()` | `//go:embed` (compiled in, no runtime FS access needed) |
| **Hot reload** | `ts-node` / `tsx` | Recompile required (fast with Go, but not instant) |

### Key Design Decisions

```
┌─────────────────────────────────────────────────────────────────────┐
│                    Dependency Inversion Pattern                      │
│                                                                     │
│  pkg/agent/ defines INTERFACES:                                     │
│    - PermissionChecker                                              │
│    - HookRunner                                                     │
│    - ContextCompactor                                               │
│    - SystemPromptAssembler                                          │
│    - SessionStore                                                   │
│                                                                     │
│  Other packages IMPLEMENT them:                                     │
│    - pkg/permission/ → PermissionChecker                            │
│    - pkg/hooks/      → HookRunner                                   │
│    - pkg/context/    → ContextCompactor                             │
│    - pkg/prompt/     → SystemPromptAssembler                        │
│    - pkg/session/    → SessionStore                                 │
│                                                                     │
│  This avoids import cycles and enables:                             │
│    - Unit testing with stubs (AllowAllChecker, NoOpHookRunner)      │
│    - Swapping implementations without touching the loop             │
│    - Embedding with only the pieces you need                        │
│                                                                     │
│  Claude Code TS uses dependency injection too, but Go interfaces    │
│  make the contracts explicit at compile time.                       │
└─────────────────────────────────────────────────────────────────────┘
```
