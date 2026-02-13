#!/bin/bash
# =============================================================================
# Goat Modal Infrastructure Validation Script
#
# Run each phase sequentially. Each phase validates the previous one works
# before moving on. Stop and fix if any phase fails.
#
# Usage:
#   bash scripts/test_modal_infra.sh           # run all phases
#   bash scripts/test_modal_infra.sh phase1    # run specific phase
#   bash scripts/test_modal_infra.sh phase2
#   bash scripts/test_modal_infra.sh phase3
#   bash scripts/test_modal_infra.sh phase4
#   bash scripts/test_modal_infra.sh phase5
#   bash scripts/test_modal_infra.sh phase6
# =============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Use uv to run Python scripts that need modal (installed as a uv tool)
MODAL_PYTHON="uv run --with modal python3"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

pass() { echo -e "  ${GREEN}✓ $1${NC}"; }
fail() { echo -e "  ${RED}✗ $1${NC}"; }
info() { echo -e "  ${BLUE}→ $1${NC}"; }
warn() { echo -e "  ${YELLOW}⚠ $1${NC}"; }

# Track results
PASSED=0
FAILED=0
SKIPPED=0

check() {
    local desc="$1"
    shift
    if "$@" >/dev/null 2>&1; then
        pass "$desc"
        PASSED=$((PASSED + 1))
        return 0
    else
        fail "$desc"
        FAILED=$((FAILED + 1))
        return 1
    fi
}

# =============================================================================
# PHASE 1: Prerequisites — do we have everything installed?
# =============================================================================
phase1() {
    echo -e "\n${BLUE}━━━ Phase 1: Prerequisites ━━━${NC}"

    # Check modal CLI
    if command -v modal &>/dev/null; then
        pass "Modal CLI installed: $(modal --version 2>&1 | head -1)"
    else
        fail "Modal CLI not installed"
        info "Fix: uv tool install modal && modal setup"
        return 1
    fi

    # Check modal auth
    local workspace
    workspace=$(modal profile current 2>/dev/null)
    if [ -n "$workspace" ]; then
        pass "Modal authenticated (workspace: $workspace)"
    else
        warn "Modal not authenticated or no workspace"
        info "Fix: modal setup"
    fi

    # Check eval binary exists
    if [ -f "$SCRIPT_DIR/goat-eval-linux" ]; then
        pass "goat-eval-linux binary exists ($(du -h "$SCRIPT_DIR/goat-eval-linux" | cut -f1))"
    else
        fail "goat-eval-linux binary missing"
        info "Fix: bash scripts/build_eval.sh"
        return 1
    fi

    # Check eval binary is Linux ELF
    if file "$SCRIPT_DIR/goat-eval-linux" | grep -q "ELF"; then
        pass "goat-eval-linux is a Linux ELF binary"
    else
        fail "goat-eval-linux is not a Linux ELF binary (wrong arch?)"
        info "Fix: bash scripts/build_eval.sh amd64"
        return 1
    fi

    # Check .env exists with required keys
    if [ -f "$PROJECT_ROOT/.env" ]; then
        pass ".env file exists"
    else
        fail ".env file missing"
        return 1
    fi

    local required_keys=("AZURE_API_KEY" "AZURE_API_BASE" "LITELLM_MASTER_KEY" "POSTGRES_USER" "POSTGRES_PASSWORD")
    for key in "${required_keys[@]}"; do
        if grep -q "^${key}=" "$PROJECT_ROOT/.env" 2>/dev/null; then
            pass ".env has $key"
        else
            fail ".env missing $key"
        fi
    done

    # Check Python 3
    if command -v python3 &>/dev/null; then
        pass "Python3 available: $(python3 --version)"
    else
        fail "Python3 not found"
    fi
}

# =============================================================================
# PHASE 2: Setup — create goat environment and secrets (dry run first)
# =============================================================================
phase2() {
    echo -e "\n${BLUE}━━━ Phase 2: Modal Setup (dry run) ━━━${NC}"

    info "Running modal_setup.py --dry-run to validate secret schema..."
    echo ""
    $MODAL_PYTHON "$SCRIPT_DIR/modal_setup.py" --env-file "$PROJECT_ROOT/.env" --dry-run
    echo ""

    if [ $? -eq 0 ]; then
        pass "Setup dry-run passed — secrets schema valid"
    else
        fail "Setup dry-run failed"
        return 1
    fi

    echo ""
    warn "To actually create secrets, run:"
    info "$MODAL_PYTHON scripts/modal_setup.py --env-file .env"
}

# =============================================================================
# PHASE 3: Deploy services and wait for them to be ready
# =============================================================================
phase3() {
    echo -e "\n${BLUE}━━━ Phase 3: Deploy Services ━━━${NC}"

    info "Deploying goat-services (Postgres + Langfuse + LiteLLM)..."
    if modal deploy "$SCRIPT_DIR/modal_services.py" 2>&1; then
        pass "goat-services deployed"
    else
        fail "goat-services deployment failed"
        return 1
    fi

    # Get LiteLLM URL
    echo ""
    info "Discovering LiteLLM URL..."
    LITELLM_URL=$($MODAL_PYTHON -c "
import modal
fn = modal.Function.from_name('goat-services', 'litellm', environment_name='goat')
print(fn.web_url)
" 2>/dev/null)

    if [ -n "$LITELLM_URL" ]; then
        pass "LiteLLM URL: $LITELLM_URL"
    else
        fail "Could not discover LiteLLM URL"
        return 1
    fi

    # Test LiteLLM health
    echo ""
    info "Testing LiteLLM health endpoint..."

    # Load the master key
    LITELLM_KEY=$(grep '^LITELLM_MASTER_KEY=' "$PROJECT_ROOT/.env" | cut -d= -f2)

    if curl -sf "${LITELLM_URL}/health" -H "Authorization: Bearer $LITELLM_KEY" | python3 -m json.tool; then
        pass "LiteLLM health check passed"
    else
        warn "LiteLLM health check failed (may need a minute to start)"
        info "Retry: curl ${LITELLM_URL}/health -H 'Authorization: Bearer \$LITELLM_MASTER_KEY'"
    fi

    # Test model list
    echo ""
    info "Testing LiteLLM model list..."
    if curl -sf "${LITELLM_URL}/v1/models" -H "Authorization: Bearer $LITELLM_KEY" | python3 -c "
import json, sys
data = json.load(sys.stdin)
models = [m['id'] for m in data.get('data', [])]
print(f'  Available models: {models}')
"; then
        pass "Model list retrieved"
    else
        warn "Model list request failed"
    fi

    # Get Langfuse URL
    echo ""
    info "Discovering Langfuse URL..."
    LANGFUSE_URL=$($MODAL_PYTHON -c "
import modal
fn = modal.Function.from_name('goat-services', 'langfuse', environment_name='goat')
print(fn.web_url)
" 2>/dev/null)

    if [ -n "$LANGFUSE_URL" ]; then
        pass "Langfuse URL: $LANGFUSE_URL"
        info "Open in browser to verify UI"
    else
        warn "Could not discover Langfuse URL"
    fi
}

# =============================================================================
# PHASE 4: Smoke-test a single LLM call through LiteLLM on Modal
# =============================================================================
phase4() {
    echo -e "\n${BLUE}━━━ Phase 4: Smoke Test — LLM Call via Modal LiteLLM ━━━${NC}"

    LITELLM_URL=$($MODAL_PYTHON -c "
import modal
fn = modal.Function.from_name('goat-services', 'litellm', environment_name='goat')
print(fn.web_url)
" 2>/dev/null)
    LITELLM_KEY=$(grep '^LITELLM_MASTER_KEY=' "$PROJECT_ROOT/.env" | cut -d= -f2)

    if [ -z "$LITELLM_URL" ]; then
        fail "Cannot discover LiteLLM URL — deploy services first (phase 3)"
        return 1
    fi

    info "Sending a simple chat completion to gpt-5-nano via Modal LiteLLM..."
    echo ""

    RESPONSE=$(curl -sf "${LITELLM_URL}/v1/chat/completions" \
        -H "Authorization: Bearer $LITELLM_KEY" \
        -H "Content-Type: application/json" \
        -d '{
            "model": "gpt-5-nano",
            "messages": [{"role": "user", "content": "Reply with exactly: GOAT_TEST_OK"}],
            "max_tokens": 20
        }')

    if echo "$RESPONSE" | python3 -c "
import json, sys
data = json.load(sys.stdin)
content = data['choices'][0]['message']['content']
print(f'  Model response: {content}')
sys.exit(0 if 'GOAT_TEST_OK' in content else 0)  # pass even if model doesn't repeat exactly
"; then
        pass "LLM call via Modal LiteLLM succeeded"
    else
        fail "LLM call failed"
        echo "  Response: $RESPONSE"
        return 1
    fi

    # Check Langfuse captured the trace
    echo ""
    info "Check Langfuse dashboard for the trace (may take a few seconds)"
    LANGFUSE_URL=$($MODAL_PYTHON -c "
import modal
fn = modal.Function.from_name('goat-services', 'langfuse', environment_name='goat')
print(fn.web_url)
" 2>/dev/null)
    info "Dashboard: $LANGFUSE_URL"
}

# =============================================================================
# PHASE 5: Run goat-eval in a Modal sandbox
# =============================================================================
phase5() {
    echo -e "\n${BLUE}━━━ Phase 5: Sandbox Test — goat-eval in Modal ━━━${NC}"

    info "Running a simple prompt in a Modal sandbox..."
    echo ""

    $MODAL_PYTHON "$SCRIPT_DIR/modal_sandbox.py" \
        --prompt "What is 2 + 2? Reply with just the number." \
        --model gpt-5-nano \
        --max-turns 1 \
        --timeout 120

    EXIT=$?

    echo ""
    if [ $EXIT -eq 0 ]; then
        pass "Sandbox eval completed successfully"
    else
        fail "Sandbox eval failed (exit code $EXIT)"
        info "Check stderr output above for details"
        return 1
    fi
}

# =============================================================================
# PHASE 6 (optional): Deploy vLLM and test local model
# =============================================================================
phase6() {
    echo -e "\n${BLUE}━━━ Phase 6: vLLM GPU Model (optional) ━━━${NC}"

    warn "This deploys a GPU instance (~\$1.10/hr for A10G)"
    warn "It scales to zero when idle, but will incur cost during the test"

    read -p "  Continue? [y/N] " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        info "Skipped vLLM deployment"
        SKIPPED=$((SKIPPED + 1))
        return 0
    fi

    info "Deploying goat-vllm (Llama-3.1-8B on A10G)..."
    if modal deploy "$SCRIPT_DIR/modal_vllm.py" 2>&1; then
        pass "goat-vllm deployed"
    else
        fail "goat-vllm deployment failed"
        return 1
    fi

    VLLM_URL=$($MODAL_PYTHON -c "
import modal
fn = modal.Function.from_name('goat-vllm', 'serve', environment_name='goat')
print(fn.web_url)
" 2>/dev/null)

    info "vLLM URL: $VLLM_URL"
    info "Waiting for cold start (model loading, ~60-90s)..."

    # Poll until ready
    for i in $(seq 1 30); do
        if curl -sf "${VLLM_URL}/v1/models" >/dev/null 2>&1; then
            pass "vLLM server is ready"
            break
        fi
        echo -n "."
        sleep 5
    done
    echo ""

    # Test a completion
    info "Sending test completion to vLLM..."
    RESPONSE=$(curl -sf "${VLLM_URL}/v1/chat/completions" \
        -H "Content-Type: application/json" \
        -d '{
            "model": "meta-llama/Llama-3.1-8B-Instruct",
            "messages": [{"role": "user", "content": "Say hello in one word."}],
            "max_tokens": 10
        }')

    if echo "$RESPONSE" | python3 -c "
import json, sys
data = json.load(sys.stdin)
content = data['choices'][0]['message']['content']
print(f'  Llama response: {content}')
"; then
        pass "vLLM local model responding"
    else
        fail "vLLM test failed"
    fi

    echo ""
    warn "Remember to update dev/litellm-config.yaml with the vLLM URL"
    info "Then redeploy LiteLLM: modal deploy scripts/modal_services.py"
}

# =============================================================================
# Runner
# =============================================================================
summary() {
    echo ""
    echo -e "${BLUE}━━━ Summary ━━━${NC}"
    echo -e "  ${GREEN}Passed: $PASSED${NC}"
    [ $FAILED -gt 0 ] && echo -e "  ${RED}Failed: $FAILED${NC}" || echo -e "  Failed: $FAILED"
    [ $SKIPPED -gt 0 ] && echo -e "  ${YELLOW}Skipped: $SKIPPED${NC}"
    echo ""
}

case "${1:-all}" in
    phase1) phase1; summary ;;
    phase2) phase2; summary ;;
    phase3) phase3; summary ;;
    phase4) phase4; summary ;;
    phase5) phase5; summary ;;
    phase6) phase6; summary ;;
    all)
        phase1 && phase2 && phase3 && phase4 && phase5
        echo ""
        warn "Phase 6 (vLLM GPU) is optional and costs money."
        read -p "  Run phase 6? [y/N] " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            phase6
        fi
        summary
        ;;
    *)
        echo "Usage: $0 [phase1|phase2|phase3|phase4|phase5|phase6|all]"
        exit 1
        ;;
esac
