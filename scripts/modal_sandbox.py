#!/usr/bin/env python
"""Run Goat's eval binary in a Modal sandbox.

Setup:
    uv tool install modal
    modal setup
    bash scripts/build_eval.sh

    # Uses existing 'openai-secret' in agent-dev environment
    # (contains OPENAI_TOKEN_EASTUS2 and OPENAI_URL_EASTUS2)
    # These are mapped to OPENAI_API_KEY and OPENAI_BASE_URL at exec time

Usage:
    # Single prompt
    uv run --with modal python scripts/modal_sandbox.py --prompt "What is 2+2?"

    # With custom model and more turns
    uv run --with modal python scripts/modal_sandbox.py \
        --prompt "Create hello.py that prints hello world" \
        --model gpt-5-nano --max-turns 10

    # Batch mode (see Phase 2)
    uv run --with modal python scripts/modal_sandbox.py --batch prompts.json

    # View results
    uv run --with modal python scripts/modal_results.py
"""

import argparse
import json
import shlex
import sys
import time
from datetime import datetime
from pathlib import Path

import modal

MODAL_ENVIRONMENT = "agent-dev"
BINARY_PATH = Path(__file__).parent / "goat-eval-linux"

app = modal.App.lookup(
    "goat-sandbox", environment_name=MODAL_ENVIRONMENT, create_if_missing=True
)

image = (
    modal.Image.debian_slim(python_version="3.13")
    .apt_install("bash", "ripgrep", "git", "curl")
    .add_local_file(str(BINARY_PATH), "/opt/goat-eval", copy=True)
    .run_commands("chmod +x /opt/goat-eval")
)

llm_secret = modal.Secret.from_name(
    "openai-secret", environment_name=MODAL_ENVIRONMENT
)

# Map openai-secret env vars to what goat-eval expects.
# openai-secret has: OPENAI_TOKEN_EASTUS2, OPENAI_URL_EASTUS2
# goat-eval wants: OPENAI_API_KEY, OPENAI_BASE_URL, EVAL_MODEL
ENV_SETUP = (
    'export OPENAI_API_KEY="${OPENAI_TOKEN_EASTUS2}" '
    'OPENAI_BASE_URL="${OPENAI_URL_EASTUS2}"'
)
results_volume = modal.Volume.from_name(
    "goat-results", environment_name=MODAL_ENVIRONMENT, create_if_missing=True
)


def _shell_quote(s: str) -> str:
    """Shell-escape a string for safe embedding in bash -c commands."""
    return shlex.quote(s)


def run_single(prompt: str, model: str | None, max_turns: int, timeout: int) -> None:
    """Run a single prompt in a Modal sandbox."""
    print(f"Creating Modal sandbox...")
    print(f"  Environment: {MODAL_ENVIRONMENT}")
    print(f"  Model: {model or '(from secret)'}")
    print(f"  Max turns: {max_turns}")
    print(f"  Timeout: {timeout}s")
    print()

    with modal.enable_output():
        sb = modal.Sandbox.create(
            image=image,
            secrets=[llm_secret],
            volumes={"/results": results_volume},
            workdir="/workspace",
            timeout=timeout,
            app=app,
        )

    eval_cmd = f'/opt/goat-eval -prompt {_shell_quote(prompt)} -max-turns {max_turns}'
    env_extra = f' EVAL_MODEL={_shell_quote(model)}' if model else ""
    shell_cmd = f'{ENV_SETUP}{env_extra} && {eval_cmd}'

    print(f"Running: /opt/goat-eval -prompt '{prompt[:80]}...' -max-turns {max_turns}")
    print()

    p = sb.exec("bash", "-c", shell_cmd)
    for line in p.stdout:
        print(line, end="")
    stderr_output = []
    for line in p.stderr:
        stderr_output.append(line)
        print(f"[stderr] {line}", end="")

    exit_code = p.returncode
    print()
    if exit_code != 0:
        print(f"Process exited with code {exit_code}")
        if stderr_output:
            print("Stderr:")
            for line in stderr_output:
                print(f"  {line}", end="")
    print("Sandbox execution complete. Terminating...")
    sb.terminate()


def run_batch(
    batch_file: str, model: str | None, timeout: int, parallel: bool
) -> None:
    """Run a batch of prompts from a JSON file."""
    batch_path = Path(batch_file)
    if not batch_path.exists():
        print(f"Error: batch file not found: {batch_file}", file=sys.stderr)
        sys.exit(1)

    tasks = json.loads(batch_path.read_text())
    if not isinstance(tasks, list):
        print("Error: batch file must contain a JSON array", file=sys.stderr)
        sys.exit(1)

    run_id = datetime.now().strftime("%Y%m%d_%H%M%S")
    results_dir = f"/results/{run_id}"

    print(f"Batch run: {len(tasks)} tasks")
    print(f"  Run ID: {run_id}")
    print(f"  Environment: {MODAL_ENVIRONMENT}")
    print(f"  Model: {model or '(from secret)'}")
    print(f"  Timeout per task: {timeout}s")
    print()

    with modal.enable_output():
        sb = modal.Sandbox.create(
            image=image,
            secrets=[llm_secret],
            volumes={"/results": results_volume},
            workdir="/workspace",
            timeout=timeout * len(tasks),  # Total timeout scales with task count
            app=app,
        )

    # Ensure results directory exists in sandbox
    sb.exec("mkdir", "-p", results_dir)

    results = []
    for i, task in enumerate(tasks):
        task_id = task.get("id", f"task-{i}")
        prompt = task["prompt"]
        max_turns = task.get("max_turns", 10)

        print(f"[{i + 1}/{len(tasks)}] {task_id}: {prompt[:60]}...")

        eval_cmd = f'/opt/goat-eval -prompt {_shell_quote(prompt)} -max-turns {max_turns}'
        env_extra = f' EVAL_MODEL={_shell_quote(model)}' if model else ""
        shell_cmd = f'{ENV_SETUP}{env_extra} && {eval_cmd}'

        start = time.time()
        p = sb.exec("bash", "-c", shell_cmd)

        stdout_lines = []
        for line in p.stdout:
            stdout_lines.append(line)
            print(f"  {line}", end="")
        stderr_lines = []
        for line in p.stderr:
            stderr_lines.append(line)

        elapsed = time.time() - start
        exit_code = p.returncode

        result = {
            "id": task_id,
            "prompt": prompt,
            "output": "".join(stdout_lines).strip(),
            "exit_code": exit_code,
            "stderr": "".join(stderr_lines).strip(),
            "elapsed_s": round(elapsed, 2),
        }
        results.append(result)

        # Write individual result to volume
        result_json = json.dumps(result, indent=2)
        sb.exec(
            "bash",
            "-c",
            f"cat > {results_dir}/{task_id}.json << 'GOAT_EOF'\n{result_json}\nGOAT_EOF",
        )

        status = "OK" if exit_code == 0 else f"FAIL (exit {exit_code})"
        print(f"  -> {status} ({elapsed:.1f}s)")
        print()

    # Write summary
    summary = {
        "run_id": run_id,
        "total_tasks": len(tasks),
        "passed": sum(1 for r in results if r["exit_code"] == 0),
        "failed": sum(1 for r in results if r["exit_code"] != 0),
        "total_elapsed_s": round(sum(r["elapsed_s"] for r in results), 2),
        "results": results,
    }
    summary_json = json.dumps(summary, indent=2)
    sb.exec(
        "bash",
        "-c",
        f"cat > {results_dir}/summary.json << 'GOAT_EOF'\n{summary_json}\nGOAT_EOF",
    )

    # Commit volume writes
    results_volume.commit()

    sb.terminate()

    # Print summary
    print("=" * 60)
    print(f"Batch complete: {summary['passed']}/{summary['total_tasks']} passed")
    print(f"Total time: {summary['total_elapsed_s']}s")
    print(f"Run ID: {run_id}")
    print(
        f"View results: uv run --with modal python scripts/modal_results.py {run_id}"
    )


def main():
    parser = argparse.ArgumentParser(
        description="Run Goat eval binary in a Modal sandbox",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Single prompt
  uv run --with modal python scripts/modal_sandbox.py --prompt "What is 2+2?"

  # Batch mode
  uv run --with modal python scripts/modal_sandbox.py --batch prompts.json

  # Custom model
  uv run --with modal python scripts/modal_sandbox.py --prompt "Hello" --model gpt-5-nano
        """,
    )

    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument("--prompt", help="Single prompt to run")
    group.add_argument(
        "--batch", help="Path to JSON file with batch prompts", metavar="FILE"
    )

    parser.add_argument(
        "--model", default="gpt-5-nano", help="Model ID (default: gpt-5-nano)"
    )
    parser.add_argument(
        "--max-turns",
        type=int,
        default=10,
        help="Max agentic loop turns (default: 10)",
    )
    parser.add_argument(
        "--timeout",
        type=int,
        default=600,
        help="Sandbox timeout in seconds (default: 600)",
    )
    parser.add_argument(
        "--parallel",
        action="store_true",
        help="Run batch tasks in parallel (not yet implemented)",
    )

    args = parser.parse_args()

    if not BINARY_PATH.exists():
        print(
            "Error: goat-eval-linux not found. Run: bash scripts/build_eval.sh",
            file=sys.stderr,
        )
        sys.exit(1)

    if args.prompt:
        run_single(args.prompt, args.model, args.max_turns, args.timeout)
    elif args.batch:
        if args.parallel:
            print(
                "Warning: --parallel not yet implemented, running sequentially",
                file=sys.stderr,
            )
        run_batch(args.batch, args.model, args.timeout, args.parallel)


if __name__ == "__main__":
    main()
