"""vLLM inference server on Modal GPU for Goat benchmarks.

Serves an OpenAI-compatible API for local model inference.
LiteLLM proxy routes requests here for local model names.

Deploy (select model via VLLM_MODEL env var):
    VLLM_MODEL=llama-3.1-8b modal deploy scripts/modal_vllm.py --env goat
    VLLM_MODEL=qwen3-4b    modal deploy scripts/modal_vllm.py --env goat
    VLLM_MODEL=qwen3-235b  modal deploy scripts/modal_vllm.py --env goat

Available models:
    llama-3.1-8b    -- meta-llama/Llama-3.1-8B-Instruct  (A10G)
    qwen3-4b        -- Qwen/Qwen3-4B-Instruct-2507       (A10G)
    qwen3-30b-a3b   -- Qwen/Qwen3-30B-A3B-Instruct-2507  (A10G)
    qwen3-235b      -- Qwen/Qwen3-235B-A22B-...-FP8      (A100-80GB)

Test:
    curl https://<workspace>--goat-vllm-<model-key>-serve.modal.run/v1/chat/completions \\
      -H "Content-Type: application/json" \\
      -d '{"model": "<hf-model-id>", "messages": [{"role": "user", "content": "Hello"}]}'

Scale to zero when idle (no GPU cost). Wakes on first request (~60s cold start).

Adapted from: https://github.com/modal-labs/modal-examples/blob/main/06_gpu_and_ml/llm-serving/gpt_oss_inference.py
"""

import os
import subprocess
import sys

import modal

MODAL_ENVIRONMENT = "goat"
MINUTES = 60

# ---------------------------------------------------------------------------
# Model registry (inline -- Modal re-imports this module inside the container,
# so we can't rely on importing a sibling file)
# ---------------------------------------------------------------------------
VLLM_MODELS = {
    "llama-3.1-8b": {
        "hf_model": "meta-llama/Llama-3.1-8B-Instruct",
        "gpu": "A10G",
        "n_gpu": 1,
        "max_model_len": 8192,
        "needs_hf_token": True,
    },
    "qwen3-4b": {
        "hf_model": "Qwen/Qwen3-4B-Instruct-2507",
        "gpu": "A10G",
        "n_gpu": 1,
        "max_model_len": 8192,
    },
    "qwen3-30b-a3b": {
        "hf_model": "Qwen/Qwen3-30B-A3B-Instruct-2507",
        "gpu": "A10G",
        "n_gpu": 1,
        "max_model_len": 8192,
    },
    "qwen3-235b": {
        "hf_model": "Qwen/Qwen3-235B-A22B-Instruct-2507-FP8",
        "gpu": "A100-80GB",
        "n_gpu": 1,
        "max_model_len": 8192,
    },
}

# ---------------------------------------------------------------------------
# Model selection
# ---------------------------------------------------------------------------
MODEL_KEY = os.environ.get("VLLM_MODEL", "llama-3.1-8b")

if MODEL_KEY not in VLLM_MODELS:
    print(f"ERROR: Unknown model key '{MODEL_KEY}'.")
    print(f"Available models: {', '.join(VLLM_MODELS.keys())}")
    sys.exit(1)

_cfg = VLLM_MODELS[MODEL_KEY]
MODEL_NAME = _cfg["hf_model"]
GPU_TYPE = _cfg["gpu"]
N_GPU = _cfg["n_gpu"]
MAX_MODEL_LEN = _cfg["max_model_len"]
VLLM_PORT = 8000

# ---------------------------------------------------------------------------
# App and resources -- dynamic app name per model
# ---------------------------------------------------------------------------
app = modal.App(f"goat-vllm-{MODEL_KEY}")

hf_cache_vol = modal.Volume.from_name(
    "goat-model-cache", create_if_missing=True, environment_name=MODAL_ENVIRONMENT
)
vllm_cache_vol = modal.Volume.from_name(
    "goat-vllm-cache", create_if_missing=True, environment_name=MODAL_ENVIRONMENT
)

# ---------------------------------------------------------------------------
# Image
# ---------------------------------------------------------------------------
vllm_image = (
    modal.Image.debian_slim(python_version="3.12")
    .pip_install(
        "vllm==0.8.5.post1",
        "huggingface_hub[hf_transfer]",
        "torch",
        "transformers>=4.45,<5.0",  # vllm needs transformers 4.x
    )
    .env({
        "HF_HUB_ENABLE_HF_TRANSFER": "1",
        # Bake model key into image so the module re-import in the container
        # resolves the same model. Secrets are too late (only available at
        # function execution, not module import).
        "VLLM_MODEL": MODEL_KEY,
    })
)

# ---------------------------------------------------------------------------
# HuggingFace secret -- always included so the dep count is stable across
# models. Harmless for non-gated models.
# Create with: modal secret create goat-hf-token HF_TOKEN=hf_xxx --env goat
# ---------------------------------------------------------------------------
hf_secret = modal.Secret.from_name("goat-hf-token", environment_name=MODAL_ENVIRONMENT)


# ---------------------------------------------------------------------------
# vLLM server function
# ---------------------------------------------------------------------------
@app.function(
    image=vllm_image,
    gpu=f"{GPU_TYPE}:{N_GPU}",
    secrets=[hf_secret],
    volumes={
        "/root/.cache/huggingface": hf_cache_vol,
        "/root/.cache/vllm": vllm_cache_vol,
    },
    timeout=60 * MINUTES,
    scaledown_window=5 * MINUTES,  # GPU shuts down after 5 min idle
)
@modal.concurrent(max_inputs=32)
@modal.web_server(port=VLLM_PORT, startup_timeout=15 * MINUTES)
def serve():
    """Start vLLM OpenAI-compatible server for the configured model."""
    cmd = [
        "python", "-m", "vllm.entrypoints.openai.api_server",
        "--model", MODEL_NAME,
        "--served-model-name", MODEL_NAME,
        "--host", "0.0.0.0",
        "--port", str(VLLM_PORT),
        "--tensor-parallel-size", str(N_GPU),
        "--max-model-len", str(MAX_MODEL_LEN),
        "--enforce-eager",  # faster startup, slightly slower inference
    ]

    print(f"Starting vLLM: {MODEL_NAME} on {GPU_TYPE} (app: goat-vllm-{MODEL_KEY})")
    print(f"Command: {' '.join(cmd)}")

    subprocess.Popen(cmd)


# ---------------------------------------------------------------------------
# Health check / local test
# ---------------------------------------------------------------------------
@app.local_entrypoint()
def test():
    """Quick health check: print the vLLM server URL."""
    url = serve.web_url
    litellm_name = f"{MODEL_KEY}-local"

    print(f"vLLM server URL: {url}")
    print(f"Model: {MODEL_NAME}")
    print(f"Model key: {MODEL_KEY}")
    print(f"GPU: {GPU_TYPE}")
    print()
    print("Test with:")
    print(f'  curl {url}/v1/models')
    print()
    print(f'  curl {url}/v1/chat/completions \\')
    print(f'    -H "Content-Type: application/json" \\')
    print(f'    -d \'{{"model": "{MODEL_NAME}", "messages": [{{"role": "user", "content": "Hello"}}]}}\'')
    print()
    print("Add to LiteLLM config (dev/litellm-config.yaml):")
    print(f"  - model_name: {litellm_name}")
    print(f"    litellm_params:")
    print(f"      model: openai/{MODEL_NAME}")
    print(f"      api_base: {url}/v1")
    print(f'      api_key: "not-needed"')
