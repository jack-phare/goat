# FEAT: Expand vLLM Model Selection

**Status**: done
**Priority**: low
**Component**: `scripts/modal_vllm.py`, `scripts/modal_gpt_oss.py`, `dev/litellm-config-modal.yaml`

## Description

Currently only `meta-llama/Llama-3.1-8B-Instruct` is deployed on vLLM. Expand to support multiple models for richer benchmarking.

### Standard vLLM models (`scripts/modal_vllm.py`)

- `meta-llama/Llama-3.1-8B-Instruct` (A10G) -- existing
- `Qwen/Qwen3-4B-Instruct-2507` (A10G)
- `Qwen/Qwen3-30B-A3B-Instruct-2507` (A10G, MoE)
- `Qwen/Qwen3-235B-A22B-Instruct-2507-FP8` (A100-80GB, MoE)

### GPT OSS models (`scripts/modal_gpt_oss.py`)

- `openai/gpt-oss-20b` (H100, MoE mxfp4)
- `openai/gpt-oss-120b` (H100, MoE mxfp4)

References:
- Qwen: https://modal.com/docs/examples/vllm_inference
- GPT OSS: https://github.com/modal-labs/modal-examples/blob/main/06_gpu_and_ml/llm-serving/gpt_oss_inference.py

## Approach

- **Model registry** (`scripts/model_registry.py`): central Python dict mapping short keys to HF model IDs, GPU types, and vLLM settings.
- **Parameterized vLLM script**: `VLLM_MODEL=<key> modal deploy scripts/modal_vllm.py` creates a uniquely-named Modal app per model (e.g. `goat-vllm-qwen3-4b`). Multiple models can be live simultaneously.
- **Dedicated GPT OSS script**: `scripts/modal_gpt_oss.py` handles the different requirements (CUDA 12.8, vLLM 0.13.0, speculative decoding, flashinfer).
- **LiteLLM routing**: each model gets a `<key>-local` entry in `litellm-config-modal.yaml`.
- **Go code**: `IsLocalModel()` in `pkg/llm/translate.go` detects `-local` suffix and applies safe defaults (lower temperature, explicit `tool_choice: "auto"`).

## Considerations

- GPU cost: A10G ($1.10/hr), A100 ($3.30/hr), H100 (~$4.50/hr) -- only spin up when benchmarking
- `scaledown_window=5*MINUTES` already handles cost control
- Model weights cached on Modal Volumes (first download is slow, subsequent are fast)
- GPT OSS needs different vLLM version (0.13.0) and CUDA base image (12.8.1)

## Verified

Smoke benchmark (5 tasks) passed on all three deployed models:
- `qwen3-4b-local`: 5/5 passed (74s)
- `llama-3.1-8b-local`: 5/5 passed (90s)
- `gpt-oss-20b-local`: 5/5 passed (241s)

Remaining models (`qwen3-30b-a3b`, `qwen3-235b`, `gpt-oss-120b`) have code ready but
are not deployed yet -- deploy on demand with the parameterized scripts.

## Lessons Learned

- Modal re-imports the script module in the container; env vars must be baked into the
  image via `.env()`, not via `modal.Secret` (which is only available at function execution).
- Modal converts dots in app names to hyphens in URLs (e.g. `llama-3.1-8b` becomes `llama-3-1-8b` in the URL slug).
- HF token secret must always be included (not conditional) to avoid dep count mismatch between local and container environments.

## Files

- `scripts/model_registry.py` -- reference model config doc (NEW)
- `scripts/modal_vllm.py` -- parameterized vLLM server with inlined registry (MODIFIED)
- `scripts/modal_gpt_oss.py` -- GPT OSS server with CUDA 12.8 + vLLM 0.13.0 (NEW)
- `dev/litellm-config-modal.yaml` -- model routing config (MODIFIED, +5 entries)
- `dev/litellm-config.yaml` -- local dev config (MODIFIED, +5 entries commented out)
- `pkg/llm/translate.go` -- `IsLocalModel()` (MODIFIED)
- `pkg/llm/request.go` -- local model tuning (MODIFIED)
- `scripts/run_cross_model_modal.sh` -- benchmark model list (MODIFIED)
