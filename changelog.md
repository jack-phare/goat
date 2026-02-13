# Changelog

## 2026-02-13 — Modal Infrastructure Complete + Cross-Model Benchmarks

Full Modal eval infrastructure deployed and validated with 5/5 pass rate across all 5 models.

### Infrastructure

- **Langfuse v2** self-hosted on Modal with co-located Postgres (`scripts/modal_services.py`, `scripts/Dockerfile.langfuse`). Key learning: Modal proxies HTTP only, so Postgres must run on localhost within the same function. Alpine base image required to match Prisma binary targets.
- **vLLM** serving Llama-3.1-8B-Instruct on A10G GPU (`scripts/modal_vllm.py`). Scales to zero when idle. Pinned to `vllm==0.8.5.post1` + `transformers<5.0` for compatibility.
- **LiteLLM proxy** routing to 6 models: gpt-4o-mini, gpt-5-nano, gpt-5-mini (Azure), llama-3.3-70b (Groq), llama-3.1-8b-local (vLLM), text-embedding-3-small (OpenAI). Config: `dev/litellm-config-modal.yaml`.

### Benchmarks

- Created 5-task smoke benchmark (`scripts/benchmark_smoke.json`): math, file creation, bash commands, code fixing, grep search.
- Cross-model runner (`scripts/run_cross_model_modal.sh`) — all 5 models passed all 5 tasks.
- **Results**: gpt-4o-mini fastest (24s), gpt-5-mini (50s), llama-3.1-8b-local (82s), gpt-5-nano (104s), llama-3.3-70b (398s with Groq tool-call retries).

### Bug Fixes

- Fixed `@modal.concurrent` deprecation, `p.wait()` before `returncode`, `Function.get_web_url()` API (`scripts/modal_sandbox.py`).

### Known Issues

- Langfuse callbacks intermittently fail (ephemeral Postgres). See `thoughts/tickets/BUG-langfuse-callback-errors.md`.
- Groq tool calling unreliable for Llama models. See `thoughts/tickets/BUG-groq-tool-calling-failures.md`.
