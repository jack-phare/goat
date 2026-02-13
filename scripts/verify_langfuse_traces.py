#!/usr/bin/env python
"""Verify that Langfuse traces are being captured for Modal LLM calls.

Fires a test chat completion through the Modal LiteLLM proxy, waits for the
async Langfuse callback, then verifies the trace arrived via two methods:

  A) Langfuse REST API  -- shows traces as structured JSON
  B) Raw Postgres SQL   -- shows the actual database rows on the volume

Usage:
    # Full round-trip: fire a call to qwen3-4b-local and verify the trace
    uv run --with modal,requests python scripts/verify_langfuse_traces.py

    # Use a different model
    uv run --with modal,requests python scripts/verify_langfuse_traces.py --model llama-3.1-8b-local

    # Skip the test call, just query existing traces
    uv run --with modal,requests python scripts/verify_langfuse_traces.py --query-only

    # Also show raw Postgres SQL output
    uv run --with modal,requests python scripts/verify_langfuse_traces.py --sql

    # Custom API keys (if different from modal_setup.py defaults)
    uv run --with modal,requests python scripts/verify_langfuse_traces.py \
        --public-key pk-lf-custom --secret-key sk-lf-custom
"""

import argparse
import json
import sys
import time

try:
    import modal
except ImportError:
    print("Error: modal not installed. Run: uv tool install modal", file=sys.stderr)
    sys.exit(1)

try:
    import requests
except ImportError:
    print("Error: requests not installed. Run: pip install requests", file=sys.stderr)
    sys.exit(1)


MODAL_ENVIRONMENT = "goat"

# Default Langfuse API keys from modal_setup.py LANGFUSE_DEFAULTS
DEFAULT_PUBLIC_KEY = "pk-lf-goat-modal"
DEFAULT_SECRET_KEY = "sk-lf-goat-modal"


def discover_urls() -> tuple[str, str]:
    """Discover LiteLLM and Langfuse URLs from deployed Modal functions."""
    litellm_url = ""
    langfuse_url = ""

    try:
        fn = modal.Function.from_name(
            "goat-services", "litellm", environment_name=MODAL_ENVIRONMENT
        )
        litellm_url = fn.get_web_url() or ""
    except Exception as e:
        print(f"Warning: Could not discover LiteLLM URL: {e}", file=sys.stderr)

    try:
        fn = modal.Function.from_name(
            "goat-services", "langfuse", environment_name=MODAL_ENVIRONMENT
        )
        langfuse_url = fn.get_web_url() or ""
    except Exception as e:
        print(f"Warning: Could not discover Langfuse URL: {e}", file=sys.stderr)

    return litellm_url, langfuse_url


def fire_test_call(
    litellm_url: str, litellm_key: str, model: str
) -> dict | None:
    """Send a simple chat completion through LiteLLM and return the response."""
    print(f"\n{'='*60}")
    print(f"Firing test call: {model}")
    print(f"{'='*60}")
    print(f"  LiteLLM: {litellm_url}")
    print(f"  Model:   {model}")

    try:
        resp = requests.post(
            f"{litellm_url}/v1/chat/completions",
            headers={
                "Authorization": f"Bearer {litellm_key}",
                "Content-Type": "application/json",
            },
            json={
                "model": model,
                "messages": [
                    {"role": "user", "content": "What is 2+2? Reply with just the number."}
                ],
                "max_tokens": 20,
            },
            timeout=120,
        )
        resp.raise_for_status()
        data = resp.json()
        content = data["choices"][0]["message"]["content"]
        usage = data.get("usage", {})
        print(f"  Response: {content.strip()}")
        print(
            f"  Tokens: {usage.get('prompt_tokens', '?')} in / "
            f"{usage.get('completion_tokens', '?')} out"
        )
        return data
    except Exception as e:
        print(f"  ERROR: {e}", file=sys.stderr)
        return None


def verify_via_api(
    langfuse_url: str, public_key: str, secret_key: str, model: str | None = None
) -> bool:
    """Query Langfuse REST API for recent traces."""
    print(f"\n{'='*60}")
    print("Method A: Langfuse REST API")
    print(f"{'='*60}")
    print(f"  Endpoint: {langfuse_url}/api/public/traces")

    try:
        params = {"limit": 5, "orderBy": "timestamp.DESC"}
        resp = requests.get(
            f"{langfuse_url}/api/public/traces",
            auth=(public_key, secret_key),
            params=params,
            timeout=15,
        )
        resp.raise_for_status()
        data = resp.json()

        traces = data.get("data", [])
        if not traces:
            print("  No traces found.")
            return False

        print(f"  Found {len(traces)} recent trace(s):\n")
        for t in traces:
            trace_id = t.get("id", "?")[:16]
            name = t.get("name", "?")
            ts = t.get("timestamp", "?")
            metadata = t.get("metadata", {}) or {}
            trace_model = metadata.get("model_id", metadata.get("model", "?"))
            inp = t.get("input", "")
            out = t.get("output", "")

            # Truncate for display
            inp_str = json.dumps(inp)[:120] if inp else "(empty)"
            out_str = json.dumps(out)[:120] if out else "(empty)"

            print(f"  Trace {trace_id}...")
            print(f"    name:      {name}")
            print(f"    model:     {trace_model}")
            print(f"    timestamp: {ts}")
            print(f"    input:     {inp_str}")
            print(f"    output:    {out_str}")
            print()

        # Also fetch observations for the most recent trace
        latest_trace_id = traces[0].get("id")
        if latest_trace_id:
            _show_observations(langfuse_url, public_key, secret_key, latest_trace_id)

        return True

    except requests.exceptions.HTTPError as e:
        print(f"  HTTP Error: {e}")
        print(f"  Response: {e.response.text[:300] if e.response else 'N/A'}")
        return False
    except Exception as e:
        print(f"  ERROR: {e}", file=sys.stderr)
        return False


def _show_observations(
    langfuse_url: str, public_key: str, secret_key: str, trace_id: str
):
    """Show observations (generations) for a specific trace."""
    try:
        resp = requests.get(
            f"{langfuse_url}/api/public/observations",
            auth=(public_key, secret_key),
            params={"traceId": trace_id, "limit": 5},
            timeout=15,
        )
        resp.raise_for_status()
        obs_list = resp.json().get("data", [])
        if not obs_list:
            return

        print(f"  Observations for trace {trace_id[:16]}...:")
        for o in obs_list:
            obs_type = o.get("type", "?")
            obs_model = o.get("model", "?")
            start = o.get("startTime", "?")
            end = o.get("endTime", "?")
            usage = o.get("usage", {}) or {}
            prompt_t = usage.get("promptTokens", usage.get("input", "?"))
            completion_t = usage.get("completionTokens", usage.get("output", "?"))
            total_t = usage.get("totalTokens", usage.get("total", "?"))
            cost = o.get("calculatedTotalCost")

            print(f"    [{obs_type}] model={obs_model}")
            print(f"      time:   {start} → {end}")
            print(f"      tokens: {prompt_t} in / {completion_t} out / {total_t} total")
            if cost is not None:
                print(f"      cost:   ${cost:.6f}")
            print()
    except Exception:
        pass  # Non-critical, don't fail


def verify_via_sql(model: str | None = None) -> bool:
    """Query Postgres directly via the langfuse_query() Modal function."""
    print(f"\n{'='*60}")
    print("Method B: Raw Postgres SQL (via Modal volume)")
    print(f"{'='*60}")

    try:
        fn = modal.Function.from_name(
            "goat-services", "langfuse_query", environment_name=MODAL_ENVIRONMENT
        )
    except Exception as e:
        print(f"  ERROR: Could not find langfuse_query function: {e}")
        print("  Deploy first: modal deploy scripts/modal_services.py --env goat")
        return False

    # Recent traces
    print("\n  === Recent traces ===\n")
    sql_traces = """
SELECT id, name, timestamp, metadata->>'model_id' AS model
FROM traces
ORDER BY timestamp DESC
LIMIT 5;
"""
    try:
        result = fn.remote(sql=sql_traces)
        print(result)
    except Exception as e:
        print(f"  ERROR querying traces: {e}")
        return False

    # Recent observations (generations)
    print("  === Recent observations (generations) ===\n")
    sql_obs = """
SELECT id, type, name, model, start_time, end_time,
       prompt_tokens, completion_tokens, total_tokens
FROM observations
ORDER BY start_time DESC
LIMIT 5;
"""
    try:
        result = fn.remote(sql=sql_obs)
        print(result)
    except Exception as e:
        print(f"  ERROR querying observations: {e}")
        return False

    # Summary stats
    print("  === Database summary ===\n")
    sql_stats = """
SELECT
    (SELECT count(*) FROM traces) AS total_traces,
    (SELECT count(*) FROM observations) AS total_observations,
    (SELECT min(timestamp) FROM traces) AS earliest_trace,
    (SELECT max(timestamp) FROM traces) AS latest_trace;
"""
    try:
        result = fn.remote(sql=sql_stats)
        print(result)
    except Exception as e:
        print(f"  ERROR: {e}")

    return True


def main():
    parser = argparse.ArgumentParser(
        description="Verify Langfuse traces from Modal LLM calls",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument(
        "--model",
        default="qwen3-4b-local",
        help="Model to test (default: qwen3-4b-local)",
    )
    parser.add_argument(
        "--query-only",
        action="store_true",
        help="Skip the test call, just query existing traces",
    )
    parser.add_argument(
        "--sql",
        action="store_true",
        help="Also show raw Postgres SQL output (Method B)",
    )
    parser.add_argument(
        "--public-key",
        default=DEFAULT_PUBLIC_KEY,
        help=f"Langfuse public key (default: {DEFAULT_PUBLIC_KEY})",
    )
    parser.add_argument(
        "--secret-key",
        default=DEFAULT_SECRET_KEY,
        help=f"Langfuse secret key (default: {DEFAULT_SECRET_KEY})",
    )
    parser.add_argument(
        "--litellm-key",
        default=None,
        help="LiteLLM master key (reads from .env if not provided)",
    )
    args = parser.parse_args()

    # Discover URLs
    print("Discovering Modal service URLs...")
    litellm_url, langfuse_url = discover_urls()

    if not langfuse_url:
        print("ERROR: Could not discover Langfuse URL. Is goat-services deployed?")
        sys.exit(1)

    # Resolve LiteLLM key
    litellm_key = args.litellm_key
    if not litellm_key:
        from pathlib import Path

        env_path = Path(__file__).parent.parent / ".env"
        if env_path.exists():
            for line in env_path.read_text().splitlines():
                if line.startswith("LITELLM_MASTER_KEY="):
                    litellm_key = line.split("=", 1)[1].strip()
                    break
    if not litellm_key:
        print("ERROR: No LiteLLM key. Set --litellm-key or add to .env")
        sys.exit(1)

    # Step 1: Fire a test call (unless --query-only)
    if not args.query_only:
        if not litellm_url:
            print("ERROR: Could not discover LiteLLM URL for test call.")
            sys.exit(1)
        result = fire_test_call(litellm_url, litellm_key, args.model)
        if not result:
            print("\nTest call failed. Cannot verify traces.")
            sys.exit(1)

        # Wait for async callback to reach Langfuse
        print("\nWaiting 5s for Langfuse callback...")
        time.sleep(5)

    # Step 2: Verify via Langfuse REST API
    api_ok = verify_via_api(
        langfuse_url, args.public_key, args.secret_key, args.model
    )

    # Step 3: Verify via raw SQL (if requested or if API found traces)
    sql_ok = False
    if args.sql:
        sql_ok = verify_via_sql(args.model)

    # Summary
    print(f"\n{'='*60}")
    print("Summary")
    print(f"{'='*60}")
    print(f"  Langfuse API traces: {'PASS' if api_ok else 'FAIL'}")
    if args.sql:
        print(f"  Postgres SQL query:  {'PASS' if sql_ok else 'FAIL'}")
    print(f"  Langfuse UI:         {langfuse_url}")
    print()

    if not api_ok:
        sys.exit(1)


if __name__ == "__main__":
    main()
