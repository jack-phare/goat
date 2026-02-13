# Changelog

## 2026-02-13 — Groq Tool-Calling Fix

Addressed `GroqException - Failed to call a function` errors that caused 10-16x slowdown on Groq/Llama benchmarks via LiteLLM mid-stream retries.

### Fixes

- **Model-aware request tuning**: Auto-detect Groq/Llama models via `IsGroqLlama()` and set `temperature: 0.3` + `tool_choice: "auto"` per Groq best practices (`pkg/llm/translate.go`, `pkg/llm/request.go`, `pkg/llm/types_wire.go`).
- **Renamed GrepTool parameters**: `-i` → `case_insensitive`, `-A` → `after_context`, `-B` → `before_context`, `-C` → `context_lines`, `-n` → `show_line_numbers`. Old names still work via fallback lookup (`pkg/tools/grep.go`).
- **Compact tool descriptions**: Added `CompactLLMTools()` returning 1-2 sentence descriptions instead of 40+ line verbose ones. Activated via `AgentConfig.CompactTools` (`pkg/tools/adapter.go`, `pkg/tools/registry.go`, `pkg/agent/config.go`).
- **Groq-optimized system prompt**: ~150 token focused prompt replaces ~3000+ token Claude Code assembler for Groq models in eval binary (`cmd/eval/main.go`).
- **Llama 4 Scout config**: Added commented-out `llama-4-scout` model with better tool calling support (`dev/litellm-config.yaml`).

### Verified

- Groq API (llama-3.3-70b + llama-4-scout) returns clean tool calls via curl and standalone Go SSE tests.
- All unit tests pass (pkg/llm, pkg/agent, pkg/tools), including backwards-compat for old GrepTool param names.

### Known Issue (pre-existing, separate bug)

- `cmd/example/` binary hangs when using direct Groq API with tools enabled. Root cause: `http.DefaultClient` has no body-read timeout, so `bufio.Scanner.Scan()` in `ParseSSEStream` blocks indefinitely if Groq's SSE connection stalls. Does NOT affect the eval binary (routes through LiteLLM which has its own retry/timeout logic).

---

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
