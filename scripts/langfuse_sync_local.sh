#!/bin/bash
# =============================================================================
# Sync Langfuse trace data from Modal volume to local docker-compose.
#
# Exports a pg_dump from the Modal Langfuse Postgres (stored on the
# goat-langfuse-pg volume) and imports it into the local docker-compose
# Langfuse instance at http://localhost:3001.
#
# Prerequisites:
#   - Modal services deployed: modal deploy scripts/modal_services.py --env goat
#   - Local docker-compose running: docker compose -f dev/docker-compose.yml up -d
#
# Usage:
#   bash scripts/langfuse_sync_local.sh
#   bash scripts/langfuse_sync_local.sh --dump-only   # just download the dump
#   bash scripts/langfuse_sync_local.sh --open         # open browser after sync
# =============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
DUMP_FILE="$PROJECT_ROOT/.langfuse-dump.sql"
MODAL_PYTHON="uv run --with modal python3"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info() { echo -e "${BLUE}>>>${NC} $1"; }
pass() { echo -e "${GREEN}  OK:${NC} $1"; }
fail() { echo -e "${RED}  FAIL:${NC} $1"; }
warn() { echo -e "${YELLOW}  WARN:${NC} $1"; }

DUMP_ONLY=false
OPEN_BROWSER=false
for arg in "$@"; do
    case $arg in
        --dump-only) DUMP_ONLY=true ;;
        --open)      OPEN_BROWSER=true ;;
    esac
done

# ── Step 1: Export pg_dump from Modal ──
info "Exporting Langfuse database from Modal volume..."

$MODAL_PYTHON -c "
import modal, sys

fn = modal.Function.from_name('goat-services', 'langfuse_query', environment_name='goat')
print('  Calling langfuse_query(dump=True)...', file=sys.stderr)
result = fn.remote(dump=True)

if result.startswith('ERROR') or result.startswith('pg_dump error'):
    print(result, file=sys.stderr)
    sys.exit(1)

# Write dump to stdout
print(result, end='')
" > "$DUMP_FILE"

DUMP_SIZE=$(wc -c < "$DUMP_FILE" | tr -d ' ')
if [ "$DUMP_SIZE" -lt 100 ]; then
    fail "Dump file too small (${DUMP_SIZE} bytes). Langfuse DB may be empty."
    echo "  Contents: $(head -5 "$DUMP_FILE")"
    exit 1
fi

pass "Dump exported: $DUMP_FILE ($(du -h "$DUMP_FILE" | cut -f1))"

if $DUMP_ONLY; then
    info "Dump saved to $DUMP_FILE (--dump-only mode)"
    exit 0
fi

# ── Step 2: Ensure local docker-compose is running ──
info "Checking local docker-compose..."

if ! docker ps --format '{{.Names}}' | grep -q goat-postgres; then
    info "Starting local docker-compose..."
    docker compose --env-file "$PROJECT_ROOT/.env" -f "$PROJECT_ROOT/dev/docker-compose.yml" up -d postgres langfuse
    info "Waiting for Postgres to be healthy..."
    sleep 5
fi

# Wait for postgres to be ready
for i in $(seq 1 15); do
    if docker exec goat-postgres pg_isready -U goat >/dev/null 2>&1; then
        pass "Local Postgres is ready"
        break
    fi
    sleep 1
done

# ── Step 3: Drop and recreate the local langfuse database, then restore ──
info "Importing dump into local Postgres..."

# Terminate existing connections to langfuse DB
docker exec goat-postgres psql -U goat -d goat -c \
    "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='langfuse' AND pid <> pg_backend_pid();" \
    >/dev/null 2>&1 || true

# Drop and recreate
docker exec goat-postgres psql -U goat -d goat -c "DROP DATABASE IF EXISTS langfuse;" >/dev/null 2>&1
docker exec goat-postgres psql -U goat -d goat -c "CREATE DATABASE langfuse OWNER goat;" >/dev/null 2>&1

# Restore the dump
docker exec -i goat-postgres psql -U goat -d langfuse < "$DUMP_FILE" >/dev/null 2>&1
RESTORE_EXIT=$?

if [ $RESTORE_EXIT -eq 0 ]; then
    pass "Database restored successfully"
else
    # psql restore often exits non-zero due to harmless warnings (roles, etc.)
    warn "Restore completed with warnings (exit $RESTORE_EXIT) -- usually harmless"
fi

# ── Step 4: Restart Langfuse to pick up new data ──
info "Restarting local Langfuse container..."
docker restart goat-langfuse >/dev/null 2>&1

# Wait for Langfuse to be healthy
for i in $(seq 1 20); do
    if docker exec goat-langfuse wget -qO- http://127.0.0.1:3000/api/public/health >/dev/null 2>&1; then
        pass "Local Langfuse is healthy"
        break
    fi
    sleep 2
done

# ── Step 5: Print summary ──
echo ""
info "Sync complete!"
echo ""
echo "  Local Langfuse UI:  http://localhost:3001"
echo "  Login:              admin@goat.local / admin1234"
echo "  Dump file:          $DUMP_FILE"
echo ""

# Show trace count
TRACE_COUNT=$(docker exec goat-postgres psql -U goat -d langfuse -tAc "SELECT count(*) FROM traces;" 2>/dev/null || echo "?")
OBS_COUNT=$(docker exec goat-postgres psql -U goat -d langfuse -tAc "SELECT count(*) FROM observations;" 2>/dev/null || echo "?")
echo "  Traces:        $TRACE_COUNT"
echo "  Observations:  $OBS_COUNT"
echo ""

if $OPEN_BROWSER; then
    if command -v open &>/dev/null; then
        open "http://localhost:3001"
    elif command -v xdg-open &>/dev/null; then
        xdg-open "http://localhost:3001"
    fi
fi
