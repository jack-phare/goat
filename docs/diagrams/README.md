# Goat Engineering Diagrams

Comprehensive architecture and component diagrams for the Goat project —
a Go port of Claude Code's agentic loop.

## Diagram Index

| # | File | Component | Description |
|---|------|-----------|-------------|
| 00 | [architecture-overview](00-architecture-overview.md) | **Full System** | Package dependency graph, Go vs TS tradeoffs, data flow overview |
| 01 | [agent-loop](01-agent-loop.md) | `pkg/agent/` | State machine, iteration lifecycle, exit reasons, Query API |
| 02 | [llm-client](02-llm-client.md) | `pkg/llm/` | SSE streaming, accumulation, ToolCallAccumulator, retry, cost tracking |
| 03 | [tool-system](03-tool-system.md) | `pkg/tools/` | Registry, 21 static tools, execution flow, TaskManager, MCP tools |
| 04 | [permission-system](04-permission-system.md) | `pkg/permission/` | 8-layer checker waterfall, 6 modes, risk matrix, rule matching |
| 05 | [hook-system](05-hook-system.md) | `pkg/hooks/` | 15 events, Go callbacks + shell, scoped hooks, context injection |
| 06 | [prompt-assembly](06-prompt-assembly.md) | `pkg/prompt/` | 133 embedded files, conditional sections, CLAUDE.md loading |
| 07 | [context-compaction](07-context-compaction.md) | `pkg/context/` | Proactive/reactive compaction, split point calculation, fallback |
| 08 | [subagent-manager](08-subagent-manager.md) | `pkg/subagent/` | 12-step spawn flow, 6 built-in agents, definition loading |
| 09 | [session-persistence](09-session-persistence.md) | `pkg/session/` | JSONL storage, async writer, checkpointing, session restore |
| 10 | [mcp-client](10-mcp-client.md) | `pkg/mcp/` | JSON-RPC 2.0, stdio+HTTP transports, dynamic tool registration |
| 11 | [agent-teams](11-agent-teams.md) | `pkg/teams/` | Multi-agent coordination, shared tasks, mailbox messaging |
| 12 | [transport-layer](12-transport-layer.md) | `pkg/transport/` | 4 transports (Channel/Stdio/WS/SSE), Router, ProcessAdapter |
| 13 | [sdk-types](13-sdk-types.md) | `pkg/types/` | Message hierarchy, content blocks, enums, type system |
| 14 | [end-to-end-request](14-end-to-end-request.md) | **Cross-cutting** | Complete trace of a single request through all components |
| 15 | [provider-setup](15-provider-setup.md) | `cmd/example/` | Provider resolution, API key lookup, LiteLLM routing, env setup |
| 16 | [skill-evaluation](16-skill-evaluation.md) | `cmd/eval/` + `pkg/prompt/` + `pkg/tools/` | Skill loading, MCP wiring, multi-turn REPL, A/B + 3-way benchmark |

## Reading Order

**For understanding the system top-down:**
1. Start with `00-architecture-overview` for the big picture
2. Read `14-end-to-end-request` to see how a real request flows
3. Dive into `01-agent-loop` for the core state machine
4. Then explore individual components as needed

**For understanding a specific component:**
- Jump directly to the relevant numbered file
- Each diagram is self-contained with full context

## Conventions

- All diagrams use ASCII art (no external rendering tools needed)
- File:line references point to actual source locations
- "Go vs TS" comparisons show how Goat differs from Claude Code
- Each diagram includes the component's relationship to other components

## Stats

- **12 packages** documented
- **800+ tests** referenced
- **133 prompt files** embedded
- **22 static tools** + dynamic MCP tools + **Skill tool**
- **15 hook events**, **6 permission modes**, **4 transport types**
- **3 benchmark skills**, **15 skill-specific eval tasks**
- **MCP config** + **multi-turn REPL** in eval binary
- **3-way A/B** benchmarking (baseline vs +skills vs +skills+mcp)
