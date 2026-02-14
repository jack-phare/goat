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

    # Skill-augmented eval (loads skills into sandbox)
    python scripts/modal_sandbox.py --prompt "Write a Go function" --skills-dir eval/skills

    # A/B comparison: run each task with and without skills
    python scripts/modal_sandbox.py --batch eval/benchmark_skills.json --skills-dir eval/skills --ab

    # MCP-augmented eval (mounts MCP server config into sandbox)
    python scripts/modal_sandbox.py --prompt "List files" --mcp-config eval/mcp_configs/filesystem.json

    # A/B with skills + MCP (3-way comparison)
    python scripts/modal_sandbox.py --batch eval/benchmark_skills.json --skills-dir eval/skills --mcp-config eval/mcp_configs/filesystem.json --ab

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
    .apt_install("bash", "ripgrep", "git", "curl", "nodejs", "npm")
    .add_local_file(str(BINARY_PATH), "/opt/goat-eval", copy=True)
    .run_commands("chmod +x /opt/goat-eval")
)

litellm_secret = modal.Secret.from_name("goat-litellm", environment_name=MODAL_ENVIRONMENT)
results_volume = modal.Volume.from_name(
    "goat-results", environment_name=MODAL_ENVIRONMENT, create_if_missing=True
)


def build_sandbox_image(
    skills_dir: str | None = None,
    mcp_config: str | None = None,
) -> modal.Image:
    """Build the sandbox image, optionally including skills and/or MCP config.

    When skills_dir is provided, the directory is copied into /opt/skills/.
    When mcp_config is provided, the JSON file is copied to /opt/mcp-config.json.
    """
    img = sandbox_image
    if skills_dir:
        skills_path = Path(skills_dir).resolve()
        if not skills_path.exists():
            print(f"Error: skills directory not found: {skills_dir}", file=sys.stderr)
            sys.exit(1)
        img = img.add_local_dir(str(skills_path), remote_path="/opt/skills")
    if mcp_config:
        mcp_path = Path(mcp_config).resolve()
        if not mcp_path.exists():
            print(f"Error: MCP config not found: {mcp_config}", file=sys.stderr)
            sys.exit(1)
        img = img.add_local_file(str(mcp_path), "/opt/mcp-config.json", copy=True)
    return img


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
    skills_dir: str | None = None,
    mcp_config: str | None = None,
) -> dict:
    """Run a single prompt in an isolated Modal sandbox. Returns result dict."""
    app = modal.App.lookup(
        "goat-sandbox", environment_name=MODAL_ENVIRONMENT, create_if_missing=True
    )

    image = build_sandbox_image(skills_dir, mcp_config)

    # Resolve LLM endpoint
    base_url = api_url or discover_litellm_url()

    print(f"Creating sandbox...")
    print(f"  Environment: {MODAL_ENVIRONMENT}")
    print(f"  Model: {model}")
    print(f"  LLM endpoint: {base_url}")
    print(f"  Max turns: {max_turns}")
    print(f"  Timeout: {timeout}s")
    if skills_dir:
        print(f"  Skills: {skills_dir}")
    if mcp_config:
        print(f"  MCP config: {mcp_config}")
    print()

    sb = modal.Sandbox.create(
        image=image,
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
    skills_flag = " -skills-dir /opt/skills" if skills_dir else ""
    mcp_flag = " -mcp-config /opt/mcp-config.json" if mcp_config else ""
    eval_cmd = f'/opt/goat-eval -prompt {_shell_quote(prompt)} -max-turns {max_turns}{skills_flag}{mcp_flag}'
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
        "skills_enabled": bool(skills_dir),
        "mcp_enabled": bool(mcp_config),
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
    image: modal.Image | None = None,
    skills_enabled: bool = False,
    mcp_enabled: bool = False,
) -> dict:
    """Run a single benchmark task in its own sandbox (async)."""
    app = await modal.App.lookup.aio(
        "goat-sandbox", environment_name=MODAL_ENVIRONMENT, create_if_missing=True
    )

    task_id = task.get("id", f"task-{task_index}")
    prompt = task["prompt"]
    max_turns = task.get("max_turns", 10)
    variant_parts = []
    if skills_enabled:
        variant_parts.append("skills")
    if mcp_enabled:
        variant_parts.append("mcp")
    variant = f" [+{'+'.join(variant_parts)}]" if variant_parts else ""

    print(f"[{task_index + 1}/{total}] {task_id}{variant}: {prompt[:60]}...")

    sb = await modal.Sandbox.create.aio(
        image=image or sandbox_image,
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
    skills_flag = " -skills-dir /opt/skills" if skills_enabled else ""
    mcp_flag = " -mcp-config /opt/mcp-config.json" if mcp_enabled else ""
    eval_cmd = f'/opt/goat-eval -prompt {_shell_quote(prompt)} -max-turns {max_turns}{skills_flag}{mcp_flag}'
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

    # Build result suffix for A/B runs
    if mcp_enabled and skills_enabled:
        result_suffix = "-skills-mcp"
    elif skills_enabled:
        result_suffix = "-skills"
    elif mcp_enabled:
        result_suffix = "-mcp"
    else:
        result_suffix = "-baseline"
    result = {
        "id": task_id,
        "prompt": prompt,
        "model": model,
        "skills_enabled": skills_enabled,
        "mcp_enabled": mcp_enabled,
        "output": "".join(stdout_lines).strip(),
        "exit_code": exit_code,
        "stderr": "".join(stderr_lines).strip(),
        "elapsed_s": round(elapsed, 2),
    }

    # Write result to volume via sandbox (safe JSON write using python, not heredoc)
    result_json = json.dumps(result, indent=2)
    result_filename = f"{task_id}{result_suffix}.json"
    write_cmd = (
        f"python3 -c 'import sys; open(sys.argv[1], \"w\").write(sys.stdin.read())' "
        f"{results_dir}/{result_filename}"
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
    skills_dir: str | None = None,
    mcp_config: str | None = None,
    ab_mode: bool = False,
) -> None:
    """Run batch of prompts with per-task sandbox isolation and parallelism.

    When ab_mode is True, each task runs in multiple variants for comparison:
    baseline, +skills (if skills_dir), +skills+mcp (if mcp_config). Results
    are stored with variant suffixes for paired comparison.
    """
    batch_path = Path(batch_file)
    if not batch_path.exists():
        print(f"Error: batch file not found: {batch_file}", file=sys.stderr)
        sys.exit(1)

    tasks = json.loads(batch_path.read_text())
    if not isinstance(tasks, list):
        print("Error: batch file must contain a JSON array", file=sys.stderr)
        sys.exit(1)

    if ab_mode and not skills_dir and not mcp_config:
        print("Error: --ab requires --skills-dir and/or --mcp-config", file=sys.stderr)
        sys.exit(1)

    # Build images for each variant
    baseline_image = sandbox_image
    skills_image = build_sandbox_image(skills_dir=skills_dir) if skills_dir else None
    mcp_image = build_sandbox_image(skills_dir=skills_dir, mcp_config=mcp_config) if mcp_config else None

    run_id = datetime.now().strftime("%Y%m%d_%H%M%S")
    results_dir = f"/results/{run_id}"

    # Resolve LLM endpoint
    base_url = api_url or discover_litellm_url()

    mode_label = "A/B" if ab_mode else ("skills+mcp" if skills_dir and mcp_config else "skills" if skills_dir else "mcp" if mcp_config else "baseline")
    print(f"Batch run: {len(tasks)} tasks ({mode_label})")
    print(f"  Run ID: {run_id}")
    print(f"  Environment: {MODAL_ENVIRONMENT}")
    print(f"  Model: {model}")
    print(f"  LLM endpoint: {base_url}")
    print(f"  Parallelism: {max_parallel}")
    print(f"  Timeout per task: {timeout}s")
    if skills_dir:
        print(f"  Skills: {skills_dir}")
    if mcp_config:
        print(f"  MCP config: {mcp_config}")
    if ab_mode:
        print(f"  Mode: A/B (each task runs with and without augmentations)")
    print()

    # Build the list of (task, image, skills_enabled, mcp_enabled) tuples to run
    runs: list[tuple[dict, modal.Image, bool, bool]] = []
    if ab_mode:
        for task in tasks:
            # Always include baseline
            runs.append((task, baseline_image, False, False))
            if skills_dir:
                runs.append((task, skills_image, True, False))
            if mcp_config:
                runs.append((task, mcp_image, bool(skills_dir), True))
    elif mcp_config:
        for task in tasks:
            runs.append((task, mcp_image, bool(skills_dir), True))
    elif skills_dir:
        for task in tasks:
            runs.append((task, skills_image, True, False))
    else:
        for task in tasks:
            runs.append((task, baseline_image, False, False))

    # Run tasks with bounded parallelism
    semaphore = asyncio.Semaphore(max_parallel)
    total = len(runs)

    async def run_with_semaphore(run_tuple, idx):
        task, image, skills_on, mcp_on = run_tuple
        async with semaphore:
            return await run_single_task_async(
                task, idx, total, model, base_url, timeout, run_id, results_dir,
                image=image, skills_enabled=skills_on, mcp_enabled=mcp_on,
            )

    start_all = time.time()
    results = await asyncio.gather(
        *(run_with_semaphore(run, i) for i, run in enumerate(runs)),
        return_exceptions=True,
    )

    # Handle exceptions
    clean_results = []
    for i, r in enumerate(results):
        if isinstance(r, Exception):
            task, _, skills_on, mcp_on = runs[i]
            task_id = task.get("id", f"task-{i}")
            print(f"  Error in {task_id}: {r}", file=sys.stderr)
            clean_results.append({
                "id": task_id,
                "prompt": task["prompt"],
                "model": model,
                "skills_enabled": skills_on,
                "mcp_enabled": mcp_on,
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
        "mode": mode_label,
        "skills_dir": skills_dir,
        "mcp_config": mcp_config,
        "total_tasks": len(tasks),
        "total_runs": len(runs),
        "passed": sum(1 for r in clean_results if r["exit_code"] == 0),
        "failed": sum(1 for r in clean_results if r["exit_code"] != 0),
        "total_elapsed_s": round(total_elapsed, 2),
        "results": clean_results,
    }

    # In A/B mode, add a paired comparison section
    if ab_mode:
        # Determine the number of variants per task
        variants_per_task = 1  # baseline
        if skills_dir:
            variants_per_task += 1
        if mcp_config:
            variants_per_task += 1

        comparisons = []
        for i in range(0, len(clean_results), variants_per_task):
            group = clean_results[i:i + variants_per_task]
            if len(group) < variants_per_task:
                break
            comparison = {"id": group[0]["id"]}
            for r in group:
                if not r["skills_enabled"] and not r["mcp_enabled"]:
                    comparison["baseline_pass"] = r["exit_code"] == 0
                    comparison["baseline_time_s"] = r["elapsed_s"]
                elif r["skills_enabled"] and not r["mcp_enabled"]:
                    comparison["skills_pass"] = r["exit_code"] == 0
                    comparison["skills_time_s"] = r["elapsed_s"]
                elif r["mcp_enabled"]:
                    comparison["mcp_pass"] = r["exit_code"] == 0
                    comparison["mcp_time_s"] = r["elapsed_s"]
            comparisons.append(comparison)

        summary["comparisons"] = comparisons
        baseline_pass = sum(1 for c in comparisons if c.get("baseline_pass"))
        summary["baseline_pass_rate"] = f"{baseline_pass}/{len(comparisons)}"
        if skills_dir:
            skills_pass = sum(1 for c in comparisons if c.get("skills_pass"))
            summary["skills_pass_rate"] = f"{skills_pass}/{len(comparisons)}"
        if mcp_config:
            mcp_pass = sum(1 for c in comparisons if c.get("mcp_pass"))
            summary["mcp_pass_rate"] = f"{mcp_pass}/{len(comparisons)}"

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
    if ab_mode:
        print(f"A/B Comparison complete ({len(tasks)} tasks, {len(runs)} runs)")
        print(f"  Baseline:    {summary['baseline_pass_rate']} passed")
        if skills_dir:
            print(f"  +Skills:     {summary['skills_pass_rate']} passed")
        if mcp_config:
            print(f"  +Skills+MCP: {summary['mcp_pass_rate']} passed")
    else:
        print(f"Batch complete: {summary['passed']}/{summary['total_runs']} passed")
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

  # Skill-augmented eval
  python scripts/modal_sandbox.py --prompt "Write Go code" --skills-dir eval/skills

  # A/B comparison (each task runs with and without skills)
  python scripts/modal_sandbox.py --batch eval/benchmark_skills.json --skills-dir eval/skills --ab

  # MCP-augmented eval
  python scripts/modal_sandbox.py --prompt "List files" --mcp-config eval/mcp_configs/filesystem.json

  # A/B with skills + MCP (3-way comparison)
  python scripts/modal_sandbox.py --batch tasks.json --skills-dir eval/skills --mcp-config eval/mcp_configs/filesystem.json --ab

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
    parser.add_argument("--skills-dir", help="Path to skills directory (mounted into sandbox)")
    parser.add_argument("--mcp-config", help="Path to MCP server config JSON file (mounted into sandbox)")
    parser.add_argument(
        "--ab", action="store_true",
        help="A/B mode: run each task with and without augmentations (requires --skills-dir and/or --mcp-config)",
    )

    args = parser.parse_args()

    if not BINARY_PATH.exists():
        print(
            "Error: goat-eval-linux not found. Run: bash scripts/build_eval.sh",
            file=sys.stderr,
        )
        sys.exit(1)

    if args.ab and not args.skills_dir and not args.mcp_config:
        parser.error("--ab requires --skills-dir and/or --mcp-config")

    if args.prompt:
        run_single(
            args.prompt, args.model, args.max_turns, args.timeout,
            api_url=args.api_url, skills_dir=args.skills_dir,
            mcp_config=args.mcp_config,
        )
    elif args.batch:
        asyncio.run(run_batch_async(
            args.batch, args.model, args.timeout, args.parallel,
            api_url=args.api_url, skills_dir=args.skills_dir,
            mcp_config=args.mcp_config, ab_mode=args.ab,
        ))


if __name__ == "__main__":
    main()
