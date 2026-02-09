"""SWE-bench Verified benchmark using Goat as the agent."""

from inspect_ai import Task, task
from inspect_evals.swe_bench import swe_bench as swe_bench_task

from goat_solver import goat_agent


@task
def swe_bench() -> Task:
    """SWE-bench Verified with Goat agent."""
    return swe_bench_task(solver=goat_agent())
