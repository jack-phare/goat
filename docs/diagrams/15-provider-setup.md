# Provider Setup & Resolution Priority

> `cmd/example/main.go` — How the example resolves which LLM provider to use,
> and how requests flow from the Go binary to the actual model.

## Provider Resolution Flow

When you run the example, provider config is resolved in this order:

```
 go run ./cmd/example/ [-provider X] [-base-url Y] [-api-key Z] [-model M]
         │
         ▼
 ┌──────────────────────────────────────────────────────────────────────┐
 │                       resolveConfig()                                │
 │                                                                      │
 │  STEP 1: Custom base URL?                                           │
 │  ─────────────────────────                                          │
 │  if -base-url is set:                                               │
 │    → use exactly what's provided (-api-key, -model required)        │
 │    → provider = "custom"                                            │
 │    → DONE                                                           │
 │                                                                      │
 │  STEP 2: Explicit provider?                                         │
 │  ──────────────────────────                                         │
 │  if -provider is set:                                               │
 │    → look up in providers map                                       │
 │    → GOTO STEP 4                                                    │
 │                                                                      │
 │  STEP 3: Auto-detect from env vars                                  │
 │  ────────────────────────────────                                   │
 │  Check env vars in this priority order:                             │
 │                                                                      │
 │    ┌─────┐    GROQ_API_KEY set?                                     │
 │    │  1  │───── YES → provider = "groq"                             │
 │    └──┬──┘                                                          │
 │       │ NO                                                          │
 │    ┌─────┐    OPENAI_API_KEY set?                                   │
 │    │  2  │───── YES → provider = "openai"                           │
 │    └──┬──┘                                                          │
 │       │ NO                                                          │
 │    ┌─────┐    ANTHROPIC_API_KEY set?                                │
 │    │  3  │───── YES → provider = "anthropic"                        │
 │    └──┬──┘                                                          │
 │       │ NO                                                          │
 │    ┌─────┐    EXECUTOR_LITELLM_KEY set?                             │
 │    │  4  │───── YES → provider = "litellm"                          │
 │    └──┬──┘                                                          │
 │       │ NO                                                          │
 │    ┌─────┐    LITELLM_MASTER_KEY set?  ◄── fallback envKeys        │
 │    │  5  │───── YES → provider = "litellm"                          │
 │    └──┬──┘                                                          │
 │       │ NO                                                          │
 │    ┌─────┐    LITELLM_API_KEY set?     ◄── fallback envKeys        │
 │    │  6  │───── YES → provider = "litellm"                          │
 │    └──┬──┘                                                          │
 │       │ NO                                                          │
 │       ▼                                                             │
 │    ERROR: "no provider specified and no API key found"              │
 │                                                                      │
 │  STEP 4: Build config from provider defaults                        │
 │  ───────────────────────────────────────────                        │
 │    baseURL  = provider default (or env override via baseURLEnv)     │
 │    apiKey   = -api-key flag, or lookupKey(primary → fallbacks)      │
 │    model    = -model flag, or provider default                      │
 │                                                                      │
 └──────────────────────────────────────────────────────────────────────┘
```

## Provider Defaults

```
 ┌────────────┬──────────────────────────────────────┬──────────────────────────────┬───────────────┐
 │  Provider  │  Base URL                            │  API Key Env Vars            │  Default Model│
 ├────────────┼──────────────────────────────────────┼──────────────────────────────┼───────────────┤
 │  groq      │  https://api.groq.com/openai/v1     │  GROQ_API_KEY                │  llama-3.3-   │
 │            │  (override: GROQ_API_BASE)           │                              │  70b-versatile│
 ├────────────┼──────────────────────────────────────┼──────────────────────────────┼───────────────┤
 │  openai    │  https://api.openai.com/v1           │  OPENAI_API_KEY              │  gpt-4o-mini  │
 ├────────────┼──────────────────────────────────────┼──────────────────────────────┼───────────────┤
 │  anthropic │  https://api.anthropic.com/v1        │  ANTHROPIC_API_KEY           │  claude-      │
 │            │                                      │                              │  sonnet-4-5   │
 ├────────────┼──────────────────────────────────────┼──────────────────────────────┼───────────────┤
 │  litellm   │  http://localhost:4000/v1            │  EXECUTOR_LITELLM_KEY        │  gpt-5-nano   │
 │            │  (override: LITELLM_BASE_URL)        │  → LITELLM_MASTER_KEY        │               │
 │            │                                      │  → LITELLM_API_KEY           │               │
 └────────────┴──────────────────────────────────────┴──────────────────────────────┴───────────────┘

 Note: LiteLLM has 3 env vars checked in order (→ = fallback).
 All other providers have a single env var.
```

## API Key Lookup: lookupKey()

```
 lookupKey(providerConfig)
         │
         ▼
 ┌─────────────────────────────────────────┐
 │  1. Check primary: os.Getenv(envKey)    │
 │     e.g. EXECUTOR_LITELLM_KEY           │
 │          │                              │
 │          ├── set? → return value        │
 │          │                              │
 │          ▼                              │
 │  2. Check fallbacks: envKeys[]          │
 │     e.g. LITELLM_MASTER_KEY             │
 │          │                              │
 │          ├── set? → return value        │
 │          │                              │
 │          ▼                              │
 │     e.g. LITELLM_API_KEY               │
 │          │                              │
 │          ├── set? → return value        │
 │          │                              │
 │          ▼                              │
 │  3. return "" (caller handles error)    │
 └─────────────────────────────────────────┘
```

## Request Flow: Example → LLM

```
 ┌──────────────────┐
 │  cmd/example/    │
 │  main.go         │
 │                  │
 │  resolveConfig() │──▶ resolvedConfig {
 │                  │       BaseURL:  "http://localhost:4000/v1"
 │                  │       APIKey:   "sk-..."
 │                  │       Model:    "gpt-5-nano"
 │                  │       Provider: "litellm"
 │                  │    }
 └────────┬─────────┘
          │
          ▼
 ┌──────────────────┐
 │  llm.NewClient() │
 │                  │
 │  ClientConfig {  │
 │    BaseURL       │
 │    APIKey        │
 │    Model         │
 │  }               │
 └────────┬─────────┘
          │
          ▼
 ┌──────────────────┐
 │  agent.RunLoop() │
 │                  │
 │  config.Model =  │
 │  "gpt-5-nano"    │──── must match client model
 │                  │
 └────────┬─────────┘
          │
          ▼
 ┌──────────────────────────────────────────────────────────────┐
 │  pkg/llm — BuildCompletionRequest                            │
 │                                                              │
 │  Model prefix logic (translate.go:36-57):                   │
 │                                                              │
 │  "gpt-5-nano"                                               │
 │    → has no "/" prefix                                       │
 │    → does NOT start with "claude-"                          │
 │    → PASS THROUGH as-is: "gpt-5-nano"                      │
 │                                                              │
 │  "claude-sonnet-4-5-20250929"                               │
 │    → has no "/" prefix                                       │
 │    → starts with "claude-"                                  │
 │    → ADD PREFIX: "anthropic/claude-sonnet-4-5-20250929"     │
 │                                                              │
 │  "groq/llama-3.3-70b-versatile"                             │
 │    → already has "/" prefix                                  │
 │    → PASS THROUGH as-is                                     │
 └────────────────────┬─────────────────────────────────────────┘
                      │
                      ▼
 ┌──────────────────────────────────────────────────────────────┐
 │  HTTP POST → {BaseURL}/chat/completions                      │
 │                                                              │
 │  Headers:                                                    │
 │    Authorization: Bearer {APIKey}                            │
 │    Content-Type: application/json                            │
 │                                                              │
 │  Body:                                                       │
 │    model: "gpt-5-nano"                                      │
 │    messages: [...]                                           │
 │    tools: [...]          (if tools enabled)                  │
 │    stream: true                                              │
 │    extra_body: {...}     (ONLY if thinking/betas present)   │
 │                                                              │
 │  extra_body is OMITTED for non-Anthropic models.            │
 │  This prevents 400 errors from Groq/OpenAI.                │
 └────────────────────┬─────────────────────────────────────────┘
                      │
                      ▼
 ┌──────────────────────────────────────────────────────────────┐
 │                    LiteLLM Proxy                              │
 │                    localhost:4000                              │
 │                                                              │
 │  Receives: model="gpt-5-nano"                               │
 │  Matches: litellm-config.yaml entry                         │
 │  Routes to: azure/gpt-5-nano                                │
 │                                                              │
 │  ┌────────────────────────────────────────────────────────┐  │
 │  │  model_list:                                           │  │
 │  │    - model_name: gpt-5-nano       ◄── matched         │  │
 │  │      litellm_params:                                   │  │
 │  │        model: azure/gpt-5-nano    ◄── actual provider  │  │
 │  │        api_key: $AZURE_API_KEY                         │  │
 │  │        api_base: $AZURE_API_BASE                       │  │
 │  │                                                        │  │
 │  │    - model_name: llama-3.3-70b                         │  │
 │  │      litellm_params:                                   │  │
 │  │        model: groq/llama-3.3-70b-versatile             │  │
 │  │        api_key: $GROQ_API_KEY                          │  │
 │  └────────────────────────────────────────────────────────┘  │
 │                                                              │
 │  Settings:                                                   │
 │    drop_params: true  → unknown params silently dropped     │
 │    langfuse callbacks → observability                       │
 │    budget: $1000/month                                      │
 └────────────────────┬─────────────────────────────────────────┘
                      │
                      ▼
 ┌──────────────────────────────────────────────────────────────┐
 │                    Azure OpenAI                               │
 │                                                              │
 │  POST {AZURE_API_BASE}/openai/deployments/gpt-5-nano/       │
 │       chat/completions?api-version=2024-06-01                │
 │                                                              │
 │  Returns: SSE stream (OpenAI format)                        │
 │    data: {"choices":[{"delta":{"content":"..."}}]}          │
 │    data: [DONE]                                              │
 └──────────────────────────────────────────────────────────────┘
```

## Direct Provider Flow (No Proxy)

```
 When using -provider groq or -provider openai directly,
 the request bypasses LiteLLM entirely:

 ┌────────────┐     ┌──────────────┐     ┌──────────────────────┐
 │  Example   │────▶│  pkg/llm     │────▶│  api.groq.com/       │
 │            │     │  Client      │     │  openai/v1/chat/     │
 │  provider: │     │              │     │  completions         │
 │  "groq"    │     │  model:      │     │                      │
 │            │     │  "llama-3.3- │     │  model: "llama-3.3-  │
 │            │     │  70b-        │     │  70b-versatile"      │
 │            │     │  versatile"  │     │                      │
 └────────────┘     └──────────────┘     └──────────────────────┘
   No prefix added (not a claude-* model)
   No extra_body (not an Anthropic model)

 ┌────────────┐     ┌──────────────┐     ┌──────────────────────┐
 │  Example   │────▶│  pkg/llm     │────▶│  api.anthropic.com/  │
 │            │     │  Client      │     │  v1/chat/completions │
 │  provider: │     │              │     │                      │
 │  "anthropic│     │  model:      │     │  model: "anthropic/  │
 │            │     │  "anthropic/ │     │  claude-sonnet-4-5-  │
 │            │     │  claude-..." │     │  20250929"           │
 └────────────┘     └──────────────┘     └──────────────────────┘
   Prefix added: "anthropic/" (bare claude-* model)
   extra_body sent if thinking/betas configured
```

## Environment Setup Cheat Sheet

```
 ┌─────────────────────────────────────────────────────────────────┐
 │  SCENARIO 1: Production (agent-hub proxy already running)       │
 │                                                                 │
 │  export EXECUTOR_LITELLM_KEY=sk-...                            │
 │  go run ./cmd/example/ -provider litellm                       │
 │  # Uses gpt-5-nano via Azure OpenAI through production proxy   │
 ├─────────────────────────────────────────────────────────────────┤
 │  SCENARIO 2: Dev proxy (local docker-compose)                   │
 │                                                                 │
 │  cp dev/.env.example dev/.env  # fill in AZURE_API_KEY etc.    │
 │  docker compose -f dev/docker-compose.yml up -d                │
 │  export LITELLM_MASTER_KEY=sk-dev-key                          │
 │  go run ./cmd/example/ -provider litellm                       │
 │  # Uses gpt-5-nano via Azure OpenAI through dev proxy          │
 ├─────────────────────────────────────────────────────────────────┤
 │  SCENARIO 3: Direct Groq (no proxy, free tier)                  │
 │                                                                 │
 │  export GROQ_API_KEY=gsk_...                                   │
 │  go run ./cmd/example/ -provider groq                          │
 │  # Uses llama-3.3-70b-versatile directly                       │
 ├─────────────────────────────────────────────────────────────────┤
 │  SCENARIO 4: Auto-detect (just set env vars)                    │
 │                                                                 │
 │  # Set any combination of keys. First match wins:              │
 │  # Priority: groq → openai → anthropic → litellm              │
 │  go run ./cmd/example/                                         │
 └─────────────────────────────────────────────────────────────────┘
```
