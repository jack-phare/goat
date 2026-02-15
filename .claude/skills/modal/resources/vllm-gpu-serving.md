# vLLM GPU Model Serving on Modal

Self-host open-weight LLMs on Modal GPUs. Each model gets its own Modal app that scales to zero when idle.

## Two Deploy Scripts

| Script | Models | vLLM | Base Image | GPU |
|--------|--------|------|------------|-----|
| `scripts/modal_vllm.py` | Llama, Qwen | 0.8.5 | `debian_slim` | A10G / A100 |
| `scripts/modal_gpt_oss.py` | GPT OSS 20B/120B | 0.13.0 | `nvidia/cuda:12.8.1` | H100 |

GPT OSS uses a separate script because it needs CUDA 12.8, a newer vLLM, and EAGLE3 speculative decoding.

## Deploy Commands

```bash
# Standard models
VLLM_MODEL=llama-3.1-8b   modal deploy scripts/modal_vllm.py --env goat
VLLM_MODEL=qwen3-4b       modal deploy scripts/modal_vllm.py --env goat
VLLM_MODEL=qwen3-30b-a3b  modal deploy scripts/modal_vllm.py --env goat
VLLM_MODEL=qwen3-235b     modal deploy scripts/modal_vllm.py --env goat

# GPT OSS models
GPT_OSS_MODEL=gpt-oss-20b   modal deploy scripts/modal_gpt_oss.py --env goat
GPT_OSS_MODEL=gpt-oss-120b  modal deploy scripts/modal_gpt_oss.py --env goat

# HuggingFace token (needed for gated models like Llama)
modal secret create goat-hf-token HF_TOKEN=hf_xxx --env goat
```

## Model Registry

| Key | HF Model ID | GPU | Cost/hr | Deploy Script |
|-----|-------------|-----|---------|---------------|
| `llama-3.1-8b` | `meta-llama/Llama-3.1-8B-Instruct` | A10G | ~$1.10 | `modal_vllm.py` |
| `qwen3-4b` | `Qwen/Qwen3-4B-Instruct-2507` | A10G | ~$1.10 | `modal_vllm.py` |
| `qwen3-30b-a3b` | `Qwen/Qwen3-30B-A3B-Instruct-2507` | A10G | ~$1.10 | `modal_vllm.py` |
| `qwen3-235b` | `Qwen/Qwen3-235B-A22B-Instruct-2507-FP8` | A100-80GB | ~$3.30 | `modal_vllm.py` |
| `gpt-oss-20b` | `openai/gpt-oss-20b` | H100 | ~$4.50 | `modal_gpt_oss.py` |
| `gpt-oss-120b` | `openai/gpt-oss-120b` | H100 | ~$4.50 | `modal_gpt_oss.py` |

All apps scale to zero after 5 min idle. Cold start: ~60-90s standard, ~2-5 min GPT OSS.

## GPU Tiers

| GPU | VRAM | Best For | Cost/hr |
|-----|------|----------|---------|
| A10G | 24GB | 4B-30B models | ~$1.10 |
| A100-80GB | 80GB | 70B+ models, FP8 | ~$3.30 |
| H100 | 80GB | 70B+ fastest, MoE | ~$4.50 |

## vLLM Configuration

### Standard Models (`modal_vllm.py`)

- Image: `debian_slim` + `vllm==0.8.5.post1` + `transformers>=4.45,<5.0`
- `--enforce-eager` — faster startup, slightly slower inference
- `--max-model-len 8192`
- 32 max concurrent inputs per replica
- `scaledown_window=5*60`

### GPT OSS Models (`modal_gpt_oss.py`)

- Image: `nvidia/cuda:12.8.1-devel-ubuntu22.04` + `vllm==0.13.0`
- EAGLE3 speculative decoding (7 speculative tokens, `RedHatAI/gpt-oss-20b-speculator.eagle3`)
- FP8 KV cache (`--kv-cache-dtype fp8`)
- FlashInfer MoE kernels (`VLLM_USE_FLASHINFER_MOE_MXFP4_MXFP8=1`)
- Compilation: `fuse_allreduce_rms`, `eliminate_noops`
- `--async-scheduling` for reduced host overhead
- `--max-model-len 32768`
- Set `FAST_BOOT = True` in script to disable compilation for faster dev iteration

## Wiring to LiteLLM

After deploying, add an entry in `dev/litellm-config-modal.yaml`:

```yaml
- model_name: llama-3.1-8b-local
  litellm_params:
    model: openai/meta-llama/Llama-3.1-8B-Instruct
    api_base: https://phare-goat--goat-vllm-llama-3-1-8b-serve.modal.run/v1
    api_key: "not-needed"
```

URL pattern: `https://<workspace>--goat-vllm-<model-key>-serve.modal.run/v1`

The `-local` suffix convention tells the Go code (`pkg/llm/translate.go`) to apply `temperature: 0.3` + `tool_choice: "auto"` for reliable tool calling.

## Testing a Model

```bash
# List models
curl https://<workspace>--goat-vllm-llama-3-1-8b-serve.modal.run/v1/models

# Chat completion
curl https://<workspace>--goat-vllm-llama-3-1-8b-serve.modal.run/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "meta-llama/Llama-3.1-8B-Instruct", "messages": [{"role": "user", "content": "Hello"}]}'
```

## Adding a New Model

1. Add entry to `VLLM_MODELS` in `scripts/modal_vllm.py`
2. Deploy: `VLLM_MODEL=<key> modal deploy scripts/modal_vllm.py --env goat`
3. Note URL from deploy output
4. Add entry to `dev/litellm-config-modal.yaml` with `<key>-local` name
5. (Optional) Add commented entry to `dev/litellm-config.yaml`

## Shared Volumes

| Volume | Mount Path | Purpose |
|--------|-----------|---------|
| `goat-model-cache` | `/root/.cache/huggingface` | HuggingFace weights |
| `goat-vllm-cache` | `/root/.cache/vllm` | Compilation cache |
| `goat-flashinfer-cache` | `/root/.cache/flashinfer` | MoE kernels (GPT OSS) |
