#!/bin/bash
# Cross-compile goat-eval for linux/amd64 (Modal sandbox target).
#
# Usage:
#   bash scripts/build_eval.sh
#
# Output: scripts/goat-eval-linux (statically linked ELF binary)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OUTPUT="$SCRIPT_DIR/goat-eval-linux"

echo "Building goat-eval for linux/amd64..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$OUTPUT" "$PROJECT_ROOT/cmd/eval/"

echo "Built: $OUTPUT"
file "$OUTPUT"
