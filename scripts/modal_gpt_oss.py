"""GPT OSS inference server on Modal GPU for Goat benchmarks.

Serves OpenAI's gpt-oss models (20B / 120B) via vLLM with speculative
decoding. Requires H100 GPU and vLLM 0.13.0+ with CUDA 12.8.

These MoE models use mxfp4 quantization and benefit from flashinfer
MoE kernels. The speculative decoding config uses an EAGLE3-based
speculator for faster inference.

Deploy (select model via GPT_OSS_MODEL env var):
    GPT_OSS_MODEL=gpt-oss-20b  modal deploy scripts/modal_gpt_oss.py --env goat
    GPT_OSS_MODEL=gpt-oss-120b modal deploy scripts/modal_gpt_oss.py --env goat

Available models:
    gpt-oss-20b  -- openai/gpt-oss-20b   (H100)
    gpt-oss-120b -- openai/gpt-oss-120b  (H100)

Scale to zero when idle (no GPU cost). Wakes on first request (~2-5 min cold start).

Adapted from: https://github.com/modal-labs/modal-examples/blob/main/06_gpu_and_ml/llm-serving/gpt_oss_inference.py
"""

import json
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
GPT_OSS_MODELS = {
    "gpt-oss-20b": {
        "hf_model": "openai/gpt-oss-20b",
        "revision": "d666cf3b67006cf8227666739edf25164aaffdeb",
        "gpu": "H100",
        "n_gpu": 1,
        "max_model_len": 32768,
    },
    "gpt-oss-120b": {
        "hf_model": "openai/gpt-oss-120b",
        "gpu": "H100",
        "n_gpu": 1,
        "max_model_len": 32768,
    },
}

# ---------------------------------------------------------------------------
# Model selection
# ---------------------------------------------------------------------------
MODEL_KEY = os.environ.get("GPT_OSS_MODEL", "gpt-oss-20b")

if MODEL_KEY not in GPT_OSS_MODELS:
    print(f"ERROR: Unknown GPT OSS model key '{MODEL_KEY}'.")
    print(f"Available models: {', '.join(GPT_OSS_MODELS.keys())}")
    sys.exit(1)

_cfg = GPT_OSS_MODELS[MODEL_KEY]
MODEL_NAME = _cfg["hf_model"]
MODEL_REVISION = _cfg.get("revision")
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
flashinfer_cache_vol = modal.Volume.from_name(
    "goat-flashinfer-cache", create_if_missing=True, environment_name=MODAL_ENVIRONMENT
)

# ---------------------------------------------------------------------------
# Image -- CUDA 12.8 base for Blackwell/Hopper GPU support
# ---------------------------------------------------------------------------
vllm_image = (
    modal.Image.from_registry(
        "nvidia/cuda:12.8.1-devel-ubuntu22.04",
        add_python="3.12",
    )
    .entrypoint([])
    .uv_pip_install(
        "vllm==0.13.0",
        "huggingface_hub[hf_transfer]==0.36.0",
    )
    .env({
        "HF_HUB_ENABLE_HF_TRANSFER": "1",
        "VLLM_USE_FLASHINFER_MOE_MXFP4_MXFP8": "1",  # fast MoE kernels
        # Bake model key into image so the module re-import in the container
        # resolves the same model.
        "GPT_OSS_MODEL": MODEL_KEY,
    })
)

# ---------------------------------------------------------------------------
# Speculative decoding config (EAGLE3-based speculator)
# Generates multiple tokens per forward pass for faster inference.
# ---------------------------------------------------------------------------
SPECULATIVE_CONFIG = {
    "model": "RedHatAI/gpt-oss-20b-speculator.eagle3",
    "num_speculative_tokens": 7,
    "method": "eagle3",
}

# ---------------------------------------------------------------------------
# vLLM engine configuration
# ---------------------------------------------------------------------------
VLLM_CONFIG = {
    "stream-interval": 20,        # return tokens in chunks of 20
    "kv-cache-dtype": "fp8",      # quantize KV cache for memory savings
    "max-num-batched-tokens": 16384,
    "max-model-len": MAX_MODEL_LEN,
}

# Compilation settings for inference performance (slower startup)
COMPILATION_CONFIG = {
    "pass_config": {"fuse_allreduce_rms": True, "eliminate_noops": True}
}

MAX_INPUTS = 32  # max concurrent requests per replica
VLLM_CONFIG["max-cudagraph-capture-size"] = MAX_INPUTS

# Set to True to disable compilation for faster startup during development
FAST_BOOT = False


# ---------------------------------------------------------------------------
# vLLM server function
# ---------------------------------------------------------------------------
@app.function(
    image=vllm_image,
    gpu=f"{GPU_TYPE}:{N_GPU}",
    volumes={
        "/root/.cache/huggingface": hf_cache_vol,
        "/root/.cache/vllm": vllm_cache_vol,
        "/root/.cache/flashinfer": flashinfer_cache_vol,
    },
    timeout=60 * MINUTES,
    scaledown_window=5 * MINUTES,  # GPU shuts down after 5 min idle
)
@modal.concurrent(max_inputs=MAX_INPUTS)
@modal.web_server(port=VLLM_PORT, startup_timeout=30 * MINUTES)
def serve():
    """Start vLLM OpenAI-compatible server for GPT OSS model."""
    cmd = [
        "vllm", "serve", MODEL_NAME,
        "--served-model-name", MODEL_NAME,
        "--host", "0.0.0.0",
        "--port", str(VLLM_PORT),
        "--async-scheduling",  # reduces host overhead
    ]

    if MODEL_REVISION:
        cmd += ["--revision", MODEL_REVISION]

    # enforce-eager disables compilation + CUDA graph capture (faster startup)
    cmd += ["--enforce-eager" if FAST_BOOT else "--no-enforce-eager"]

    cmd += ["--tensor-parallel-size", str(N_GPU)]
    cmd += ["--compilation-config", json.dumps(COMPILATION_CONFIG)]
    cmd += ["--speculative-config", json.dumps(SPECULATIVE_CONFIG)]

    # Add remaining config flags
    cmd += [
        item for k, v in VLLM_CONFIG.items() for item in (f"--{k}", str(v))
    ]

    print(f"Starting vLLM (GPT OSS): {MODEL_NAME} on {GPU_TYPE} (app: goat-vllm-{MODEL_KEY})")
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
    print("Add to LiteLLM config (dev/litellm-config-modal.yaml):")
    print(f"  - model_name: {litellm_name}")
    print(f"    litellm_params:")
    print(f"      model: openai/{MODEL_NAME}")
    print(f"      api_base: {url}/v1")
    print(f'      api_key: "not-needed"')
