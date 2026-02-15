# Modal Eval Sandboxes

Run the `goat-eval` binary in isolated Modal sandboxes. Each task gets its own Debian container with the binary and tools.

## Prerequisites

```bash
# Build the eval binary for Linux
bash scripts/build_eval.sh
```

This cross-compiles `goat-eval-linux` with `CGO_ENABLED=0 GOOS=linux`.

## Running Evals

```bash
# Single prompt
python scripts/modal_sandbox.py --prompt "Write a Python function that reverses a string"

# Specific model
python scripts/modal_sandbox.py --prompt "Write fizzbuzz" --model gpt-5-mini

# Local GPU model (after deploying vLLM)
python scripts/modal_sandbox.py --prompt "Hello" --model llama-3.1-8b-local

# Batch mode with parallelism
python scripts/modal_sandbox.py --batch scripts/benchmark_smoke.json --parallel 4

# With MCP servers
python scripts/modal_sandbox.py --prompt "List files" \
  --mcp-config eval/mcp_configs/filesystem.json

# A/B comparison: baseline vs skills vs MCP (3-way)
python scripts/modal_sandbox.py \
  --batch eval/benchmark_skills.json \
  --skills-dir eval/skills \
  --mcp-config eval/mcp_configs/filesystem.json \
  --ab
```

## Viewing Results

```bash
# List all runs
python scripts/modal_results.py

# Full details for a run
python scripts/modal_results.py <run_id> --full
```

Results stored on the `goat-results` Modal volume.

## Cross-Model Benchmarks

```bash
# Runs smoke benchmark across multiple models
bash scripts/run_cross_model_modal.sh
```

Tests: `gpt-5-nano`, `gpt-5-mini`, `gpt-4o-mini`, `llama-3.3-70b`, and any deployed local models.

## Infrastructure Validation

```bash
# 6-phase test: prerequisites → setup → deploy → smoke → eval → GPU
bash scripts/test_modal_infra.sh
```

## How It Works

`modal_sandbox.py` creates a Debian-based Modal image with the `goat-eval-linux` binary. Each sandbox:
- Gets `OPENAI_BASE_URL` pointing to the Modal LiteLLM proxy
- Gets `OPENAI_API_KEY` and `EVAL_MODEL` from secrets/args
- Runs the binary with the prompt and tool set
- Results are written to the `goat-results` volume

## Batch File Format

JSON array of tasks (see `scripts/benchmark_smoke.json`):

```json
[
  {"id": "task-1", "prompt": "What is 2+2?"},
  {"id": "task-2", "prompt": "Create a file called hello.txt with 'Hello World'"}
]
```
