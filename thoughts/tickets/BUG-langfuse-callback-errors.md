# BUG: Langfuse Callback Errors on Modal

**Status**: fixed
**Priority**: medium
**Component**: `scripts/modal_services.py`

## Description

LiteLLM's Langfuse callbacks produce intermittent "Unexpected error occurred" errors during benchmark runs. The LLM inference itself succeeds (200 OK) but trace data is not reliably reaching Langfuse.

## Symptoms

```
Unexpected error occurred. Please check your request and contact support: https://langfuse.com/support.
POST /v1/chat/completions -> 200 OK
```

The error appears in Modal LiteLLM logs during benchmark runs. It is non-blocking -- inference pipeline works fine.

## Root Cause

Two issues combined to cause the failures:

1. **Ephemeral Postgres**: Postgres was co-located inside the Langfuse container with ephemeral storage. On container restarts (Modal auto-scaling / cold starts), Postgres data was lost and Prisma migrations had to re-run.

2. **Wrong internal URL**: LiteLLM was configured with `LANGFUSE_HOST=http://goat-services-langfuse.modal.internal:3000`, but Modal's `@modal.web_server` proxies on port 80, not the container's internal port. The readiness check always timed out, and callbacks silently failed.

## Fix Applied

Three changes in `scripts/modal_services.py`:

1. **Persistent Postgres via Modal Volume** (`goat-langfuse-pg`): Postgres data dir is
   now stored on a volume. On first deploy the pre-built template from the Docker image
   is copied to the volume; on subsequent restarts the existing data is reused, making
   Prisma migrations a near-instant no-op.

2. **Correct internal URL**: Changed `LANGFUSE_HOST` to
   `http://goat-services-langfuse.modal.internal` (no port) with public URL fallback.
   Modal proxies `@modal.web_server` through port 80 internally.

3. **Langfuse readiness check in LiteLLM startup**: Before starting the proxy, the
   `litellm()` function polls Langfuse `/api/public/health` via internal then public URL.
   This prevents callbacks from hitting a still-booting Langfuse instance during
   simultaneous container starts (e.g. fresh deploys).

## Files

- `scripts/modal_services.py` -- volume mount, URL fix, readiness check
- `scripts/verify_langfuse_traces.py` -- verification script
- `scripts/Dockerfile.langfuse` -- updated comment
