#!/usr/bin/env python
"""Run Goat's eval binary in Modal sandboxes with LiteLLM routing.

Each benchmark task runs in its own isolated sandbox (clean filesystem,
no side-effect leakage). Batch mode supports parallel execution.

The sandbox calls the LiteLLM proxy deployed on Modal (scripts/modal_services.py)
for LLM inference. LiteLLM routes to Azure, Groq, OpenAI APIs or local vLLM.

Setup:
    uv tool install modal
    modal setup
    python scripts/modal_setup.py          # create goat environment + secrets
    modal deploy scripts/modal_services.py # deploy LiteLLM + Langfuse + Postgres
    bash scripts/build_eval.sh             # build goat-eval-linux binary

Usage:
    # Single prompt
    python scripts/modal_sandbox.py --prompt "What is 2+2?"

    # Custom model (routed through LiteLLM)
    python scripts/modal_sandbox.py --prompt "Write fizzbuzz" --model gpt-5-mini

    # Batch mode (parallel)
    python scripts/modal_sandbox.py --batch prompts.json --parallel 4

    # Direct API mode (bypass LiteLLM, call provider directly)
    python scripts/modal_sandbox.py --prompt "Hello" --api-url https://api.groq.com/openai/v1

    # View results
    python scripts/modal_results.py
"""

import argparse
import asyncio
import json
import shlex
import sys
import time
from datetime import datetime
from pathlib import Path

try:
    import modal
except ImportError:
    print("Error: modal not installed. Run: uv tool install modal", file=sys.stderr)
    sys.exit(1)

MODAL_ENVIRONMENT = "goat"
BINARY_PATH = Path(__file__).parent / "goat-eval-linux"

# ---------------------------------------------------------------------------
# Modal resources
# ---------------------------------------------------------------------------
sandbox_image = (
    modal.Image.debian_slim(python_version="3.13")
    .apt_install("bash", "ripgrep", "git", "curl")
    .add_local_file(str(BINARY_PATH), "/opt/goat-eval", copy=True)
    .run_commands("chmod +x /opt/goat-eval")
)

litellm_secret = modal.Secret.from_name("goat-litellm", environment_name=MODAL_ENVIRONMENT)
results_volume = modal.Volume.from_name(
    "goat-results", environment_name=MODAL_ENVIRONMENT, create_if_missing=True
)


def _shell_quote(s: str) -> str:
    return shlex.quote(s)


def discover_litellm_url() -> str:
    """Discover the LiteLLM proxy URL from the deployed goat-services app."""
    try:
        # Look up the deployed litellm function's web URL
        fn = modal.Function.from_name("goat-services", "litellm", environment_name=MODAL_ENVIRONMENT)
        url = fn.get_web_url()
        if url:
            return url
    except Exception:
        pass

    print("Warning: Could not discover LiteLLM URL from goat-services.", file=sys.stderr)
    print("  Deploy it first: modal deploy scripts/modal_services.py", file=sys.stderr)
    print("  Falling back to localhost:4000", file=sys.stderr)
    return "http://localhost:4000"


# ---------------------------------------------------------------------------
# Single task runner
# ---------------------------------------------------------------------------
def run_single(
    prompt: str,
    model: str,
    max_turns: int,
    timeout: int,
    api_url: str | None = None,
) -> dict:
    """Run a single prompt in an isolated Modal sandbox. Returns result dict."""
    app = modal.App.lookup(
        "goat-sandbox", environment_name=MODAL_ENVIRONMENT, create_if_missing=True
    )

    # Resolve LLM endpoint
    base_url = api_url or discover_litellm_url()

    print(f"Creating sandbox...")
    print(f"  Environment: {MODAL_ENVIRONMENT}")
    print(f"  Model: {model}")
    print(f"  LLM endpoint: {base_url}")
    print(f"  Max turns: {max_turns}")
    print(f"  Timeout: {timeout}s")
    print()

    sb = modal.Sandbox.create(
        image=sandbox_image,
        secrets=[litellm_secret],
        volumes={"/results": results_volume},
        workdir="/workspace",
        timeout=timeout,
        app=app,
    )

    # Build the command -- env vars set inline, not from secret mapping
    env_cmd = (
        f'export OPENAI_BASE_URL={_shell_quote(base_url + "/v1")} '
        f'OPENAI_API_KEY="$LITELLM_MASTER_KEY" '
        f'EVAL_MODEL={_shell_quote(model)}'
    )
    eval_cmd = f'/opt/goat-eval -prompt {_shell_quote(prompt)} -max-turns {max_turns}'
    shell_cmd = f'{env_cmd} && {eval_cmd}'

    print(f"Running: goat-eval -prompt '{prompt[:80]}...' -max-turns {max_turns}")
    print()

    start = time.time()
    p = sb.exec("bash", "-c", shell_cmd)

    stdout_lines = []
    for line in p.stdout:
        stdout_lines.append(line)
        print(line, end="")

    stderr_lines = []
    for line in p.stderr:
        stderr_lines.append(line)
        print(f"[stderr] {line}", end="", file=sys.stderr)

    p.wait()
    elapsed = time.time() - start
    exit_code = p.returncode

    sb.terminate()

    result = {
        "prompt": prompt,
        "model": model,
        "output": "".join(stdout_lines).strip(),
        "exit_code": exit_code,
        "stderr": "".join(stderr_lines).strip(),
        "elapsed_s": round(elapsed, 2),
    }

    print()
    if exit_code != 0:
        print(f"Process exited with code {exit_code}")
    print(f"Done in {elapsed:.1f}s")

    return result


# ---------------------------------------------------------------------------
# Batch runner with per-task isolation and optional parallelism
# ---------------------------------------------------------------------------
async def run_single_task_async(
    task: dict,
    task_index: int,
    total: int,
    model: str,
    base_url: str,
    timeout: int,
    run_id: str,
    results_dir: str,
) -> dict:
    """Run a single benchmark task in its own sandbox (async)."""
    app = await modal.App.lookup.aio(
        "goat-sandbox", environment_name=MODAL_ENVIRONMENT, create_if_missing=True
    )

    task_id = task.get("id", f"task-{task_index}")
    prompt = task["prompt"]
    max_turns = task.get("max_turns", 10)

    print(f"[{task_index + 1}/{total}] {task_id}: {prompt[:60]}...")

    sb = await modal.Sandbox.create.aio(
        image=sandbox_image,
        secrets=[litellm_secret],
        volumes={"/results": results_volume},
        workdir="/workspace",
        timeout=timeout,
        app=app,
    )

    env_cmd = (
        f'export OPENAI_BASE_URL={_shell_quote(base_url + "/v1")} '
        f'OPENAI_API_KEY="$LITELLM_MASTER_KEY" '
        f'EVAL_MODEL={_shell_quote(model)}'
    )
    eval_cmd = f'/opt/goat-eval -prompt {_shell_quote(prompt)} -max-turns {max_turns}'
    shell_cmd = f'{env_cmd} && {eval_cmd}'

    start = time.time()
    p = await sb.exec.aio("bash", "-c", shell_cmd)

    stdout_lines = []
    async for line in p.stdout:
        stdout_lines.append(line)

    stderr_lines = []
    async for line in p.stderr:
        stderr_lines.append(line)

    await p.wait.aio()
    elapsed = time.time() - start
    exit_code = p.returncode

    result = {
        "id": task_id,
        "prompt": prompt,
        "model": model,
        "output": "".join(stdout_lines).strip(),
        "exit_code": exit_code,
        "stderr": "".join(stderr_lines).strip(),
        "elapsed_s": round(elapsed, 2),
    }

    # Write result to volume via sandbox (safe JSON write using python, not heredoc)
    result_json = json.dumps(result, indent=2)
    write_cmd = (
        f"python3 -c 'import sys; open(sys.argv[1], \"w\").write(sys.stdin.read())' "
        f"{results_dir}/{task_id}.json"
    )
    write_p = await sb.exec.aio("bash", "-c", f"mkdir -p {results_dir} && echo {_shell_quote(result_json)} | {write_cmd}")
    # Drain output
    async for _ in write_p.stdout:
        pass

    await sb.terminate.aio()

    status = "OK" if exit_code == 0 else f"FAIL (exit {exit_code})"
    print(f"  [{task_index + 1}/{total}] {task_id} -> {status} ({elapsed:.1f}s)")

    return result


async def run_batch_async(
    batch_file: str,
    model: str,
    timeout: int,
    max_parallel: int,
    api_url: str | None = None,
) -> None:
    """Run batch of prompts with per-task sandbox isolation and parallelism."""
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

    # Resolve LLM endpoint
    base_url = api_url or discover_litellm_url()

    print(f"Batch run: {len(tasks)} tasks")
    print(f"  Run ID: {run_id}")
    print(f"  Environment: {MODAL_ENVIRONMENT}")
    print(f"  Model: {model}")
    print(f"  LLM endpoint: {base_url}")
    print(f"  Parallelism: {max_parallel}")
    print(f"  Timeout per task: {timeout}s")
    print()

    # Run tasks with bounded parallelism
    semaphore = asyncio.Semaphore(max_parallel)
    total = len(tasks)

    async def run_with_semaphore(task, idx):
        async with semaphore:
            return await run_single_task_async(
                task, idx, total, model, base_url, timeout, run_id, results_dir,
            )

    start_all = time.time()
    results = await asyncio.gather(
        *(run_with_semaphore(task, i) for i, task in enumerate(tasks)),
        return_exceptions=True,
    )

    # Handle exceptions
    clean_results = []
    for i, r in enumerate(results):
        if isinstance(r, Exception):
            task_id = tasks[i].get("id", f"task-{i}")
            print(f"  Error in {task_id}: {r}", file=sys.stderr)
            clean_results.append({
                "id": task_id,
                "prompt": tasks[i]["prompt"],
                "model": model,
                "output": "",
                "exit_code": -1,
                "stderr": str(r),
                "elapsed_s": 0,
            })
        else:
            clean_results.append(r)

    total_elapsed = time.time() - start_all

    # Write summary to volume
    summary = {
        "run_id": run_id,
        "model": model,
        "total_tasks": len(tasks),
        "passed": sum(1 for r in clean_results if r["exit_code"] == 0),
        "failed": sum(1 for r in clean_results if r["exit_code"] != 0),
        "total_elapsed_s": round(total_elapsed, 2),
        "results": clean_results,
    }

    # Write summary via a temporary sandbox
    app = await modal.App.lookup.aio(
        "goat-sandbox", environment_name=MODAL_ENVIRONMENT, create_if_missing=True
    )
    sb = await modal.Sandbox.create.aio(
        image=sandbox_image,
        volumes={"/results": results_volume},
        workdir="/workspace",
        timeout=60,
        app=app,
    )
    summary_json = json.dumps(summary, indent=2)
    write_cmd = (
        f"mkdir -p {results_dir} && "
        f"python3 -c 'import sys; open(sys.argv[1], \"w\").write(sys.stdin.read())' "
        f"{results_dir}/summary.json"
    )
    p = await sb.exec.aio("bash", "-c", f"echo {_shell_quote(summary_json)} | {write_cmd}")
    async for _ in p.stdout:
        pass
    # Note: volume.commit() can only be called inside a Modal container.
    # From the local client, writes via sandbox exec are auto-committed.
    await sb.terminate.aio()

    # Print summary
    print()
    print("=" * 60)
    print(f"Batch complete: {summary['passed']}/{summary['total_tasks']} passed")
    print(f"Total time: {total_elapsed:.1f}s (wall clock)")
    print(f"Run ID: {run_id}")
    print(f"View results: python scripts/modal_results.py {run_id}")


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------
def main():
    parser = argparse.ArgumentParser(
        description="Run Goat eval in isolated Modal sandboxes",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Single prompt (uses LiteLLM on Modal)
  python scripts/modal_sandbox.py --prompt "What is 2+2?"

  # Batch with 4 parallel sandboxes
  python scripts/modal_sandbox.py --batch prompts.json --parallel 4

  # Use a specific model
  python scripts/modal_sandbox.py --prompt "Hello" --model llama-3.1-8b-local

  # Direct API (bypass LiteLLM)
  python scripts/modal_sandbox.py --prompt "Hello" --api-url https://api.groq.com/openai/v1
        """,
    )

    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument("--prompt", help="Single prompt to run")
    group.add_argument("--batch", help="Path to JSON file with batch prompts", metavar="FILE")

    parser.add_argument("--model", default="gpt-5-nano", help="Model ID (default: gpt-5-nano)")
    parser.add_argument("--max-turns", type=int, default=10, help="Max agentic loop turns (default: 10)")
    parser.add_argument("--timeout", type=int, default=600, help="Per-task timeout in seconds (default: 600)")
    parser.add_argument("--parallel", type=int, default=4, help="Max parallel sandboxes for batch (default: 4)")
    parser.add_argument("--api-url", help="Override LLM endpoint URL (bypass LiteLLM discovery)")

    args = parser.parse_args()

    if not BINARY_PATH.exists():
        print(
            "Error: goat-eval-linux not found. Run: bash scripts/build_eval.sh",
            file=sys.stderr,
        )
        sys.exit(1)

    if args.prompt:
        run_single(args.prompt, args.model, args.max_turns, args.timeout, args.api_url)
    elif args.batch:
        asyncio.run(run_batch_async(args.batch, args.model, args.timeout, args.parallel, args.api_url))


if __name__ == "__main__":
    main()
