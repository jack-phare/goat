# LiteLLM Model Routing

LiteLLM is the unified proxy that routes all LLM requests. The Go binary speaks OpenAI chat completions to one endpoint — LiteLLM handles routing to Azure, Groq, OpenAI, Anthropic, or self-hosted vLLM.

## The Go Binary Only Needs 3 Env Vars

```
OPENAI_BASE_URL  →  LiteLLM proxy URL
OPENAI_API_KEY   →  proxy auth key
EVAL_MODEL       →  model name from config
```

## Two Config Files

| File | Deployment | Difference |
|------|-----------|------------|
| `dev/litellm-config.yaml` | Local Docker | Stateful (`database_url`), vLLM entries commented out |
| `dev/litellm-config-modal.yaml` | Modal cloud | Stateless (no database), vLLM entries active |

## Config Format

```yaml
model_list:
  # Cloud API
  - model_name: gpt-5-nano
    litellm_params:
      model: azure/gpt-5-nano
      api_key: os.environ/AZURE_API_KEY
      api_base: os.environ/AZURE_API_BASE
      api_version: "2024-06-01"
    model_info:
      base_model: gpt-4o-mini    # cost tracking fallback

  # Self-hosted vLLM
  - model_name: llama-3.1-8b-local
    litellm_params:
      model: openai/meta-llama/Llama-3.1-8B-Instruct
      api_base: https://phare-goat--goat-vllm-llama-3-1-8b-serve.modal.run/v1
      api_key: "not-needed"
```

Provider prefixes: `azure/`, `groq/`, `openai/`, `anthropic/`

## Available Models

### Cloud

| Name | Provider | Prefix |
|------|----------|--------|
| `gpt-4o-mini` | Azure OpenAI | `azure/gpt-4o-mini` |
| `gpt-5-nano` | Azure OpenAI | `azure/gpt-5-nano` |
| `gpt-5-mini` | Azure OpenAI | `azure/gpt-5-mini` |
| `llama-3.3-70b` | Groq | `groq/llama-3.3-70b-versatile` |
| `text-embedding-3-small` | OpenAI | `openai/text-embedding-3-small` |

### Local GPU (require vLLM deployment)

| Name | Backend |
|------|---------|
| `llama-3.1-8b-local` | vLLM on A10G |
| `qwen3-4b-local` | vLLM on A10G |
| `qwen3-30b-a3b-local` | vLLM on A10G |
| `qwen3-235b-local` | vLLM on A100 |
| `gpt-oss-20b-local` | vLLM on H100 |
| `gpt-oss-120b-local` | vLLM on H100 |

## Local Dev Stack (Docker Compose)

```bash
# Start (reads provider keys from root .env)
docker compose --env-file .env -f dev/docker-compose.yml up -d

# Verify
curl http://localhost:4000/v1/models -H "Authorization: Bearer sk-dev-key"

# Run example
LITELLM_MASTER_KEY=sk-dev-key go run ./cmd/example/ -provider litellm

# Tear down
docker compose -f dev/docker-compose.yml down
```

Services: Postgres (5432), Langfuse (3001), LiteLLM (4000).

### Required .env (template at `dev/.env.example`)

```bash
LITELLM_MASTER_KEY=sk-dev-key
AZURE_API_KEY=         # At least ONE provider needed
AZURE_API_BASE=
GROQ_API_KEY=
OPENAI_API_KEY=
ANTHROPIC_API_KEY=
```

## Modal Deployment

```bash
modal deploy scripts/modal_services.py --env goat
```

Runs stateless (no database). Uses `dev/litellm-config-modal.yaml`.

## Key Settings

```yaml
litellm_settings:
  drop_params: true          # Critical — silently drop unsupported params
  max_budget: 1000
  budget_duration: 1mo
  success_callback: ["langfuse"]
  failure_callback: ["langfuse"]
```

`drop_params: true` prevents errors when sending provider-specific params (e.g. Anthropic thinking blocks) to models that don't support them.

## Go Code Integration

| File | Purpose |
|------|---------|
| `pkg/llm/config.go` | `ClientConfig.BaseURL` — the proxy URL |
| `pkg/llm/pricing.go` | `FetchPricing()` — pulls from `/model/info` endpoint |
| `pkg/llm/request.go` | `extra_body` — LiteLLM passthrough for Anthropic fields |
| `pkg/llm/translate.go` | `IsLocalModel()` — `-local` suffix detection |
| `cmd/example/main.go` | Provider config with key lookup: `EXECUTOR_LITELLM_KEY` > `LITELLM_MASTER_KEY` > `LITELLM_API_KEY` |

## Adding a New Cloud Model

1. Add entry to `dev/litellm-config.yaml` and `dev/litellm-config-modal.yaml`
2. Pass provider key through Docker Compose env and Modal secrets
3. Restart: `docker compose -f dev/docker-compose.yml restart litellm`

## Adding a New vLLM Model

1. Deploy the vLLM app (see [vllm-gpu-serving.md](vllm-gpu-serving.md))
2. Add entry to config with `<key>-local` name and Modal URL
