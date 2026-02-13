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
                                 ┌──────────┐ ┌────────────────┐
                                 │ Langfuse │ │ vLLM (GPU:A10G)│
                                 │ (traces) │ │ Llama-3.1-8B   │
                                 └────┬─────┘ └────────────────┘
                                      ▼
                                 ┌──────────┐
                                 │ Postgres  │
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
modal deploy scripts/modal_services.py

# (Optional) Deploy vLLM local model server
modal deploy scripts/modal_vllm.py
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
| `modal_vllm.py` | Deploy vLLM GPU model server (Llama-3.1-8B on A10G) |
| `modal_sandbox.py` | Run goat-eval in isolated per-task sandboxes |
| `modal_results.py` | View benchmark results from Modal volume |
| `build_eval.sh` | Cross-compile goat-eval binary for Linux |

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
| `goat-pgdata` | Postgres data persistence |
| `goat-results` | Benchmark results |
| `goat-model-cache` | HuggingFace model weights for vLLM |

## Models Available Through LiteLLM

| Model Name | Provider | Notes |
|------------|----------|-------|
| `gpt-5-nano` | Azure OpenAI | Cheapest, good for testing |
| `gpt-5-mini` | Azure OpenAI | Better quality |
| `gpt-4o-mini` | Azure OpenAI | Previous gen |
| `llama-3.3-70b` | Groq | Fast open-weight |
| `llama-3.1-8b-local` | vLLM on Modal GPU | Requires modal_vllm.py deployed |

## Langfuse Dashboard

After deploying, access Langfuse at the URL shown in Modal logs:

```bash
modal app logs goat-services
```

Default credentials (set in `goat-langfuse` secret):
- Email: `admin@goat.local`
- Password: (shown during setup)

## Troubleshooting

**"Secret not found" errors**: Run `python scripts/modal_setup.py` to create secrets.

**LiteLLM not discoverable**: Make sure services are deployed: `modal deploy scripts/modal_services.py`

**vLLM cold start slow**: First request after idle takes ~60-90s to load model weights. Subsequent requests are fast.

**Binary not found**: Run `bash scripts/build_eval.sh` to build the eval binary.
