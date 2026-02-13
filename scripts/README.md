# Goat Modal Infrastructure

Run Goat benchmarks on Modal's cloud with LiteLLM routing, Langfuse observability,
and optional local GPU model serving via vLLM.

## Architecture

```
Your Machine                     Modal Cloud (goat environment)
─────────────                    ─────────────────────────────────
                                 ┌─────────────────────────────┐
modal_sandbox.py ──Modal SDK──▶  │  Sandbox (per task, CPU)    │
                                 │  goat-eval binary           │
                                 │  Bash/Read/Write/Edit/Glob/ │
                                 │  Grep tools                 │
                                 └─────────┬──────────────────┘
                                           │ OPENAI_BASE_URL
                                           ▼
                                 ┌─────────────────────────────┐
                                 │  LiteLLM Proxy (port 4000)  │
                                 │  Routes to:                 │
                                 │   - Azure (gpt-5-nano/mini) │
                                 │   - Groq (llama-3.3-70b)    │
                                 │   - vLLM local (see below)  │
                                 └────┬──────────┬────────────┘
                                      │          │
                                      ▼          ▼
                                 ┌──────────┐ ┌────────────────────────┐
                                 │ Langfuse │ │ vLLM (per-model apps)  │
                                 │ (traces) │ │ goat-vllm-llama-3-1-8b │
                                 └────┬─────┘ │ goat-vllm-qwen3-4b    │
                                      ▼       │ goat-vllm-gpt-oss-20b │
                                 ┌──────────┐ │ ...etc (scale to zero) │
                                 │ Postgres  │ └────────────────────────┘
                                 └──────────┘
```

## Quick Start

### 1. Install Modal

```bash
uv tool install modal
modal setup
```

### 2. Set Up Environment and Secrets

```bash
# Interactive setup -- reads from .env, prompts for confirmation
python scripts/modal_setup.py

# Or dry-run first to see what will be created
python scripts/modal_setup.py --dry-run
```

### 3. Deploy Services

```bash
# Deploy Postgres + Langfuse + LiteLLM
modal deploy scripts/modal_services.py --env goat

# Deploy vLLM local model servers (one app per model, each scales to zero)
VLLM_MODEL=llama-3.1-8b modal deploy scripts/modal_vllm.py --env goat
VLLM_MODEL=qwen3-4b modal deploy scripts/modal_vllm.py --env goat
VLLM_MODEL=qwen3-30b-a3b modal deploy scripts/modal_vllm.py --env goat

# Deploy GPT OSS models (dedicated script: CUDA 12.8, vLLM 0.13.0, speculative decoding)
GPT_OSS_MODEL=gpt-oss-20b modal deploy scripts/modal_gpt_oss.py --env goat
```

### 4. Build the Eval Binary

```bash
bash scripts/build_eval.sh
```

### 5. Run Benchmarks

```bash
# Single prompt
python scripts/modal_sandbox.py --prompt "Write a Python function that reverses a string"

# With specific model
python scripts/modal_sandbox.py --prompt "Write fizzbuzz" --model gpt-5-mini

# Use local model (after deploying vLLM)
python scripts/modal_sandbox.py --prompt "Hello" --model llama-3.1-8b-local

# Batch mode with 4 parallel sandboxes
python scripts/modal_sandbox.py --batch prompts.json --parallel 4

# View results
python scripts/modal_results.py
python scripts/modal_results.py <run_id> --full
```

## Scripts

| Script | Purpose |
|--------|---------|
| `modal_setup.py` | Interactive environment + secrets creation |
| `modal_services.py` | Deploy Postgres, Langfuse, LiteLLM as Modal Functions |
| `modal_vllm.py` | Parameterized vLLM server — deploys Llama/Qwen models via `VLLM_MODEL` env var |
| `modal_gpt_oss.py` | GPT OSS server — CUDA 12.8, vLLM 0.13.0, speculative decoding via `GPT_OSS_MODEL` |
| `model_registry.py` | Reference doc for all model configs (HF IDs, GPUs, vLLM settings) |
| `modal_sandbox.py` | Run goat-eval in isolated per-task sandboxes |
| `modal_results.py` | View benchmark results from Modal volume |
| `verify_langfuse_traces.py` | Verify Langfuse traces via REST API + raw Postgres SQL |
| `langfuse_sync_local.sh` | Sync Modal Langfuse data to local docker-compose |
| `build_eval.sh` | Cross-compile goat-eval binary for Linux |
| `run_cross_model_modal.sh` | Run smoke benchmark across multiple models |
| `test_modal_infra.sh` | 6-phase infrastructure validation (secrets, deploy, health, traces) |

## Modal Secrets

Created by `modal_setup.py` in the `goat` environment:

| Secret | Contents |
|--------|----------|
| `goat-llm-providers` | Azure, Groq, OpenAI, Anthropic API keys |
| `goat-litellm` | LiteLLM master key |
| `goat-langfuse` | Langfuse auto-provision config |
| `goat-postgres` | Postgres credentials |

## Modal Volumes

| Volume | Purpose |
|--------|---------|
| `goat-langfuse-pg` | Langfuse Postgres data persistence (survives restarts) |
| `goat-results` | Benchmark results |
| `goat-model-cache` | HuggingFace model weights for vLLM |
| `goat-vllm-cache` | vLLM compilation cache |
| `goat-flashinfer-cache` | FlashInfer MoE kernel cache (GPT OSS) |

## Models Available Through LiteLLM

### Cloud Models

| Model Name | Provider | Notes |
|------------|----------|-------|
| `gpt-5-nano` | Azure OpenAI | Cheapest, good for testing |
| `gpt-5-mini` | Azure OpenAI | Better quality |
| `gpt-4o-mini` | Azure OpenAI | Previous gen |
| `llama-3.3-70b` | Groq | Fast open-weight |

### Local Models (vLLM on Modal)

All local models use the `-local` suffix. Deploy with parameterized scripts — each creates a uniquely-named Modal app that scales to zero independently.

| Model Name | HF Model | GPU | Deploy Command |
|------------|----------|-----|----------------|
| `llama-3.1-8b-local` | meta-llama/Llama-3.1-8B-Instruct | A10G | `VLLM_MODEL=llama-3.1-8b modal deploy scripts/modal_vllm.py` |
| `qwen3-4b-local` | Qwen/Qwen3-4B-Instruct-2507 | A10G | `VLLM_MODEL=qwen3-4b modal deploy scripts/modal_vllm.py` |
| `qwen3-30b-a3b-local` | Qwen/Qwen3-30B-A3B-Instruct-2507 | A10G | `VLLM_MODEL=qwen3-30b-a3b modal deploy scripts/modal_vllm.py` |
| `qwen3-235b-local` | Qwen/Qwen3-235B-A22B-Instruct-2507-FP8 | A100-80GB | `VLLM_MODEL=qwen3-235b modal deploy scripts/modal_vllm.py` |
| `gpt-oss-20b-local` | openai/gpt-oss-20b | H100 | `GPT_OSS_MODEL=gpt-oss-20b modal deploy scripts/modal_gpt_oss.py` |
| `gpt-oss-120b-local` | openai/gpt-oss-120b | H100 | `GPT_OSS_MODEL=gpt-oss-120b modal deploy scripts/modal_gpt_oss.py` |

> **GPU costs**: A10G (~$1.10/hr), A100-80GB (~$3.30/hr), H100 (~$4.50/hr). All apps scale to zero when idle (`scaledown_window=5min`).

> **Go code**: `IsLocalModel()` in `pkg/llm/translate.go` auto-detects `-local` suffix and applies `temperature: 0.3` + `tool_choice: "auto"` for reliable tool calling.

## Langfuse Dashboard

After deploying, access Langfuse at the URL shown in Modal logs:

```bash
modal app logs goat-services
```

Default credentials (set in `goat-langfuse` secret):
- Email: `admin@goat.local`
- Password: (shown during setup)

## Langfuse Trace Verification

### Verify traces are landing

```bash
# Fire a test call and verify traces via Langfuse REST API
uv run --with modal,requests python scripts/verify_langfuse_traces.py

# Use a different model
uv run --with modal,requests python scripts/verify_langfuse_traces.py --model llama-3.1-8b-local

# Skip test call, just query existing traces
uv run --with modal,requests python scripts/verify_langfuse_traces.py --query-only

# Also show raw Postgres SQL output
uv run --with modal,requests python scripts/verify_langfuse_traces.py --sql
```

### Sync traces to local Langfuse

Pull all trace data from the Modal volume into local docker-compose for debugging:

```bash
# Full sync: dump from Modal, restore into local Postgres, restart Langfuse
bash scripts/langfuse_sync_local.sh

# Just download the dump file
bash scripts/langfuse_sync_local.sh --dump-only

# Sync and open the browser
bash scripts/langfuse_sync_local.sh --open
```

After syncing, view traces at http://localhost:3001 (login: `admin@goat.local` / `admin1234`).

### Query Postgres directly

The `langfuse_query()` Modal function runs arbitrary SQL against the volume-backed Postgres:

```python
import modal
fn = modal.Function.from_name("goat-services", "langfuse_query", environment_name="goat")

# Run SQL
print(fn.remote(sql="SELECT count(*) FROM traces;"))

# Export full database
dump = fn.remote(dump=True)
```

## Troubleshooting

**"Secret not found" errors**: Run `python scripts/modal_setup.py` to create secrets.

**LiteLLM not discoverable**: Make sure services are deployed: `modal deploy scripts/modal_services.py`

**vLLM cold start slow**: First request after idle takes ~60-90s to load model weights. Subsequent requests are fast.

**Langfuse traces missing**: Run `uv run --with modal,requests python scripts/verify_langfuse_traces.py --sql` to check if traces are landing. If the Postgres volume is empty, redeploy services: `modal deploy scripts/modal_services.py --env goat`.

**Binary not found**: Run `bash scripts/build_eval.sh` to build the eval binary.
