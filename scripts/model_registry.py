"""Central model registry for Modal vLLM deployments.

Reference file documenting all available models and their configs.

NOTE: modal_vllm.py and modal_gpt_oss.py each inline their own copy of
the relevant registry dict. Modal re-imports the script module inside the
container, so sibling file imports don't work. If you add a model here,
also add it to the corresponding deploy script.
"""

# ---------------------------------------------------------------------------
# Standard vLLM models (Llama, Qwen, etc.)
# Deployed via: VLLM_MODEL=<key> modal deploy scripts/modal_vllm.py --env goat
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
# GPT OSS models (require different vLLM version, CUDA image, and GPU)
# Deployed via: GPT_OSS_MODEL=<key> modal deploy scripts/modal_gpt_oss.py --env goat
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
# LiteLLM model name convention: <short-key>-local
# e.g. "qwen3-4b" -> "qwen3-4b-local" in litellm-config-modal.yaml
# ---------------------------------------------------------------------------
LITELLM_SUFFIX = "-local"


def litellm_name(key: str) -> str:
    """Return the LiteLLM model_name for a registry key."""
    return f"{key}{LITELLM_SUFFIX}"
