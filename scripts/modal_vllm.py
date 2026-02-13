"""vLLM inference server on Modal GPU for Goat benchmarks.

Serves an OpenAI-compatible API for local model inference.
LiteLLM proxy routes requests here for local model names.

Deploy:
    modal deploy scripts/modal_vllm.py

Test:
    curl https://<workspace>--goat-vllm-serve.modal.run/v1/chat/completions \\
      -H "Content-Type: application/json" \\
      -d '{"model": "meta-llama/Llama-3.1-8B-Instruct", "messages": [{"role": "user", "content": "Hello"}]}'

Scale to zero when idle (no GPU cost). Wakes on first request (~60s cold start).

Adapted from: https://github.com/modal-labs/modal-examples/blob/main/06_gpu_and_ml/llm-serving/gpt_oss_inference.py
"""

import subprocess

import modal

MODAL_ENVIRONMENT = "goat"
MINUTES = 60

# ---------------------------------------------------------------------------
# Model configuration
# ---------------------------------------------------------------------------
# Start with a small model to validate the pipeline.
# Llama-3.1-8B-Instruct fits comfortably on A10G (24GB VRAM).
MODEL_NAME = "meta-llama/Llama-3.1-8B-Instruct"
GPU_TYPE = "A10G"
N_GPU = 1
VLLM_PORT = 8000

# ---------------------------------------------------------------------------
# App and resources
# ---------------------------------------------------------------------------
app = modal.App("goat-vllm")

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
    .env({"HF_HUB_ENABLE_HF_TRANSFER": "1"})
)

# ---------------------------------------------------------------------------
# HuggingFace secret (for gated models like Llama)
# Required for gated models. Create with:
#   modal secret create goat-hf-token HF_TOKEN=hf_xxx --env goat
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
        "--max-model-len", "8192",
        "--enforce-eager",  # faster startup, slightly slower inference
    ]

    print(f"Starting vLLM: {MODEL_NAME} on {GPU_TYPE}")
    print(f"Command: {' '.join(cmd)}")

    subprocess.Popen(cmd)


# ---------------------------------------------------------------------------
# Health check / local test
# ---------------------------------------------------------------------------
@app.local_entrypoint()
def test():
    """Quick health check: print the vLLM server URL."""
    url = serve.web_url
    print(f"vLLM server URL: {url}")
    print(f"Model: {MODEL_NAME}")
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
    print(f"  - model_name: llama-3.1-8b-local")
    print(f"    litellm_params:")
    print(f"      model: openai/{MODEL_NAME}")
    print(f"      api_base: {url}/v1")
    print(f'      api_key: "not-needed"')
