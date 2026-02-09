"""HumanEval benchmark using Goat as the agent."""

from inspect_ai import Task, task
from inspect_evals.humaneval import humaneval as humaneval_task

from goat_solver import goat_agent


@task
def humaneval() -> Task:
    """HumanEval code generation benchmark with Goat agent."""
    return humaneval_task(solver=goat_agent())
