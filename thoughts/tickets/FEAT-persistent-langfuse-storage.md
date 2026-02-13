# FEAT: Persistent Langfuse Storage

**Status**: done
**Priority**: medium
**Component**: `scripts/modal_services.py`

## Description

Langfuse trace data now persists across Modal container restarts using a volume-backed Postgres. Historical traces survive cold starts, deploys, and auto-scaling events.

## Implementation

1. **Modal Volume** (`goat-langfuse-pg`): Mounted at `/pgdata` in the Langfuse function. On first boot, the pre-initialized Postgres data dir from the Docker image is copied to the volume. On subsequent restarts, the existing data is reused directly.

2. **Clean shutdown**: SIGTERM handler runs `pg_ctl stop -m fast` before container teardown, flushing WAL to the volume and avoiding crash recovery on restart.

3. **`langfuse_query()` function**: Lightweight Modal function that mounts the same volume, starts Postgres, and runs arbitrary SQL or `pg_dump`. Used by verification and sync scripts.

## Verification & Tooling

- **`scripts/verify_langfuse_traces.py`**: Fires a test call, then verifies traces via Langfuse REST API (JSON) and raw Postgres SQL.
- **`scripts/langfuse_sync_local.sh`**: Exports `pg_dump` from Modal, restores into local docker-compose Langfuse at `http://localhost:3001`.
- **`scripts/test_modal_infra.sh`**: Phase 3 checks volume exists; Phase 4 auto-verifies traces via API.

## Files

- `scripts/modal_services.py` -- volume mount, shutdown handler, `langfuse_query()`
- `scripts/verify_langfuse_traces.py` -- trace verification (API + SQL)
- `scripts/langfuse_sync_local.sh` -- Modal → local sync
- `scripts/test_modal_infra.sh` -- enhanced validation
- `scripts/Dockerfile.langfuse` -- updated comment (template for volume)
