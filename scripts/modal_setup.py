#!/usr/bin/env python
"""Interactive setup for Goat's Modal environment and secrets.

Creates the 'goat' Modal environment and provisions all required secrets.
Every secret creation/update requires explicit user confirmation.

Usage:
    uv run --with modal python scripts/modal_setup.py
    uv run --with modal python scripts/modal_setup.py --env-file .env
    uv run --with modal python scripts/modal_setup.py --dry-run
"""

import argparse
import os
import secrets
import sys
from pathlib import Path

try:
    import modal
except ImportError:
    print("Error: modal not installed. Run: uv tool install modal", file=sys.stderr)
    sys.exit(1)

MODAL_ENVIRONMENT = "goat"

# Secrets schema: name -> list of (key, description, required)
SECRETS_SCHEMA = {
    "goat-llm-providers": [
        ("AZURE_API_KEY", "Azure OpenAI API key", True),
        ("AZURE_API_BASE", "Azure OpenAI base URL (e.g. https://xxx.openai.azure.com)", True),
        ("GROQ_API_KEY", "Groq API key", False),
        ("OPENAI_API_KEY", "OpenAI API key (direct, not Azure)", False),
        ("ANTHROPIC_API_KEY", "Anthropic API key", False),
    ],
    "goat-litellm": [
        ("LITELLM_MASTER_KEY", "LiteLLM proxy master key", True),
    ],
    "goat-langfuse": [
        ("LANGFUSE_NEXTAUTH_SECRET", "Langfuse NextAuth secret (auto-generated if empty)", False),
        ("LANGFUSE_SALT", "Langfuse salt (auto-generated if empty)", False),
        ("LANGFUSE_INIT_ORG_ID", "Langfuse org ID", False),
        ("LANGFUSE_INIT_ORG_NAME", "Langfuse org name", False),
        ("LANGFUSE_INIT_PROJECT_ID", "Langfuse project ID", False),
        ("LANGFUSE_INIT_PROJECT_NAME", "Langfuse project name", False),
        ("LANGFUSE_INIT_PROJECT_PUBLIC_KEY", "Langfuse public key", False),
        ("LANGFUSE_INIT_PROJECT_SECRET_KEY", "Langfuse secret key", False),
        ("LANGFUSE_INIT_USER_EMAIL", "Langfuse admin email", False),
        ("LANGFUSE_INIT_USER_PASSWORD", "Langfuse admin password", False),
        ("LANGFUSE_INIT_USER_NAME", "Langfuse admin display name", False),
    ],
    "goat-postgres": [
        ("POSTGRES_USER", "Postgres username", True),
        ("POSTGRES_PASSWORD", "Postgres password (auto-generated if empty)", True),
        ("POSTGRES_DB", "Postgres database name", True),
    ],
}

# Defaults for auto-generation
LANGFUSE_DEFAULTS = {
    "LANGFUSE_INIT_ORG_ID": "goat",
    "LANGFUSE_INIT_ORG_NAME": "Goat",
    "LANGFUSE_INIT_PROJECT_ID": "goat-evals",
    "LANGFUSE_INIT_PROJECT_NAME": "Goat Evals",
    "LANGFUSE_INIT_PROJECT_PUBLIC_KEY": "pk-lf-goat-modal",
    "LANGFUSE_INIT_PROJECT_SECRET_KEY": "sk-lf-goat-modal",
    "LANGFUSE_INIT_USER_EMAIL": "admin@goat.local",
    "LANGFUSE_INIT_USER_PASSWORD": secrets.token_hex(16),
    "LANGFUSE_INIT_USER_NAME": "Admin",
}

POSTGRES_DEFAULTS = {
    "POSTGRES_USER": "goat",
    "POSTGRES_PASSWORD": secrets.token_hex(32),
    "POSTGRES_DB": "goat",
}


def load_dotenv(env_file: str) -> dict[str, str]:
    """Load key=value pairs from a .env file (simple parser, no interpolation)."""
    env = {}
    path = Path(env_file)
    if not path.exists():
        return env
    for line in path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        if "=" in line:
            key, _, value = line.partition("=")
            env[key.strip()] = value.strip()
    return env


def resolve_value(key: str, dot_env: dict[str, str], defaults: dict[str, str] | None = None) -> str:
    """Resolve a value from .env, defaults, or auto-generate."""
    # Try .env first
    if key in dot_env and dot_env[key]:
        return dot_env[key]
    # Try OS env
    if os.environ.get(key):
        return os.environ[key]
    # Try defaults
    if defaults and key in defaults:
        return defaults[key]
    # Auto-generate for known secret/salt fields
    if "SECRET" in key or "SALT" in key or "PASSWORD" in key:
        return secrets.token_hex(32)
    return ""


def confirm(prompt: str) -> bool:
    """Ask user for yes/no confirmation."""
    while True:
        response = input(f"{prompt} [y/N] ").strip().lower()
        if response in ("y", "yes"):
            return True
        if response in ("n", "no", ""):
            return False


def create_secret(name: str, values: dict[str, str], dry_run: bool) -> bool:
    """Create or update a Modal secret with user confirmation."""
    import subprocess as sp

    print(f"\n{'=' * 60}")
    print(f"Secret: {name}")
    print(f"Environment: {MODAL_ENVIRONMENT}")
    print(f"{'=' * 60}")

    for key, value in values.items():
        display = value[:8] + "..." if len(value) > 12 else value
        if "PASSWORD" in key or "SECRET" in key or "KEY" in key or "SALT" in key:
            display = value[:4] + "****" + value[-4:] if len(value) > 8 else "****"
        print(f"  {key} = {display}")

    if dry_run:
        print(f"  [DRY RUN] Would create secret '{name}'")
        return True

    if not confirm(f"\nCreate/update secret '{name}'?"):
        print(f"  Skipped '{name}'")
        return False

    try:
        # Use modal CLI to create the secret
        cmd = [
            "modal", "secret", "create", name,
            "--env", MODAL_ENVIRONMENT,
            "--force",  # overwrite if exists
        ]
        for key, value in values.items():
            cmd.append(f"{key}={value}")

        result = sp.run(cmd, capture_output=True, text=True)
        if result.returncode == 0:
            print(f"  Created '{name}' in '{MODAL_ENVIRONMENT}' environment")
            return True
        else:
            print(f"  Error: {result.stderr.strip()}", file=sys.stderr)
            return False
    except Exception as e:
        print(f"  Error creating '{name}': {e}", file=sys.stderr)
        return False


def main():
    parser = argparse.ArgumentParser(
        description="Set up Modal environment and secrets for Goat",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument(
        "--env-file", default=".env",
        help="Path to .env file with provider keys (default: .env)",
    )
    parser.add_argument(
        "--dry-run", action="store_true",
        help="Show what would be created without actually creating anything",
    )
    args = parser.parse_args()

    print("Goat Modal Setup")
    print("=" * 60)
    print(f"Environment: {MODAL_ENVIRONMENT}")
    print(f"Env file: {args.env_file}")
    if args.dry_run:
        print("MODE: DRY RUN (no changes will be made)")
    print()

    # Ensure environment exists
    import subprocess as sp
    env_check = sp.run(
        ["modal", "environment", "list"],
        capture_output=True, text=True,
    )
    if MODAL_ENVIRONMENT not in env_check.stdout:
        print(f"Creating Modal environment '{MODAL_ENVIRONMENT}'...")
        if not args.dry_run:
            sp.run(["modal", "environment", "create", MODAL_ENVIRONMENT], check=True)
            print(f"  Environment '{MODAL_ENVIRONMENT}' created")
        else:
            print(f"  [DRY RUN] Would create environment '{MODAL_ENVIRONMENT}'")
    else:
        print(f"Environment '{MODAL_ENVIRONMENT}' already exists")

    # Load .env
    dot_env = load_dotenv(args.env_file)
    if dot_env:
        print(f"Loaded {len(dot_env)} variables from {args.env_file}")
    else:
        print(f"Warning: No .env file found at {args.env_file}")
        print("  Values will be auto-generated or prompted interactively")

    # 1. goat-llm-providers
    llm_values = {}
    for key, desc, required in SECRETS_SCHEMA["goat-llm-providers"]:
        value = resolve_value(key, dot_env)
        if not value and required:
            print(f"\n  Required: {key} ({desc})")
            value = input(f"  Enter value for {key}: ").strip()
            if not value:
                print(f"  Error: {key} is required but not provided", file=sys.stderr)
                sys.exit(1)
        if value:
            llm_values[key] = value
    create_secret("goat-llm-providers", llm_values, args.dry_run)

    # 2. goat-litellm
    litellm_values = {}
    for key, desc, required in SECRETS_SCHEMA["goat-litellm"]:
        value = resolve_value(key, dot_env)
        if not value:
            value = "sk-" + secrets.token_hex(32)
            print(f"  Auto-generated {key}")
        litellm_values[key] = value
    create_secret("goat-litellm", litellm_values, args.dry_run)

    # 3. goat-langfuse
    langfuse_values = {}
    for key, desc, required in SECRETS_SCHEMA["goat-langfuse"]:
        value = resolve_value(key, dot_env, LANGFUSE_DEFAULTS)
        if value:
            langfuse_values[key] = value
    create_secret("goat-langfuse", langfuse_values, args.dry_run)

    # 4. goat-postgres
    pg_values = {}
    for key, desc, required in SECRETS_SCHEMA["goat-postgres"]:
        value = resolve_value(key, dot_env, POSTGRES_DEFAULTS)
        pg_values[key] = value
    create_secret("goat-postgres", pg_values, args.dry_run)

    # Summary
    print(f"\n{'=' * 60}")
    print("Setup complete!")
    print()
    print("Next steps:")
    print("  1. Deploy services: modal deploy scripts/modal_services.py")
    print("  2. Deploy vLLM:     modal deploy scripts/modal_vllm.py")
    print("  3. Run benchmarks:  python scripts/modal_sandbox.py --prompt 'Hello'")


if __name__ == "__main__":
    main()
