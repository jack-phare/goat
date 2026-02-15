---
name: modal
description: Modal cloud infrastructure for Goat. Use whenever working with Modal deployments, GPU model serving, LiteLLM proxy routing, Langfuse observability, eval sandboxes, or any cloud infrastructure task. Read the relevant resource file for the specific component you need.
---

# Modal Infrastructure

Goat runs its cloud infrastructure on [Modal](https://modal.com) — LLM proxy routing, GPU inference, observability, and eval sandboxes. This skill covers the full stack.

## Architecture

```
Your Machine                     Modal Cloud (goat environment)
─────────────                    ─────────────────────────────────
                                 ┌─────────────────────────────┐
modal_sandbox.py ──Modal SDK──▶  │  Sandbox (per task, CPU)    │
                                 │  goat-eval binary           │
                                 └─────────┬──────────────────┘
                                           │ OPENAI_BASE_URL
                                           ▼
                                 ┌─────────────────────────────┐
                                 │  LiteLLM Proxy (port 4000)  │
                                 │  Routes to cloud + GPU      │
                                 └────┬──────────┬────────────┘
                                      │          │
                                      ▼          ▼
                                 ┌──────────┐ ┌────────────────────────┐
                                 │ Langfuse │ │ vLLM (per-model apps)  │
                                 │ (traces) │ │ scale-to-zero GPUs     │
                                 └────┬─────┘ └────────────────────────┘
                                      ▼
                                 ┌──────────┐
                                 │ Postgres  │
                                 └──────────┘
```

## Quick Start

```bash
# 1. Install Modal
uv tool install modal
modal setup

# 2. Provision environment + secrets (reads from .env)
python scripts/modal_setup.py            # interactive
python scripts/modal_setup.py --dry-run  # preview only

# 3. Deploy core services (LiteLLM + Langfuse + Postgres)
modal deploy scripts/modal_services.py --env goat

# 4. (Optional) Deploy GPU models
VLLM_MODEL=llama-3.1-8b modal deploy scripts/modal_vllm.py --env goat

# 5. Build eval binary + run a benchmark
bash scripts/build_eval.sh
python scripts/modal_sandbox.py --prompt "Write fizzbuzz" --model gpt-5-nano
```

## Resources

Each component has a dedicated reference file. Read the one you need:

| Resource | When to Read |
|----------|-------------|
| [resources/vllm-gpu-serving.md](resources/vllm-gpu-serving.md) | Deploying models on GPUs, vLLM config, adding new models, GPU tiers and costs |
| [resources/litellm-routing.md](resources/litellm-routing.md) | LiteLLM proxy config, model routing, provider setup, local Docker dev stack |
| [resources/langfuse.md](resources/langfuse.md) | Langfuse tracing, verification, syncing traces, querying Postgres |
| [resources/sandboxes.md](resources/sandboxes.md) | Eval sandbox execution, batch mode, A/B comparisons, results |

## Secrets

Created by `scripts/modal_setup.py` in the `goat` environment:

| Secret | Contents |
|--------|----------|
| `goat-llm-providers` | Azure, Groq, OpenAI, Anthropic API keys |
| `goat-litellm` | LiteLLM proxy master key |
| `goat-langfuse` | Langfuse auto-provision config |
| `goat-postgres` | Postgres credentials |
| `goat-hf-token` | HuggingFace token (for gated models) |

## Volumes

| Volume | Purpose |
|--------|---------|
| `goat-langfuse-pg` | Langfuse Postgres persistence |
| `goat-results` | Benchmark results |
| `goat-model-cache` | HuggingFace model weights |
| `goat-vllm-cache` | vLLM compilation cache |
| `goat-flashinfer-cache` | FlashInfer MoE kernels (GPT OSS) |

## Key Scripts

| Script | Purpose |
|--------|---------|
| `scripts/modal_setup.py` | Interactive environment + secrets creation |
| `scripts/modal_services.py` | Deploy LiteLLM + Langfuse + Postgres |
| `scripts/modal_vllm.py` | Deploy vLLM server (`VLLM_MODEL` env var) |
| `scripts/modal_gpt_oss.py` | Deploy GPT OSS server (`GPT_OSS_MODEL` env var) |
| `scripts/modal_sandbox.py` | Run goat-eval in isolated sandboxes |
| `scripts/modal_results.py` | View benchmark results |
| `scripts/build_eval.sh` | Cross-compile eval binary for Linux |
| `scripts/test_modal_infra.sh` | 6-phase infrastructure validation |
| `scripts/run_cross_model_modal.sh` | Cross-model benchmark runner |
| `scripts/verify_langfuse_traces.py` | Verify Langfuse traces |
| `scripts/langfuse_sync_local.sh` | Sync Modal traces to local |
| `scripts/README.md` | Full infrastructure docs |

## Troubleshooting

| Problem | Fix |
|---------|-----|
| "Secret not found" | `python scripts/modal_setup.py` |
| Services not running | `modal deploy scripts/modal_services.py --env goat` |
| Need service URLs | `modal app logs goat-services` |
| List running apps | `modal app list --env goat` |
| Eval binary missing | `bash scripts/build_eval.sh` |
