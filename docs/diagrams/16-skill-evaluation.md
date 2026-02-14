# 16 — Skill & MCP Evaluation Framework

How skills and MCP servers are loaded, injected, and invoked during eval benchmarks,
how multi-turn REPL mode works, and how the A/B/3-way comparison mode works.

## Skill System Overview

Goat's skill system mirrors Claude Code's three-tier progressive disclosure:

```
Level 1: Metadata in system prompt (~100 tokens/skill)
         "- go-expert: Go coding expert... When writing Go code"

Level 2: Full body returned on invocation (~2-6k tokens)
         LLM calls Skill tool → gets full SKILL.md body

Level 3: Bundled files/context (not yet implemented)
```

## Eval Binary Skill Pipeline

The eval binary (`cmd/eval/main.go`) wires skills when `-skills-dir` is provided:

```
cmd/eval/main.go
    │
    │  -skills-dir ./eval/skills
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│  1. LOAD                                                │
│                                                         │
│  prompt.NewSkillLoader(skillsDir, "")                   │
│      └─ scans {skillsDir}/.claude/skills/{name}/SKILL.md│
│      └─ parses YAML frontmatter + markdown body         │
│      └─ returns map[string]types.SkillEntry             │
│                                                         │
│  Files:                                                 │
│    pkg/prompt/skill_loader.go    — discovery             │
│    pkg/prompt/skill_frontmatter.go — YAML parsing        │
└──────────────────────┬──────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────┐
│  2. REGISTER                                            │
│                                                         │
│  prompt.NewSkillRegistry()                              │
│      └─ thread-safe map[string]SkillEntry               │
│      └─ FormatSkillsList() → bullet list for prompt     │
│      └─ satisfies agent.SkillProvider interface          │
│                                                         │
│  File: pkg/prompt/skill_registry.go                      │
└──────────────────────┬──────────────────────────────────┘
                       │
              ┌────────┴────────┐
              ▼                 ▼
┌──────────────────┐  ┌──────────────────────────────────┐
│  3a. SYSTEM      │  │  3b. TOOL                        │
│     PROMPT       │  │                                  │
│                  │  │  tools.SkillProviderAdapter       │
│  config.Skills = │  │      └─ bridges SkillRegistry    │
│    registry      │  │         to tools.SkillProvider   │
│                  │  │                                  │
│  Assembler sees  │  │  tools.SkillTool{                │
│  Skills != nil,  │  │    Skills:  adapter,             │
│  injects:        │  │    ArgSub:  prompt.SubstituteArgs│
│                  │  │  }                               │
│  "Available      │  │      └─ registered in Registry   │
│   skills:        │  │                                  │
│   - go-expert    │  │  File: pkg/tools/skilltool.go    │
│   - testing-..." │  │  File: pkg/tools/skill_adapter.go│
│                  │  │                                  │
│  File:           │  │                                  │
│  pkg/prompt/     │  │                                  │
│  assembler.go    │  │                                  │
│  :112-121        │  │                                  │
└──────────────────┘  └──────────────────────────────────┘
```

## Skill Invocation Flow

When the LLM sees skills listed in the system prompt and a relevant
user request, it emits a Skill tool call:

```
User: "Write a Go function with proper error handling"
                │
                ▼
┌───────────────────────────────────────┐
│  LLM sees system prompt:             │
│    "Available skills:                │
│     - go-expert: Go coding expert..."│
│                                      │
│  LLM emits:                          │
│    tool_use: Skill                   │
│    input: {skill: "go-expert"}       │
└───────────────┬───────────────────────┘
                │
                ▼
┌───────────────────────────────────────┐
│  SkillTool.Execute()                 │
│    1. adapter.GetSkillInfo("go-expert")
│    2. registry.GetSkill("go-expert") │
│    3. return ToolOutput{Content: body}│
│                                      │
│  Returns 3,797 chars of Go idioms,   │
│  error handling patterns, stdlib...  │
└───────────────┬───────────────────────┘
                │
                ▼
┌───────────────────────────────────────┐
│  LLM sees tool_result with full      │
│  skill body. Generates response      │
│  informed by Go expert knowledge.    │
└───────────────────────────────────────┘
```

## MCP Server Wiring

The eval binary (`cmd/eval/main.go`) wires MCP servers when `-mcp-config` is provided:

```
cmd/eval/main.go
    │
    │  -mcp-config eval/mcp_configs/filesystem.json
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│  1. LOAD CONFIG                                         │
│                                                         │
│  loadMCPConfig(path)                                    │
│      └─ os.ReadFile → json.Unmarshal                    │
│      └─ map[string]types.McpServerConfig                │
│      └─ validates non-empty map                         │
│                                                         │
│  Example config:                                        │
│  {                                                      │
│    "filesystem": {                                      │
│      "type": "stdio",                                   │
│      "command": "npx",                                  │
│      "args": ["-y", "@modelcontextprotocol/...", "/ws"] │
│    }                                                    │
│  }                                                      │
└──────────────────────┬──────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────┐
│  2. CONNECT                                             │
│                                                         │
│  mcpClient := mcp.NewClient(registry)                   │
│  for name, cfg := range servers {                       │
│      mcpClient.Connect(ctx, name, cfg)                  │
│  }                                                      │
│      └─ launches stdio subprocess per server            │
│      └─ JSON-RPC initialize handshake                   │
│      └─ registers mcp__{name}__{tool} in Registry       │
│      └─ non-fatal on failure (logs warning to stderr)   │
│                                                         │
│  File: pkg/mcp/client.go                                │
└──────────────────────┬──────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────┐
│  3. REGISTER RESOURCE TOOLS                             │
│                                                         │
│  registry.Register(ListMcpResourcesTool{Client: mcp})   │
│  registry.Register(ReadMcpResourceTool{Client: mcp})    │
│                                                         │
│  config.MCPServers = servers                            │
│      └─ assembler injects MCP section into system prompt│
│                                                         │
│  Files:                                                 │
│    pkg/tools/mcp_resources.go                           │
│    pkg/prompt/assembler.go:82                           │
└─────────────────────────────────────────────────────────┘
```

## Multi-Turn REPL Mode

When `-multi-turn` is passed, the binary enters a conversational REPL
that reads follow-up prompts from stdin after each turn completes:

```
stdin                  cmd/eval/main.go              agent.Query
─────                  ────────────────              ───────────
"What is 2+2?"  ──▶   initial promptText    ──▶    RunLoop(ctx, prompt, config)
                                                        │
                       ┌────────────────────────────────┘
                       │  goroutine: range query.Messages()
                       │    AssistantMessage → lastText
                       │    ResultMessage(SuccessTurn) ──▶ turnDone channel
                       │
stdout: "4"    ◀──     │  print lastText
stderr: {"turn":1}     │  printTurnMeta(m)
                       │
                       │  ◀── turnDone signal
                       │
"Multiply by 3" ──▶   stdinScanner.Scan()
                       │
                       └──▶ query.SendUserMessage(line)
                               │
                               ▼
                           agent loop resumes
                               │
stdout: "12"   ◀──     ResultMessage(SuccessTurn)
stderr: {"turn":2}     printTurnMeta(m)
                       │
EOF            ──▶     scanner returns false
                       │
                       └──▶ query.Close()
                           query.Wait()
```

Key sync: the `turnDone` channel prevents reading the next stdin line
until the current turn is fully complete (`ResultSubtypeSuccessTurn`).

## Eval Binary Usage

```bash
# Baseline (no skills, no MCP)
goat-eval -prompt "Write a Go function..." -max-turns 10

# With skills
goat-eval -prompt "Write a Go function..." -max-turns 10 \
          -skills-dir ./eval/skills

# With MCP servers
goat-eval -prompt "List files in /workspace" \
          -mcp-config eval/mcp_configs/filesystem.json

# With skills + MCP
goat-eval -prompt "Read the config and fix the bug" -max-turns 15 \
          -skills-dir ./eval/skills \
          -mcp-config eval/mcp_configs/filesystem.json

# Multi-turn REPL
echo -e "What is 2+2?\nMultiply by 3" | goat-eval -multi-turn

# Skills dir layout (standard .claude convention):
eval/skills/
  .claude/skills/
    go-expert/SKILL.md        # Go idioms, error handling, stdlib
    project-context/SKILL.md  # Simulated project knowledge
    testing-patterns/SKILL.md # Table-driven tests, mocks, fixtures

# MCP config dir:
eval/mcp_configs/
  filesystem.json             # @modelcontextprotocol/server-filesystem
```

## A/B Benchmark Mode (2-way and 3-way)

The Modal sandbox runner (`scripts/modal_sandbox.py`) supports A/B testing
with skills, MCP, or both:

```bash
# 2-way: baseline vs +skills
python scripts/modal_sandbox.py \
    --batch eval/benchmark_skills.json \
    --skills-dir eval/skills --ab

# 2-way: baseline vs +mcp
python scripts/modal_sandbox.py \
    --batch eval/benchmark_skills.json \
    --mcp-config eval/mcp_configs/filesystem.json --ab

# 3-way: baseline vs +skills vs +skills+mcp
python scripts/modal_sandbox.py \
    --batch eval/benchmark_skills.json \
    --skills-dir eval/skills \
    --mcp-config eval/mcp_configs/filesystem.json --ab
```

### 3-Way Comparison Flow

```
                        ┌─────────────────────┐
                        │  benchmark.json      │
                        │  N tasks             │
                        └──────────┬──────────┘
                                   │
              ┌────────────────────┼────────────────────┐
              │                    │                    │
              ▼                    ▼                    ▼
    ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
    │  Baseline Run   │  │  Skills Run     │  │  Skills+MCP Run │
    │  (no extras)    │  │  (+skills)      │  │  (+skills+mcp)  │
    │                 │  │                 │  │                 │
    │  N sandboxes    │  │  N sandboxes    │  │  N sandboxes    │
    │  goat-eval      │  │  goat-eval      │  │  goat-eval      │
    │  -prompt "..."  │  │  -prompt "..."  │  │  -prompt "..."  │
    │                 │  │  -skills-dir    │  │  -skills-dir    │
    │                 │  │    /opt/skills  │  │    /opt/skills  │
    │                 │  │                 │  │  -mcp-config    │
    │                 │  │                 │  │    /opt/mcp-..  │
    └────────┬────────┘  └────────┬────────┘  └────────┬────────┘
             │                    │                    │
             └────────────────────┼────────────────────┘
                                  ▼
                        ┌─────────────────┐
                        │  summary.json   │
                        │                 │
                        │  baseline: 12/15│
                        │  +skills:  14/15│
                        │  +skills+mcp:   │
                        │           14/15 │
                        │                 │
                        │  Per-task:       │
                        │  ┌─────────────┐│
                        │  │task-1:      ││
                        │  │ base: PASS  ││
                        │  │ skill: PASS ││
                        │  │ mcp:  PASS  ││
                        │  └─────────────┘│
                        └─────────────────┘
```

### Sandbox MCP Wiring

When `--mcp-config` is passed, the sandbox image includes Node.js/npm
and the config file is mounted at `/opt/mcp-config.json`:

```
scripts/modal_sandbox.py
    │
    │  --mcp-config eval/mcp_configs/filesystem.json
    │
    ▼
┌────────────────────────────────────────────────────┐
│  build_sandbox_image(mcp_config=path)              │
│    └─ base image: debian_slim + bash, ripgrep,     │
│       git, curl, nodejs, npm                       │
│    └─ .add_local_file(path, "/opt/mcp-config.json")│
└────────────────────────┬───────────────────────────┘
                         │
                         ▼
┌────────────────────────────────────────────────────┐
│  Sandbox runs:                                     │
│  /opt/goat-eval -prompt "..." \                    │
│    -mcp-config /opt/mcp-config.json                │
│                                                    │
│  goat-eval connects to MCP servers:                │
│    npx @modelcontextprotocol/server-filesystem /ws │
│    → registers mcp__filesystem__* tools            │
│    → agent can read/write/list sandbox files       │
└────────────────────────────────────────────────────┘
```

## Benchmark Task Categories

Each skill has 5 benchmark tasks across 3 categories:

```
┌──────────────────────────────────────────────────────┐
│  skill_relevant: true     │  Tasks where the skill   │
│  (3 per skill)            │  knowledge is directly   │
│                           │  applicable. Skills      │
│                           │  should measurably help. │
├───────────────────────────┼──────────────────────────┤
│  skill_relevant: false    │  Baseline tasks where    │
│  target_skill: same       │  skills shouldn't help   │
│  (1 per skill)            │  or hurt. Controls for   │
│                           │  skill overhead.         │
├───────────────────────────┼──────────────────────────┤
│  skill_relevant: false    │  Adjacent-domain tasks.  │
│  target_skill: same       │  Tests if skill knowledge│
│  (1 per skill)            │  generalizes.            │
└───────────────────────────┴──────────────────────────┘
```

## Test Skills

| Skill | Tokens | Tests |
|-------|--------|-------|
| `go-expert` | ~1k | Error wrapping, errgroup concurrency, functional options |
| `project-context` | ~1.1k | Add endpoint, add entity, write middleware |
| `testing-patterns` | ~1.7k | Table-driven tests, mock interfaces, httptest |

## Key Files

| File | Purpose |
|------|---------|
| `cmd/eval/main.go` | `-skills-dir`, `-mcp-config`, `-multi-turn` flags, all wiring |
| `cmd/eval/mcp_test.go` | Tests for `loadMCPConfig` (5 tests) |
| `cmd/eval/multiturn_test.go` | Tests for `printTurnMeta`, `extractText` (3 tests) |
| `cmd/eval/skills_integration_test.go` | Pipeline test: load → register → adapt → invoke |
| `cmd/eval/skills_e2e_test.go` | Live LLM test: secret-word skill invoked by gpt-4o-mini |
| `eval/skills/.claude/skills/*/SKILL.md` | 3 benchmark skills |
| `eval/mcp_configs/filesystem.json` | Example MCP config for sandbox filesystem access |
| `eval/benchmark_skills.json` | 15 skill-specific benchmark tasks |
| `scripts/modal_sandbox.py` | `--skills-dir`, `--mcp-config`, `--ab` flags for benchmarking |
| `pkg/mcp/client.go` | MCP client: connects servers, registers dynamic tools |
| `pkg/prompt/skill_loader.go` | Filesystem skill discovery |
| `pkg/prompt/skill_registry.go` | Thread-safe skill storage |
| `pkg/tools/skilltool.go` | LLM-callable Skill tool |
| `pkg/tools/skill_adapter.go` | Bridges registry → tool interfaces |
