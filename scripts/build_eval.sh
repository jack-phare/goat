#!/bin/bash
# Cross-compile goat-eval for Linux (Modal sandbox target).
#
# Builds both amd64 and arm64 binaries. Modal typically runs amd64
# but may use arm64 for some GPU instances.
#
# Usage:
#   bash scripts/build_eval.sh           # build both architectures
#   bash scripts/build_eval.sh amd64     # build amd64 only
#   bash scripts/build_eval.sh arm64     # build arm64 only
#
# Output: scripts/goat-eval-linux (amd64), scripts/goat-eval-linux-arm64

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ARCH="${1:-both}"

build_arch() {
    local arch="$1"
    local suffix=""
    if [ "$arch" = "arm64" ]; then
        suffix="-arm64"
    fi
    local output="$SCRIPT_DIR/goat-eval-linux${suffix}"

    echo "Building goat-eval for linux/${arch}..."
    CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -o "$output" "$PROJECT_ROOT/cmd/eval/"

    echo "Built: $output"
    file "$output"
}

case "$ARCH" in
    amd64)
        build_arch amd64
        ;;
    arm64)
        build_arch arm64
        ;;
    both)
        build_arch amd64
        build_arch arm64
        ;;
    *)
        echo "Usage: $0 [amd64|arm64|both]" >&2
        exit 1
        ;;
esac
