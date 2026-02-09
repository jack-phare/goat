"""Inspect solver that runs Goat's agentic loop inside a Docker sandbox.

Goat calls the LLM provider directly (via OPENAI_BASE_URL env var) rather
than going through inspect's agent bridge. This keeps the architecture simple
and lets us route through LiteLLM or any OpenAI-compatible endpoint.

Required env vars (set on the host, passed through to the sandbox):
  OPENAI_BASE_URL  - LLM endpoint (e.g. http://host.docker.internal:4000/v1)
  OPENAI_API_KEY   - API key for the endpoint
  EVAL_MODEL       - Model ID to use (e.g. gpt-5-nano)
"""

import os
import sys
from pathlib import Path

from inspect_ai.agent import Agent, AgentState, agent
from inspect_ai.model import ChatMessageUser, ContentText
from inspect_ai.util import sandbox

# Pre-built Linux binary path (built via: CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o evals/goat-eval-linux ./cmd/eval/)
_BINARY_PATH = Path(__file__).parent / "goat-eval-linux"
_REMOTE_PATH = "/opt/goat-eval"


def _user_prompt(messages: list) -> str:
    """Extract the user prompt text from the message list."""
    for msg in reversed(messages):
        if isinstance(msg, ChatMessageUser):
            if isinstance(msg.content, str):
                return msg.content
            if isinstance(msg.content, list):
                parts = []
                for block in msg.content:
                    if isinstance(block, ContentText):
                        parts.append(block.text)
                    elif isinstance(block, str):
                        parts.append(block)
                return "\n".join(parts)
    return ""


@agent
def goat_agent(max_turns: int = 100, model: str = "") -> Agent:
    """Run Goat's agentic loop as a subprocess in the sandbox.

    The binary calls the LLM directly via OPENAI_BASE_URL. Set these
    env vars before running inspect eval:

        OPENAI_BASE_URL=http://host.docker.internal:4000/v1
        OPENAI_API_KEY=<litellm-key>
        EVAL_MODEL=gpt-5-nano
    """
    _binary_bytes: bytes | None = None

    async def execute(state: AgentState) -> AgentState:
        nonlocal _binary_bytes
        prompt = _user_prompt(state.messages)

        # Resolve env vars — prefer explicit args, fall back to host env
        base_url = os.environ.get("OPENAI_BASE_URL", "http://host.docker.internal:4000/v1")
        api_key = os.environ.get("OPENAI_API_KEY", "")
        eval_model = model or os.environ.get("EVAL_MODEL", "gpt-5-nano")

        sb = sandbox()

        # Inject the goat-eval binary if it's not already present
        check = await sb.exec(["test", "-x", _REMOTE_PATH])
        if not check.success:
            if _binary_bytes is None:
                _binary_bytes = _BINARY_PATH.read_bytes()
            await sb.write_file(_REMOTE_PATH, _binary_bytes)
            await sb.exec(["chmod", "+x", _REMOTE_PATH])

        result = await sb.exec(
            cmd=[
                _REMOTE_PATH,
                "-prompt",
                prompt,
                "-max-turns",
                str(max_turns),
            ],
            env={
                "OPENAI_BASE_URL": base_url,
                "OPENAI_API_KEY": api_key,
                "EVAL_MODEL": eval_model,
            },
            timeout=600,
        )

        if result.stdout.strip():
            state.output.completion = result.stdout.strip()

        if not result.success:
            print(
                f"goat-eval exit={result.returncode} stderr: {result.stderr[:500]}",
                file=sys.stderr,
            )

        return state

    return execute
