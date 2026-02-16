# Goat v0.1

Go port of the [Claude Code](https://docs.anthropic.com/en/docs/agents-and-tools/claude-code/overview) agentic loop.

Goat implements the full agent lifecycle -- LLM streaming, tool execution, permission checking, context compaction, session persistence, subagent spawning, team coordination, MCP integration, and prompt assembly -- as a Go library with an OpenAI-compatible LLM backend. It speaks SSE to any `/v1/chat/completions` endpoint (OpenAI, Anthropic, Groq, Azure via LiteLLM, self-hosted vLLM).

## Project Layout

```
goat/
├── cmd/
│   ├── eval/          Headless eval binary for benchmarks
│   └── example/       Interactive multi-provider example
├── pkg/               Core library (13 packages, ~250 Go files)
├── dev/               Docker Compose dev stack (LiteLLM, Langfuse, Postgres)
├── eval/              Eval benchmark assets (skills, MCP configs)
├── evals/             Python-based inspect_ai benchmark harness
├── scripts/           Modal cloud infra, build scripts, cross-model runners
├── docs/              Detailed docs (quickstart, diagrams, evals)
└── thoughts/          Specs and plans (archived)
```

### Packages (`pkg/`)

- **`agent`** -- Core agentic loop. State machine, `RunLoop()` entry point, `Query` channel-based message stream, all interface definitions.
- **`llm`** -- OpenAI-compatible streaming client. SSE parser, tool call accumulator, cost tracker, retry with backoff.
- **`tools`** -- Tool interface + 22 implementations (Bash, Read, Write, Edit, Glob, Grep, WebFetch, WebSearch, Agent, MCP, etc.) + task manager for background processes.
- **`types`** -- SDK message types, content blocks, control protocol.
- **`prompt`** -- System prompt assembly from 133 embedded Piebald v2.1.37 prompt files. Skill loader with fsnotify hot-reload.
- **`permission`** -- 7-layer permission checker (mode, rules, hooks, prompter). Risk classification, glob/regex matching.
- **`hooks`** -- Lifecycle hook system. Go callbacks + shell commands on 15 events.
- **`context`** -- Context window compaction. LLM-powered summarization with truncation fallback.
- **`session`** -- JSONL file-based persistence. Async writer, checkpoints, rewind.
- **`subagent`** -- 12-step subagent spawn flow. 6 built-in agent types, resume support.
- **`teams`** -- Multi-agent team coordination. Mailbox messaging, shared tasks, gate-based sync.
- **`mcp`** -- Model Context Protocol client. JSON-RPC over stdio and HTTP/SSE transports.
- **`transport`** -- Communication layer: channel, stdio, WebSocket, SSE transports.

## Quick Start

**Prerequisites:** Go 1.24+, a `.env` file with at least one provider API key.

```bash
# Build
go build ./...

# Test (800+ tests, no network calls)
go test -race ./...

# Run the interactive example (with tools -- agent can read files, run commands, etc.)
go run ./cmd/example/ -provider litellm -prompt "List files in this directory" -max-turns 3
go run ./cmd/example/ -provider openai -prompt "What is 2+2?" -max-turns 3

# Pure chat (no tools)
go run ./cmd/example/ -provider openai -no-tools -prompt "What is 2+2?"
go run ./cmd/example/ -provider litellm -no-tools -prompt "What is 2+2?"

# Other providers
go run ./cmd/example/ -provider groq -prompt "List files here"
go run ./cmd/example/ -provider anthropic -prompt "Read go.mod"
```

**Verified working** (2026-02-16): OpenAI direct (gpt-4o-mini) and LiteLLM (gpt-5-nano) both pass with tools on and off. LiteLLM is the recommended path -- it provides accurate cost tracking and routes to any backend (Azure, Groq, self-hosted vLLM). OpenAI direct works but cost reports `$0.000000` since OpenAI lacks a pricing discovery endpoint.

See [docs/quickstart](docs/quickstart/README.md) for full provider config, multi-turn mode, dev proxy setup, and code examples.

## Using as a Library

```bash
go get github.com/jg-phare/goat
```

Minimal example:

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/llm"
	"github.com/jg-phare/goat/pkg/types"
)

func main() {
	client := llm.NewClient(llm.ClientConfig{
		BaseURL: "https://api.groq.com/openai/v1",
		APIKey:  os.Getenv("GROQ_API_KEY"),
		Model:   "llama-3.3-70b-versatile",
	})

	cwd, _ := os.Getwd()
	registry := agent.DefaultRegistry(cwd, nil)

	config := agent.DefaultConfig()
	config.LLMClient = client
	config.Model = "llama-3.3-70b-versatile"
	config.ToolRegistry = registry
	config.CWD = cwd
	config.MaxTurns = 5

	ctx := context.Background()
	query := agent.RunLoop(ctx, "What is 2 + 2?", config)

	for msg := range query.Messages() {
		switch m := msg.(type) {
		case types.AssistantMessage:
			for _, block := range m.Message.Content {
				if block.Type == "text" {
					fmt.Println(block.Text)
				}
			}
		case *types.AssistantMessage:
			for _, block := range m.Message.Content {
				if block.Type == "text" {
					fmt.Println(block.Text)
				}
			}
		}
	}

	query.Wait()
	fmt.Printf("Turns: %d, Cost: $%.6f\n", query.TurnCount(), query.TotalCostUSD())
}
```

See [docs/quickstart](docs/quickstart/README.md) for provider variants (OpenAI, Anthropic, LiteLLM), multi-turn conversations, and permission configuration.

## Eval Binary

`cmd/eval/` is a headless binary that runs the agentic loop in sandboxed environments and prints the final result to stdout. Used by both benchmark systems.

### Environment variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `OPENAI_BASE_URL` | `http://localhost:13131/v1` | LLM endpoint |
| `OPENAI_API_KEY` | `"inspect"` | API key |
| `EVAL_MODEL` | `"inspect"` | Model ID |

### Flags

| Flag | Default | Purpose |
|------|---------|---------|
| `-prompt` | stdin | Prompt text |
| `-cwd` | current dir | Tool working directory |
| `-max-turns` | 100 | Max agentic loop iterations |
| `-skills-dir` | -- | Load skills from `.claude/skills/*/SKILL.md` |
| `-mcp-config` | -- | JSON file with MCP server configurations |
| `-multi-turn` | false | REPL mode: read follow-up prompts from stdin |

### Two eval paths

**Local (Inspect AI)** -- standardized academic benchmarks (HumanEval, Terminal-Bench, SWE-bench) running in Docker containers via the [UK AISI inspect_ai](https://inspect.aisi.org.uk/) framework.

```bash
# Build static Linux binary
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o evals/goat-eval-linux ./cmd/eval/

# Run HumanEval (164 samples, ~5 min, ~$0.50)
evals/.venv/bin/inspect eval evals/humaneval.py \
  --model openai/gpt-5-nano \
  --model-base-url http://localhost:4000/v1
```

See [docs/evals](docs/evals/README.md) for full setup, all benchmarks, and troubleshooting.

**Cloud (Modal)** -- parallel sandboxed execution with A/B testing of skills and MCP augmentations. Supports cross-model comparison and Langfuse observability.

Requires one-time setup: `uv tool install modal && modal setup`, then deploy the LiteLLM proxy and supporting services onto the `goat` Modal environment:

```bash
uv run --with modal python scripts/modal_setup.py     # create goat environment + secrets
modal deploy scripts/modal_services.py --env goat      # deploy LiteLLM + Langfuse + Postgres
bash scripts/build_eval.sh                             # build goat-eval-linux binary
```

Optionally deploy local GPU models via vLLM (each scales to zero independently):

```bash
VLLM_MODEL=llama-3.1-8b modal deploy scripts/modal_vllm.py --env goat
VLLM_MODEL=qwen3-4b modal deploy scripts/modal_vllm.py --env goat
```

Then run evals:

```bash
# Single task
uv run --with modal python scripts/modal_sandbox.py --prompt "What is 2+2?"

# Batch with A/B (baseline vs +skills vs +skills+mcp)
uv run --with modal python scripts/modal_sandbox.py --batch scripts/benchmark_smoke.json \
  --skills-dir eval/skills --mcp-config eval/mcp_configs/filesystem.json --ab
```

The sandbox auto-discovers the LiteLLM proxy URL from the deployed `goat-services` app. Without it, sandboxes cannot reach an LLM endpoint.

See [scripts/README.md](scripts/README.md) for Modal deployment, vLLM GPU serving, and infrastructure details.

## Benchmark Results

Baseline run: 2026-02-09, `gpt-5-nano` via LiteLLM, macOS ARM64 Docker.

| Benchmark | Samples | Accuracy | Runtime | Cost |
|-----------|---------|----------|---------|------|
| HumanEval | 164/164 | **42.1%** | 4m 57s | ~$0.50 |
| Terminal-Bench 2.0 | 89/89 | **1.1%** | 17m 45s | ~$0.30 |
| SWE-bench | -- | not yet run | -- | -- |

Modal smoke benchmark (5 tasks): **5/5 pass** across all 5 models (gpt-4o-mini, gpt-5-nano, gpt-5-mini, llama-3.3-70b, llama-3.1-8b-local).

These are goat-only numbers. Claude Code head-to-head comparison is the next milestone.

See [docs/evals/benchmark-results.md](docs/evals/benchmark-results.md) for full results and cross-model plans.

## Roadmap: Claude Code Parity

### Done

- Core agentic loop (state machine, multi-turn, interrupts, exit reasons)
- LLM client (SSE streaming, retry with backoff, cost tracking, dynamic pricing)
- 22 tools (Bash, Read, Write, Edit, Glob, Grep, WebFetch, WebSearch, Agent, Notebook, Todo, MCP, Teams, etc.)
- System prompt assembly (133 Piebald v2.1.37 prompt files, conditional sections)
- Permission system (7-layer checker, rules, modes, skill-scoped permissions)
- Hook lifecycle (shell + Go callbacks, 15 events, async execution)
- Context compaction (LLM-powered summarization + session memory + truncation fallback)
- Session persistence (JSONL, async writer, checkpoints, rewind, cleanup)
- Subagent spawning (12-step flow, 6 built-in types, resume, hot-reload)
- Team coordination (mailbox messaging, shared tasks, gate-based sync)
- MCP client (JSON-RPC over stdio + HTTP/SSE, dynamic tool registration)
- Transport layer (channel, stdio, WebSocket, SSE)
- Skill system (loader, registry, fsnotify hot-reload, eval benchmark skills)
- Eval infra: Inspect AI wrappers (HumanEval, Terminal-Bench, SWE-bench)
- Eval infra: Modal sandboxes with A/B skill/MCP testing, cross-model runner

### In Progress

- Cross-model benchmark sweep (runner ready, only gpt-5-nano baseline done)
- SWE-bench first run (blocked on 280GB Docker image pull)

### Planned

- Claude Code head-to-head: run identical evals on Claude Code CLI and goat, publish comparison table
- Interactive TUI/CLI frontend (goat is library-only today)
- Production permission prompter (current options: AllowAll or StubPrompter)
- WebSearch/WebFetch with real providers (current: stub search provider)

## Known Caveats

- **LiteLLM recommended over direct APIs.** Cost tracking only works via LiteLLM (direct OpenAI/Groq/Anthropic lack pricing discovery). LiteLLM also handles retries and timeout edge cases.
- Direct Groq API may hang with tools enabled due to SSE connection stalls. Route Groq through LiteLLM instead.
- No standalone CLI. Goat is a library -- embed it in your own program, or use `cmd/example/` for interactive use and `cmd/eval/` for benchmarks.

## Documentation

- [Quick Start](docs/quickstart/README.md) -- install, run, embed, provider config
- [Architecture Diagrams](docs/diagrams/README.md) -- 17 diagrams covering all packages
- [Eval Benchmarks](docs/evals/README.md) -- Inspect AI setup, running benchmarks, results
- [Modal Infrastructure](scripts/README.md) -- cloud deployment, vLLM, Langfuse, sandboxes
- [Changelog](changelog.md) -- detailed change history
