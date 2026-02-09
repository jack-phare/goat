"""Terminal-Bench 2.0 benchmark using Goat as the agent."""

from inspect_ai import Task, task
from inspect_evals.terminal_bench_2 import terminal_bench_2

from goat_solver import goat_agent


@task
def terminal_bench() -> Task:
    """Terminal-Bench 2.0 with Goat agent."""
    t = terminal_bench_2()
    t.solver = goat_agent()
    return t
