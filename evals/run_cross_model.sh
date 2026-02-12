#!/usr/bin/env bash
# Cross-model benchmark runner for GOAT-15.
#
# Runs HumanEval and Terminal-Bench with each model configured in the LiteLLM proxy.
# Results are collected into evals/results/ and summarized in a comparison report.
#
# Prerequisites:
#   1. LiteLLM proxy running: cd dev && docker compose up -d
#   2. Docker Desktop running (for Inspect AI sandboxes)
#   3. Go binary built: CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o evals/goat-eval-linux ./cmd/eval/
#   4. Python venv activated: cd evals && source .venv/bin/activate
#
# Usage:
#   ./evals/run_cross_model.sh                    # run all models, all benchmarks
#   ./evals/run_cross_model.sh gpt-5-nano         # single model
#   BENCHMARKS=humaneval ./evals/run_cross_model.sh  # single benchmark
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="${SCRIPT_DIR}/results"
REPORT_FILE="${RESULTS_DIR}/cross-model-comparison.md"

# Models to benchmark (override with first argument)
DEFAULT_MODELS=(
  "gpt-5-nano"
  "gpt-5-mini"
  "gpt-4o-mini"
  "llama-3.3-70b"
)

# Benchmarks to run (override with BENCHMARKS env var)
DEFAULT_BENCHMARKS=(
  "humaneval"
  "terminal_bench"
)

MODELS=("${@:-${DEFAULT_MODELS[@]}}")
if [[ ${#MODELS[@]} -eq 0 ]]; then
  MODELS=("${DEFAULT_MODELS[@]}")
fi

BENCHMARKS_STR="${BENCHMARKS:-}"
if [[ -n "$BENCHMARKS_STR" ]]; then
  IFS=',' read -ra BENCHMARKS <<< "$BENCHMARKS_STR"
else
  BENCHMARKS=("${DEFAULT_BENCHMARKS[@]}")
fi

# LiteLLM proxy URL (default local)
LITELLM_URL="${OPENAI_BASE_URL:-http://localhost:4000/v1}"
LITELLM_KEY="${OPENAI_API_KEY:-}"

# Ensure results directory exists
mkdir -p "$RESULTS_DIR"

echo "=== GOAT Cross-Model Benchmark ==="
echo "Models:     ${MODELS[*]}"
echo "Benchmarks: ${BENCHMARKS[*]}"
echo "LiteLLM:    $LITELLM_URL"
echo "Results:    $RESULTS_DIR"
echo ""

# Check prerequisites
if [[ ! -f "${SCRIPT_DIR}/goat-eval-linux" ]]; then
  echo "ERROR: goat-eval-linux binary not found."
  echo "Build it: CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o evals/goat-eval-linux ./cmd/eval/"
  exit 1
fi

# Run benchmarks
for model in "${MODELS[@]}"; do
  for bench in "${BENCHMARKS[@]}"; do
    echo "--- Running: ${bench} with ${model} ---"
    
    log_file="${RESULTS_DIR}/${model}_${bench}_$(date +%Y%m%d_%H%M%S).log"
    
    OPENAI_BASE_URL="$LITELLM_URL" \
    OPENAI_API_KEY="$LITELLM_KEY" \
    EVAL_MODEL="$model" \
    inspect eval "${SCRIPT_DIR}/${bench}.py" \
      --model "openai/${model}" \
      --model-base-url "$LITELLM_URL" \
      --sandbox-compose "${SCRIPT_DIR}/compose.yaml" \
      --log-dir "$RESULTS_DIR" \
      2>&1 | tee "$log_file"
    
    echo "--- Done: ${bench} with ${model} ---"
    echo ""
  done
done

# Generate summary report
echo "=== Generating comparison report ==="

cat > "$REPORT_FILE" << 'REPORT_HEADER'
# Cross-Model Benchmark Comparison

## Run Configuration

| Setting | Value |
|---------|-------|
REPORT_HEADER

echo "| Date | $(date +%Y-%m-%d) |" >> "$REPORT_FILE"
echo "| Models | ${MODELS[*]} |" >> "$REPORT_FILE"
echo "| Benchmarks | ${BENCHMARKS[*]} |" >> "$REPORT_FILE"
echo "| LiteLLM URL | $LITELLM_URL |" >> "$REPORT_FILE"
echo "| Goat version | $(git -C "$SCRIPT_DIR/.." rev-parse --short HEAD 2>/dev/null || echo 'unknown') |" >> "$REPORT_FILE"

cat >> "$REPORT_FILE" << 'REPORT_BODY'

## Results

> Review Inspect AI log files in `evals/results/` for detailed per-sample data.
> Use `inspect view` to browse results interactively.

### Summary Table

| Model | Benchmark | Accuracy | Stderr | Samples | Runtime | Est. Cost |
|-------|-----------|----------|--------|---------|---------|-----------|

*Fill in from inspect eval output or log files.*

### Analysis

#### Cost-Performance Frontier

*Plot accuracy vs estimated cost per model to identify the best value models.*

#### Model Recommendations

| Task Type | Best Model | Rationale |
|-----------|-----------|-----------|
| Code generation (HumanEval) | | |
| Terminal/CLI (Terminal-Bench) | | |
| Complex engineering (SWE-bench) | | |

## Next Steps

- [ ] Fill in results table from inspect eval output
- [ ] Run SWE-bench if Docker space allows (~280GB images)
- [ ] Compare Goat results against Claude Code native (GOAT-14)
- [ ] Generate accuracy vs cost scatter plot
REPORT_BODY

echo ""
echo "=== Cross-model benchmark complete ==="
echo "Results: $RESULTS_DIR"
echo "Report:  $REPORT_FILE"
echo ""
echo "View results interactively: inspect view --log-dir $RESULTS_DIR"
