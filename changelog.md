# Changelog

## 2026-02-13 — Custom Skills Evaluation Framework

Wired goat's existing skill infrastructure into the eval binary and created benchmark skills and tasks for A/B testing skill-augmented vs vanilla eval runs.

### Eval Binary Skill Support

- **`-skills-dir` flag** (`cmd/eval/main.go`): When provided, loads skills from the given directory using `prompt.NewSkillLoader`, registers them in a `SkillRegistry`, wires a `SkillProviderAdapter`, and registers the `SkillTool` with argument substitution. The assembler injects skill metadata into the system prompt. When omitted, behavior is unchanged (baseline mode).

### Benchmark Skills

Three skills created in `eval/skills/.claude/skills/`, following the standard `.claude/skills/{name}/SKILL.md` convention:

- **`go-expert`** (~1k tokens): Go idioms, error handling (`fmt.Errorf` wrapping, sentinel errors), concurrency patterns (errgroup, context cancellation), functional options, stdlib usage (slog, encoding/json, filepath).
- **`project-context`** (~1.1k tokens): Simulated layered Go web app — directory structure, repository pattern, handler/service/domain/storage layers, chi router conventions, config-from-env pattern.
- **`testing-patterns`** (~1.7k tokens): Table-driven tests with `t.Run`, hand-crafted mocks via small interfaces, `httptest` for handler testing, golden files, `t.Helper()`/`t.Cleanup()` patterns, benchmarks.

### Skill-Specific Benchmark Tasks

- **`eval/benchmark_skills.json`**: 15 tasks (5 per skill) across 3 categories:
  - **Skill-relevant** (3 per skill): tasks where the specific skill knowledge is directly applicable.
  - **Skill-agnostic** (1 per skill): baseline tasks where skills shouldn't help or hurt.
  - **Skill-transfer** (1 per skill): adjacent-domain tasks testing generalization.

### A/B Benchmark Runner

- **`--skills-dir` flag** (`scripts/modal_sandbox.py`): Mounts the skills directory into the Modal sandbox at `/opt/skills/` and passes `-skills-dir /opt/skills` to `goat-eval`.
- **`--ab` flag**: Runs each task twice (baseline without skills, then with skills), stores results with `-baseline`/`-skills` suffixes, and generates a paired comparison summary with pass rates for both variants.

### Tests

- **Integration test** (`cmd/eval/skills_integration_test.go`): Validates the full pipeline — load 3 skills from `eval/skills/`, register in registry, format for system prompt, wire adapter, invoke SkillTool, verify body content, verify tool registry.
- **E2E test** (`cmd/eval/skills_e2e_test.go`): Runs the full agent loop against a live LLM (gpt-4o-mini). Uses a `secret-word` test skill containing "BANANA-TRUMPET-42" — confirms the LLM sees skills in the system prompt, emits `tool_use: Skill(skill="secret-word")`, receives the body, and includes the secret word in its final response. Requires `OPENAI_API_KEY`.

### Verified

- Integration test: 7/7 subtests pass (load, register, format, invoke x3, content check, unknown error, registry).
- E2E test: gpt-4o-mini invoked the Skill tool and returned "The secret word is: BANANA-TRUMPET-42." (2.77s).
- All 72 existing skill unit tests pass (pkg/prompt, pkg/tools).
- Eval binary loads 3 skills from `eval/skills/` and compiles cleanly.

---

## 2026-02-13 — Persistent Langfuse Storage + Trace Verification

Langfuse trace data now persists across Modal container restarts via a volume-backed Postgres. Added tooling to verify traces and sync them to local dev.

### Persistence

- **Volume-backed Postgres** (`goat-langfuse-pg`): Postgres data dir copied from image template on first boot, reused on restarts. Prisma migrations become instant no-ops. (`scripts/modal_services.py`)
- **Clean Postgres shutdown**: SIGTERM handler flushes WAL to volume before container teardown, preventing crash recovery on restart.
- **Langfuse readiness gate**: LiteLLM waits for Langfuse `/api/public/health` before starting, trying internal URL then public URL fallback.
- **Fixed internal URL**: Modal's `@modal.web_server` proxies on port 80, not the container port. Changed from `goat-services-langfuse.modal.internal:3000` to `goat-services-langfuse.modal.internal` with public URL fallback. This was the root cause of all Langfuse callback failures.

### Trace Verification

- **`scripts/verify_langfuse_traces.py`**: Fires a test call through LiteLLM (default: `qwen3-4b-local`), then verifies the trace via Langfuse REST API (JSON) and optionally raw Postgres SQL (`--sql`).
- **`scripts/langfuse_sync_local.sh`**: Exports a `pg_dump` from the Modal volume and restores it into local docker-compose Langfuse at `http://localhost:3001`.
- **`langfuse_query()` Modal function**: Runs arbitrary SQL or `pg_dump` against the volume-backed Postgres. Used by both scripts above.

### Test Improvements

- Phase 3 now verifies the `goat-langfuse-pg` volume exists after deploy.
- Phase 4 now automatically verifies traces via the Langfuse REST API instead of a manual dashboard check.

### Bug Fixes

- Fixed: Langfuse callbacks intermittently failing due to ephemeral Postgres (`BUG-langfuse-callback-errors`).

---

## 2026-02-13 — vLLM Model Expansion

Parameterized the vLLM deployment to support multiple local models on Modal, expanding from a single hardcoded Llama model to 6 configurable models across 3 GPU tiers.

### New Models

- **Qwen3-4B** (`qwen3-4b-local`): Qwen/Qwen3-4B-Instruct-2507 on A10G -- lightweight, fast.
- **Qwen3-30B-A3B** (`qwen3-30b-a3b-local`): MoE model, 3B active params on A10G.
- **Qwen3-235B** (`qwen3-235b-local`): Large MoE on A100-80GB (deploy on demand).
- **GPT OSS 20B** (`gpt-oss-20b-local`): OpenAI's open MoE model on H100 with speculative decoding.
- **GPT OSS 120B** (`gpt-oss-120b-local`): Larger variant on H100 (deploy on demand).

### Infrastructure

- **Parameterized vLLM script**: `VLLM_MODEL=<key> modal deploy scripts/modal_vllm.py` creates a uniquely-named app per model (e.g. `goat-vllm-qwen3-4b`). Multiple models live simultaneously, each scales to zero independently.
- **Dedicated GPT OSS script** (`scripts/modal_gpt_oss.py`): CUDA 12.8 image, vLLM 0.13.0, EAGLE3 speculative decoding, flashinfer MoE kernels.
- **Model registry** (`scripts/model_registry.py`): reference doc for all model configs. Deploy scripts inline configs to avoid Modal container import issues.
- **LiteLLM routing**: all 6 new models added to `litellm-config-modal.yaml` with `-local` suffix naming convention.

### Go Code

- `IsLocalModel()` in `pkg/llm/translate.go` detects `-local` suffix models and applies temperature 0.3 + `tool_choice: "auto"` for reliable tool calling with open-weight models.

### Verified

- Smoke benchmark (5 tasks) passed 5/5 on qwen3-4b-local (74s), llama-3.1-8b-local (90s), gpt-oss-20b-local (241s).

---

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

- Groq tool calling unreliable for Llama models. See `thoughts/tickets/BUG-groq-tool-calling-failures.md`.
