# Eval Benchmarks

Goat uses the [UK AISI inspect_ai](https://inspect.aisi.org.uk/) framework to run standardized benchmarks against the agentic loop.

## Architecture

```
┌─────────────┐     ┌──────────────┐     ┌──────────────┐
│  inspect_ai  │────▶│ Docker sandbox│────▶│  goat-eval   │
│  (host)      │     │  (container)  │     │  (binary)    │
└─────────────┘     └──────┬───────┘     └──────┬───────┘
                           │                     │
                           │              ┌──────▼───────┐
                           │              │  LiteLLM     │
                           │              │  proxy       │
                           └──────────────┤  (host:4000) │
                                          └──────────────┘
```

1. **inspect_ai** orchestrates the benchmark on the host (loads datasets, scores results)
2. Each sample runs in a **Docker sandbox** container
3. The **goat-eval binary** (statically compiled Go) runs the agentic loop inside the container
4. Goat calls **LiteLLM** on the host directly via `host.docker.internal:4000`

For benchmarks that provide their own Docker environments (e.g. Terminal-Bench), the goat-eval binary is injected into the challenge container at runtime.

## Prerequisites

- **Go 1.24+** (for building the eval binary)
- **Docker Desktop** (for sandbox containers)
- **Python 3.12+** (for inspect_ai)
- **uv** (Python package manager)
- **LiteLLM proxy** running on port 4000

## Quick Setup

### 1. Start LiteLLM proxy

```bash
docker compose -f dev/docker-compose.yml up -d litellm

# Verify it's running
curl -s http://localhost:4000/health
```

### 2. Set up Python environment

```bash
cd evals

# Create venv and install dependencies
uv venv
uv pip install -e .
uv pip install openai inspect-cyber

# Verify
.venv/bin/inspect --version
```

### 3. Build the eval binary

For benchmarks using the default sandbox (HumanEval), the Docker image is built automatically from `evals/Dockerfile`. No manual build needed.

For benchmarks that provide their own containers (Terminal-Bench, SWE-bench), build a static Linux binary:

```bash
# From the project root
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o evals/goat-eval-linux ./cmd/eval/
```

> **Note**: Use `GOARCH=amd64` if running on Intel/AMD Docker hosts.

### 4. Set environment variables

```bash
# Read API key from .env file
KEY=$(grep EXECUTOR_LITELLM_KEY .env | head -1 | cut -d= -f2)

export OPENAI_BASE_URL=http://host.docker.internal:4000/v1
export OPENAI_API_KEY="$KEY"
export EVAL_MODEL=gpt-5-nano
```

## Running Benchmarks

All commands are run from the project root.

### HumanEval (code generation)

164 samples, ~5 minutes, ~$0.50 with gpt-5-nano.

```bash
evals/.venv/bin/inspect eval evals/humaneval.py \
  --model openai/gpt-5-nano \
  --model-base-url http://localhost:4000/v1
```

### Terminal-Bench 2.0 (terminal/CLI challenges)

89 samples, ~18 minutes, ~$0.30 with gpt-5-nano.

> **First run**: Downloads ~15GB of pre-built challenge Docker images. Subsequent runs use cached images.

```bash
# Requires the static binary (see step 3 above)
evals/.venv/bin/inspect eval evals/terminal_bench.py \
  --model openai/gpt-5-nano \
  --model-base-url http://localhost:4000/v1
```

### SWE-bench Verified (real-world bug fixing)

Full dataset is large. Use `--limit` for a subset.

> **First run**: Downloads ~280GB of Docker images. Plan accordingly.

```bash
evals/.venv/bin/inspect eval evals/swe_bench.py \
  --model openai/gpt-5-nano \
  --model-base-url http://localhost:4000/v1 \
  --limit 50
```

## Viewing Results

```bash
# Launch the inspect web viewer
evals/.venv/bin/inspect view

# Logs are stored in logs/
ls -lt logs/
```

## Resuming Interrupted Runs

If a run is interrupted (Docker error, timeout, Ctrl+C), resume it:

```bash
evals/.venv/bin/inspect eval-retry logs/<log-file>.eval
```

## Using Different Models

Replace `gpt-5-nano` with any model available on your LiteLLM proxy:

```bash
# With gpt-5-mini
evals/.venv/bin/inspect eval evals/humaneval.py \
  --model openai/gpt-5-mini \
  --model-base-url http://localhost:4000/v1

# With Claude
evals/.venv/bin/inspect eval evals/humaneval.py \
  --model openai/claude-sonnet-4-5 \
  --model-base-url http://localhost:4000/v1
```

## Goat-Eval Binary Flags

The headless eval binary (`cmd/eval/main.go`) supports these flags:

| Flag | Description |
|------|-------------|
| `-prompt` | Inline prompt text (otherwise reads from stdin) |
| `-cwd` | Working directory for the agent (default: current dir) |
| `-max-turns` | Maximum agentic turns (default: 10) |
| `-skills-dir` | Path to skills directory (loads `.claude/skills/*/SKILL.md`) |
| `-mcp-config` | Path to JSON file with MCP server configurations |
| `-multi-turn` | Enable multi-turn REPL mode (read follow-up prompts from stdin) |

### MCP Config

Wire MCP servers (e.g. filesystem access) into the eval binary:

```bash
goat-eval -prompt "List files in /workspace" \
  -mcp-config eval/mcp_configs/filesystem.json
```

Config format — maps server names to `McpServerConfig`:

```json
{
  "filesystem": {
    "type": "stdio",
    "command": "npx",
    "args": ["-y", "@modelcontextprotocol/server-filesystem", "/workspace"]
  }
}
```

On startup, the binary connects to each server (non-fatal on failure), registers `mcp__*` tools dynamically, and injects MCP metadata into the system prompt.

### Multi-Turn REPL

For conversational benchmarks, use `-multi-turn` to send follow-up prompts via stdin:

```bash
# Pipe multi-turn input
echo -e "What is 2+2?\nNow multiply by 3" | \
  goat-eval -multi-turn

# Or use -prompt for the first turn, stdin for follow-ups
goat-eval -multi-turn -prompt "What is 2+2?" <<'EOF'
Now multiply by 3
What about divided by 2?
EOF
```

Each turn's response is printed to stdout. Turn metadata (turn number, cost) is emitted as JSON to stderr:
```json
{"turn":1,"cost_usd":0.000150}
```

EOF or an empty line triggers graceful shutdown. Single-shot mode (no `-multi-turn`) is unchanged.

## File Structure

```
evals/
├── Dockerfile         # Multi-stage build: Go builder → Python runtime
├── compose.yaml       # Docker sandbox config (4 CPUs, 8GB RAM)
├── pyproject.toml     # Python dependencies (inspect-ai, inspect-evals)
├── goat_solver.py     # Inspect agent solver (runs goat-eval in sandbox)
├── goat-eval-linux    # Pre-built static binary (gitignored)
├── humaneval.py       # HumanEval task wrapper
├── terminal_bench.py  # Terminal-Bench 2.0 task wrapper
└── swe_bench.py       # SWE-bench Verified task wrapper

cmd/eval/
├── main.go            # Headless eval binary source
├── mcp_test.go        # Tests for loadMCPConfig
├── multiturn_test.go  # Tests for multi-turn helpers
└── skills_*_test.go   # Skill integration + E2E tests

eval/mcp_configs/
└── filesystem.json    # Example MCP config for sandbox filesystem access

logs/                  # Inspect eval logs (gitignored)
```

## Troubleshooting

### "No services started" / Docker build errors

Usually a transient Docker Hub issue. Retry the command. If persistent, check Docker Desktop is running and has internet access.

### "goat-eval exit=127"

The binary isn't found in the sandbox. For benchmarks with custom containers (Terminal-Bench), ensure `evals/goat-eval-linux` exists:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o evals/goat-eval-linux ./cmd/eval/
```

### LiteLLM connection errors

Verify the proxy is running and accessible from Docker:

```bash
curl -s http://localhost:4000/health
docker run --rm --add-host=host.docker.internal:host-gateway alpine wget -qO- http://host.docker.internal:4000/health
```

### Slow first run

Terminal-Bench and SWE-bench download large Docker images on first run (15GB and 280GB respectively). These are cached for subsequent runs.
