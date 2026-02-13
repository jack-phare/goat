#!/usr/bin/env bash
# Run the smoke benchmark across all available models on Modal.
#
# Usage:
#   bash scripts/run_cross_model_modal.sh
#
# Prerequisites:
#   - modal deploy scripts/modal_services.py --env goat   (LiteLLM + Langfuse)
#   - modal deploy scripts/modal_vllm.py --env goat       (vLLM for local model)
#   - bash scripts/build_eval.sh amd64                    (goat-eval binary)
#
# Results are stored on the goat-results Modal Volume and can be viewed with:
#   uv run --with modal python3 scripts/modal_results.py

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BENCHMARK="$SCRIPT_DIR/benchmark_smoke.json"
PARALLEL=2
MODAL_PYTHON="uv run --with modal python3"

# Models to benchmark (must match names in dev/litellm-config-modal.yaml)
MODELS="gpt-5-nano gpt-5-mini gpt-4o-mini llama-3.3-70b llama-3.1-8b-local"

echo "================================================================"
echo "Cross-Model Benchmark: $(date)"
echo "Benchmark: $BENCHMARK"
echo "Models: $MODELS"
echo "Parallelism: $PARALLEL"
echo "================================================================"
echo ""

# Track results in temp file (compatible with bash 3.x on macOS)
RESULTS_FILE=$(mktemp)
trap "rm -f $RESULTS_FILE" EXIT

for model in $MODELS; do
    echo "────────────────────────────────────────────────────────────────"
    echo "Running: $model"
    echo "────────────────────────────────────────────────────────────────"

    # Capture output and extract run ID
    output=$($MODAL_PYTHON "$SCRIPT_DIR/modal_sandbox.py" \
        --batch "$BENCHMARK" \
        --model "$model" \
        --parallel "$PARALLEL" 2>&1) || true

    echo "$output"
    echo ""

    # Extract run ID from output (format: "Run ID: 20260213_113758")
    run_id=$(echo "$output" | grep "Run ID:" | tail -1 | awk '{print $NF}')
    # Extract pass/total from output (format: "Batch complete: 5/5 passed")
    result=$(echo "$output" | grep "Batch complete:" | tail -1 | sed 's/.*Batch complete: //' | sed 's/ passed.*//')

    if [ -n "$run_id" ]; then
        echo "$model|${result:-UNKNOWN}|$run_id" >> "$RESULTS_FILE"
    else
        echo "$model|FAILED|N/A" >> "$RESULTS_FILE"
    fi
done

# Print comparison summary
echo ""
echo "================================================================"
echo "CROSS-MODEL COMPARISON SUMMARY"
echo "================================================================"
printf "%-25s  %-15s  %-20s\n" "MODEL" "RESULT" "RUN ID"
echo "────────────────────────────────────────────────────────────────"

failed_count=0
while IFS='|' read -r model result run_id; do
    printf "%-25s  %-15s  %-20s\n" "$model" "$result" "$run_id"
    if [ "$result" = "FAILED" ]; then
        failed_count=$((failed_count + 1))
    fi
done < "$RESULTS_FILE"

total=$(echo "$MODELS" | wc -w | tr -d ' ')
echo ""
echo "Total models: $total"
echo "Failed: $failed_count"

echo ""
echo "View detailed results:"
while IFS='|' read -r model result run_id; do
    if [ "$run_id" != "N/A" ]; then
        echo "  $model: $MODAL_PYTHON scripts/modal_results.py $run_id"
    fi
done < "$RESULTS_FILE"

echo ""
echo "View Langfuse traces:"
echo "  https://phare-goat--goat-services-langfuse.modal.run"
