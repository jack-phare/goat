# Goat Modal Sandbox

Run Goat's eval binary in isolated [Modal](https://modal.com) containers. The sandbox provides safe tool execution — Bash commands and file operations run inside the container, not on your host machine.

```
Host (macOS/Linux)          Modal Sandbox (Debian)        LLM Provider
┌─────────────────┐        ┌───────────────────────┐     ┌──────────────┐
│ modal_sandbox.py │──────▶│ /opt/goat-eval        │────▶│ Azure/OpenAI │
│                  │  SSH   │ bash, git, ripgrep    │ API │              │
│ (cross-compiled  │  ───▶ │ /workspace (CWD)      │────▶│              │
│  binary upload)  │       │ /results (Volume)      │     │              │
└─────────────────┘        └───────────────────────┘     └──────────────┘
```

## Prerequisites

- [Go](https://golang.org) (for building the eval binary)
- [uv](https://docs.astral.sh/uv/) (Python package manager)
- [Modal](https://modal.com) account

## One-Time Setup

### 1. Install Modal CLI

```bash
uv tool install modal
modal setup  # Opens browser to authenticate
```

### 2. Create Modal Secret

Create a secret in the `agent-dev` environment with your LLM provider credentials:

```bash
modal secret create -e agent-dev goat-llm-secret \
    OPENAI_BASE_URL=https://your-endpoint.openai.azure.com \
    OPENAI_API_KEY=your-api-key \
    EVAL_MODEL=gpt-5-nano
```

The secret must contain:
- `OPENAI_BASE_URL` — Your LLM endpoint (Azure OpenAI, OpenAI API, etc.)
- `OPENAI_API_KEY` — API key for the endpoint
- `EVAL_MODEL` — Default model ID (can be overridden with `--model`)

### 3. Build the Eval Binary

Cross-compile for Linux/amd64 (Modal's target architecture):

```bash
bash scripts/build_eval.sh
```

This produces `scripts/goat-eval-linux` (a statically linked ELF binary, ~50MB). Rebuild whenever you change `cmd/eval/` or its dependencies.

## Usage

### Single Prompt

```bash
# Simple question
uv run --with modal python scripts/modal_sandbox.py --prompt "What is 2+2?"

# Tool use with more turns
uv run --with modal python scripts/modal_sandbox.py \
    --prompt "Create a hello.py that prints hello world" \
    --max-turns 10

# Override model
uv run --with modal python scripts/modal_sandbox.py \
    --prompt "Explain Go interfaces" \
    --model gpt-5-mini
```

### Batch Mode

Create a JSON file with your prompts:

```json
[
  {"id": "simple-math", "prompt": "What is 2+2?", "max_turns": 3},
  {"id": "file-create", "prompt": "Create hello.py that prints hello world", "max_turns": 10},
  {"id": "grep-find", "prompt": "Find all .go files containing 'func main'", "max_turns": 5}
]
```

Run the batch:

```bash
uv run --with modal python scripts/modal_sandbox.py --batch prompts.json
```

Each task result is saved to the Modal Volume as JSON with output, exit code, stderr, and timing.

### Viewing Results

```bash
# List all runs
uv run --with modal python scripts/modal_results.py

# View a specific run
uv run --with modal python scripts/modal_results.py 20260209_143000

# Full output (no truncation)
uv run --with modal python scripts/modal_results.py 20260209_143000 --full
```

## CLI Reference

### `modal_sandbox.py`

| Flag | Default | Description |
|------|---------|-------------|
| `--prompt` | (required\*) | Single prompt to run |
| `--batch` | (required\*) | Path to JSON batch file |
| `--model` | from secret | Override EVAL_MODEL |
| `--max-turns` | 10 | Max agentic loop turns |
| `--timeout` | 600 | Sandbox timeout (seconds) |

\* One of `--prompt` or `--batch` is required.

### `modal_results.py`

| Flag | Default | Description |
|------|---------|-------------|
| `run_id` | (optional) | Run ID to view (omit to list all) |
| `--full` | false | Show complete output |

## Troubleshooting

### "goat-eval-linux not found"
Run `bash scripts/build_eval.sh` to compile the binary.

### "Secret not found"
Create the Modal secret: `modal secret create -e agent-dev goat-llm-secret ...` (see setup above).

### Sandbox timeout
Increase with `--timeout 1200` (seconds). Default is 600s (10 minutes).

### Binary not executable / wrong architecture
Ensure you're cross-compiling with `CGO_ENABLED=0 GOOS=linux GOARCH=amd64`. The build script handles this automatically.

### Network errors from sandbox
Modal sandboxes have outbound internet access by default. Verify your `OPENAI_BASE_URL` is reachable from Modal's infrastructure (not `localhost` or private network).
