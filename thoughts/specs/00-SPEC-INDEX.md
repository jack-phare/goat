# Claude Code Go Port: Component Specification Index

**Version**: 2.1.37 (Claude Code CLI, Feb 7 2026)
**SDK Version**: @anthropic-ai/claude-agent-sdk 0.2.37 / claude-agent-sdk-python 0.1.21
**Source Material**: TS SDK `sdk.d.ts`, `sdk-tools.d.ts`, `sdk.mjs`; Piebald-AI system prompts; Anthropic Messages API streaming docs; LiteLLM proxy docs

---

## Specification Documents

| # | Component | Go Package | Spec File | Source References |
|---|-----------|------------|-----------|-------------------|
| 1 | **LLM Client & Streaming** | `pkg/llm/` | [01-LLM-CLIENT.md](./01-LLM-CLIENT.md) | Anthropic Messages API, LiteLLM `/chat/completions`, SSE event types |
| 2 | **Message Types & Protocol** | `pkg/types/` | [02-MESSAGE-TYPES.md](./02-MESSAGE-TYPES.md) | `sdk.d.ts:1140-1578` (SDKMessage union), `sdk-tools.d.ts` |
| 3 | **Agentic Loop** | `pkg/agent/` | [03-AGENTIC-LOOP.md](./03-AGENTIC-LOOP.md) | `sdk.d.ts:948-1083` (Query interface), `sdk.mjs` (minified loop) |
| 4 | **Tool Registry & Execution** | `pkg/tools/` | [04-TOOL-REGISTRY.md](./04-TOOL-REGISTRY.md) | `sdk-tools.d.ts:1-1570` (all tool input schemas), system prompts |
| 5 | **System Prompt Assembly** | `pkg/prompt/` | [05-SYSTEM-PROMPT.md](./05-SYSTEM-PROMPT.md) | Piebald-AI repo (60+ prompt files), `sdk.d.ts:776-800` |
| 6 | **Permission System** | `pkg/permission/` | [06-PERMISSION-SYSTEM.md](./06-PERMISSION-SYSTEM.md) | `sdk.d.ts:824-897` (PermissionMode, PermissionResult, PermissionUpdate) |
| 7 | **Hook System** | `pkg/hooks/` | [07-HOOK-SYSTEM.md](./07-HOOK-SYSTEM.md) | `sdk.d.ts:254-277` (HookEvent, HookCallback), Python CHANGELOG |
| 8 | **Subagent & Task Manager** | `pkg/subagent/` | [08-SUBAGENT-MANAGER.md](./08-SUBAGENT-MANAGER.md) | `sdk.d.ts:33-67` (AgentDefinition), `sdk-tools.d.ts:32-73` (AgentInput) |
| 9 | **Context & Compaction** | `pkg/context/` | [09-CONTEXT-COMPACTION.md](./09-CONTEXT-COMPACTION.md) | `sdk.d.ts:1162-1171` (SDKCompactBoundaryMessage), system prompts |
| 10 | **MCP Integration** | `pkg/mcp/` | [10-MCP-INTEGRATION.md](./10-MCP-INTEGRATION.md) | `sdk.d.ts:290-398` (McpServerConfig variants), control requests |
| 11 | **Session & Checkpoint Store** | `pkg/session/` | [11-SESSION-CHECKPOINT.md](./11-SESSION-CHECKPOINT.md) | `sdk.d.ts:1086-1094` (RewindFilesResult), `sdk.d.ts:1444-1459` (SDKSession) |
| 12 | **Transport Layer** | `pkg/transport/` | [12-TRANSPORT.md](./12-TRANSPORT.md) | `sdk.d.ts:1771-1795` (Transport interface), `sdk.d.ts:1656-1710` (SpawnedProcess) |

---

## Architecture Decision: Why These Specs Exist

Claude Code's core agent runtime is a **closed-source binary** (~220MB compiled). The official SDKs (TS/Python) are thin wrappers that spawn this binary as a subprocess and communicate via stdin/stdout JSON-line protocol.

**What we CAN specify from public sources:**
- Complete SDK type contracts (`sdk.d.ts` — the canonical API surface)
- All 18 built-in tool input/output schemas (`sdk-tools.d.ts`)
- System prompt catalog (~60 files from Piebald-AI extraction)
- Messages API streaming wire format (SSE events)
- Control protocol between SDK↔CLI (control_request/control_response)
- Hook event lifecycle and callback signatures

**What we MUST reimplement in Go (not wrap):**
- The agentic tool-use loop (currently inside the binary)
- Direct LLM API calls via LiteLLM (replacing the CLI subprocess)
- Tool execution (Bash, FileRead, FileWrite, Grep, Glob, etc.)
- Context window management and compaction
- Session persistence and file checkpointing

**Verification Strategy:**
Each spec includes a "Verification Checklist" section defining:
1. Input/output contracts from the SDK types
2. Wire-format assertions for LiteLLM compatibility
3. Behavioral equivalence tests against the SDK
