#!/usr/bin/env python
"""View Goat sandbox results from Modal volume.

Usage:
    # List all runs
    uv run --with modal python scripts/modal_results.py

    # View specific run
    uv run --with modal python scripts/modal_results.py 20260209_143000

    # View with full output (no truncation)
    uv run --with modal python scripts/modal_results.py 20260209_143000 --full
"""

import argparse
import json
import sys

import modal

MODAL_ENVIRONMENT = "agent-dev"


def get_volume():
    """Get the goat-results volume."""
    return modal.Volume.from_name("goat-results", environment_name=MODAL_ENVIRONMENT)


def list_runs():
    """List all runs in the volume."""
    volume = get_volume()

    print(f"Results in 'goat-results' volume ({MODAL_ENVIRONMENT}):\n")

    runs = []
    for entry in volume.listdir("/"):
        run_id = entry.path.lstrip("/")
        runs.append(run_id)

    if not runs:
        print("  No runs found.")
        return

    runs.sort(reverse=True)

    for run_id in runs:
        # Try to read summary for extra info
        try:
            data = b""
            for chunk in volume.read_file(f"/{run_id}/summary.json"):
                data += chunk
            summary = json.loads(data)
            passed = summary.get("passed", "?")
            total = summary.get("total_tasks", "?")
            elapsed = summary.get("total_elapsed_s", "?")
            print(f"  {run_id}  ({passed}/{total} passed, {elapsed}s)")
        except Exception:
            print(f"  {run_id}")

    print(f"\nTotal: {len(runs)} runs")
    print(
        f"\nView a run: uv run --with modal python scripts/modal_results.py <run_id>"
    )


def view_run(run_id: str, full: bool = False):
    """View results for a specific run."""
    volume = get_volume()

    # Try to read summary first
    try:
        data = b""
        for chunk in volume.read_file(f"/{run_id}/summary.json"):
            data += chunk
        summary = json.loads(data)
    except Exception:
        summary = None

    if summary:
        print(f"Run: {run_id}")
        print(
            f"Tasks: {summary['total_tasks']} ({summary['passed']} passed, {summary['failed']} failed)"
        )
        print(f"Total time: {summary['total_elapsed_s']}s")
        print()

        for r in summary["results"]:
            status = "PASS" if r["exit_code"] == 0 else f"FAIL (exit {r['exit_code']})"
            print(f"[{status}] {r['id']} ({r['elapsed_s']}s)")
            print(f"  Prompt: {r['prompt'][:100]}")
            output = r["output"]
            if not full and len(output) > 200:
                output = output[:200] + "..."
            print(f"  Output: {output}")
            if r["stderr"]:
                stderr = r["stderr"]
                if not full and len(stderr) > 200:
                    stderr = stderr[:200] + "..."
                print(f"  Stderr: {stderr}")
            print()
    else:
        # Fall back to reading individual result files
        print(f"Run: {run_id}")
        print()
        try:
            for entry in volume.listdir(f"/{run_id}"):
                filename = entry.path.split("/")[-1]
                if not filename.endswith(".json"):
                    continue
                data = b""
                for chunk in volume.read_file(entry.path):
                    data += chunk
                result = json.loads(data)
                print(f"  {filename}:")
                print(f"    {json.dumps(result, indent=2)}")
                print()
        except Exception as e:
            print(f"Error reading run '{run_id}': {e}", file=sys.stderr)
            sys.exit(1)

    if not full:
        print("Use --full to see complete output without truncation")


def main():
    parser = argparse.ArgumentParser(
        description="View Goat sandbox results from Modal volume",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  uv run --with modal python scripts/modal_results.py
  uv run --with modal python scripts/modal_results.py 20260209_143000
  uv run --with modal python scripts/modal_results.py 20260209_143000 --full
        """,
    )
    parser.add_argument(
        "run_id",
        nargs="?",
        help="Run ID to view (omit to list all runs)",
    )
    parser.add_argument(
        "--full",
        action="store_true",
        help="Show full output without truncation",
    )
    args = parser.parse_args()

    if args.run_id:
        view_run(args.run_id, args.full)
    else:
        list_runs()


if __name__ == "__main__":
    main()
