# Benchmark Results

## Run: 2026-02-09 (Baseline)

**Model**: `gpt-5-nano` via LiteLLM proxy
**Goat version**: commit `f5f1986` (main)
**Platform**: macOS Darwin 25.2.0 / Apple Silicon (ARM64)
**Docker**: Desktop with Linux ARM64 containers

### HumanEval

| Metric | Value |
|--------|-------|
| Samples | 164/164 |
| **Accuracy** | **0.421 (42.1%)** |
| Stderr | 0.039 |
| Runtime | 4 min 57 sec |
| Scorer | `verify` |

HumanEval tests code generation: given a function signature and docstring, generate the implementation. Goat runs inside a Docker sandbox with Bash, Read, Write, Edit, Glob, and Grep tools available.

### Terminal-Bench 2.0

| Metric | Value |
|--------|-------|
| Samples | 89/89 |
| **Accuracy** | **0.011 (1.1%)** |
| Stderr | 0.011 |
| Runtime | 17 min 45 sec |
| Scorer | `terminal_bench_2_scorer` |

Terminal-Bench tests terminal/CLI problem-solving in Docker challenge environments. The goat-eval binary is injected into each challenge container at runtime via `sandbox().write_file()`.

**Published baselines for comparison** (direct API, no agentic loop):

| Model | Score |
|-------|-------|
| gpt-5-nano | 12.4% |
| gpt-5-mini | 31.5% |
| claude-haiku-4-5 | 37.1% |

The gap between our 1.1% and the baseline 12.4% is likely due to the binary injection approach and prompt engineering differences. The Goat agent receives the challenge prompt but interacts with the environment through its own tool loop rather than direct model-to-terminal interaction.

### SWE-bench Verified

Not yet run. Requires ~280GB of Docker images on first pull.

## Environment Details

- LiteLLM proxy running locally on `http://localhost:4000/v1`
- Docker sandbox: 4 CPUs, 8GB RAM per container
- Host-to-container networking via `host.docker.internal`
- Goat eval binary: statically compiled Go (CGO_ENABLED=0), ~8.7MB

## Cost Estimate

| Benchmark | Estimated Cost |
|-----------|---------------|
| HumanEval (164 samples) | ~$0.50 |
| Terminal-Bench (89 samples) | ~$0.30 |
| SWE-bench (50 samples) | ~$1.00 |

All costs via gpt-5-nano, the cheapest available model.

## Cross-Model Benchmark (GOAT-15)

**Status**: Runner script ready at `evals/run_cross_model.sh`

### Models to benchmark

| Model | Provider | Status |
|-------|----------|--------|
| gpt-5-nano | Azure OpenAI | baseline done (see above) |
| gpt-5-mini | Azure OpenAI | pending |
| gpt-4o-mini | Azure OpenAI | pending |
| llama-3.3-70b | Groq | pending |

### How to run

```bash
# 1. Start LiteLLM proxy
cd dev && docker compose up -d

# 2. Build Linux eval binary
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o evals/goat-eval-linux ./cmd/eval/

# 3. Activate evals venv
cd evals && source .venv/bin/activate

# 4. Run all models
./evals/run_cross_model.sh

# 5. Or run a single model
./evals/run_cross_model.sh gpt-5-mini
```

Results will be saved to `evals/results/` with an auto-generated comparison report.
