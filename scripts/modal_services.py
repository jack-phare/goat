"""Goat infrastructure on Modal: LiteLLM proxy + Langfuse (with co-located Postgres).

LiteLLM routes LLM calls to Azure, Groq, OpenAI, Anthropic, or local vLLM.
Langfuse provides observability (traces, costs, latency) via HTTP callbacks.
Postgres runs inside the Langfuse container (Modal proxies HTTP only, not raw TCP,
so Postgres must be on localhost within the same function).

Deploy:
    modal deploy scripts/modal_services.py --env goat

Test LiteLLM:
    curl https://<workspace>--goat-services-litellm.modal.run/v1/models \\
      -H "Authorization: Bearer <LITELLM_MASTER_KEY>"

View Langfuse UI:
    Open https://<workspace>--goat-services-langfuse.modal.run
"""

import subprocess
import os
import time
from pathlib import Path

import modal

MODAL_ENVIRONMENT = "goat"
MINUTES = 60

# ---------------------------------------------------------------------------
# App
# ---------------------------------------------------------------------------
app = modal.App("goat-services")

# ---------------------------------------------------------------------------
# Secrets
# ---------------------------------------------------------------------------
litellm_secret = modal.Secret.from_name("goat-litellm", environment_name=MODAL_ENVIRONMENT)
llm_providers_secret = modal.Secret.from_name("goat-llm-providers", environment_name=MODAL_ENVIRONMENT)
langfuse_secret = modal.Secret.from_name("goat-langfuse", environment_name=MODAL_ENVIRONMENT)
postgres_secret = modal.Secret.from_name("goat-postgres", environment_name=MODAL_ENVIRONMENT)

# ---------------------------------------------------------------------------
# Images
# ---------------------------------------------------------------------------

# Langfuse: Alpine image with Python + Node.js + Postgres (all co-located).
# The Langfuse v2 app is copied from the official image. Postgres data dir
# is pre-initialized in the image build step (ephemeral, fine for dev/benchmarks).
langfuse_image = modal.Image.from_dockerfile(
    str(Path(__file__).parent / "Dockerfile.langfuse"),
)

# LiteLLM: stateless proxy with langfuse client (pinned to v2 for litellm compat).
litellm_image = (
    modal.Image.debian_slim(python_version="3.12")
    .pip_install("litellm[proxy]", "langfuse>=2.0,<3.0")
    .add_local_file(
        str(Path(__file__).parent.parent / "dev" / "litellm-config-modal.yaml"),
        remote_path="/app/config.yaml",
    )
)


# ---------------------------------------------------------------------------
# Langfuse (with co-located Postgres)
# ---------------------------------------------------------------------------
@app.function(
    image=langfuse_image,
    secrets=[langfuse_secret, postgres_secret],
    timeout=24 * 60 * MINUTES,
    min_containers=1,
    memory=2048,
)
@modal.concurrent(max_inputs=100)
@modal.web_server(port=3000, startup_timeout=5 * MINUTES)
def langfuse():
    """Run Langfuse v2 with co-located Postgres on localhost."""
    pg_user = os.environ.get("POSTGRES_USER", "goat")
    pg_pass = os.environ.get("POSTGRES_PASSWORD", "goat")

    # ── Start Postgres (localhost, pre-initialized data dir) ──
    print("Starting co-located Postgres...")
    subprocess.run(
        ["su-exec", "postgres", "pg_ctl", "start",
         "-D", "/var/lib/postgresql/data", "-l", "/tmp/pg.log",
         "-w", "-o", "-p 5432"],
        check=True,
    )

    # Wait for Postgres
    for _ in range(30):
        r = subprocess.run(
            ["su-exec", "postgres", "pg_isready", "-p", "5432"],
            capture_output=True,
        )
        if r.returncode == 0:
            break
        time.sleep(1)

    # Create role and database (idempotent)
    subprocess.run(
        ["su-exec", "postgres", "psql", "-p", "5432", "-c",
         f"DO $$ BEGIN IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='{pg_user}') "
         f"THEN CREATE ROLE {pg_user} WITH LOGIN PASSWORD '{pg_pass}' CREATEDB; END IF; END $$;"],
        check=False,
    )
    subprocess.run(
        ["su-exec", "postgres", "psql", "-p", "5432", "-tc",
         f"SELECT 1 FROM pg_database WHERE datname='langfuse'"],
        capture_output=True,
    )
    # Create langfuse database if it doesn't exist
    r = subprocess.run(
        ["su-exec", "postgres", "psql", "-p", "5432", "-tc",
         "SELECT 1 FROM pg_database WHERE datname='langfuse'"],
        capture_output=True, text=True,
    )
    if "1" not in (r.stdout or ""):
        subprocess.run(
            ["su-exec", "postgres", "createdb", "-p", "5432", "-O", pg_user, "langfuse"],
            check=False,
        )
    print(f"  Postgres ready (localhost:5432, user={pg_user})")

    # ── Configure Langfuse env vars ──
    database_url = f"postgresql://{pg_user}:{pg_pass}@127.0.0.1:5432/langfuse"

    # Langfuse needs ENCRYPTION_KEY, NEXTAUTH_SECRET, and SALT
    salt = os.environ.get("LANGFUSE_SALT", "a" * 64)

    os.environ["DATABASE_URL"] = database_url
    os.environ["DIRECT_URL"] = database_url
    os.environ["NEXTAUTH_URL"] = "http://localhost:3000"
    os.environ.setdefault("HOSTNAME", "0.0.0.0")
    if not os.environ.get("ENCRYPTION_KEY"):
        os.environ["ENCRYPTION_KEY"] = salt
    if not os.environ.get("NEXTAUTH_SECRET"):
        os.environ["NEXTAUTH_SECRET"] = salt
    if not os.environ.get("SALT"):
        os.environ["SALT"] = salt

    print(f"Langfuse starting...")
    print(f"  DATABASE_URL: {database_url[:50]}...")
    print(f"  Org: {os.environ.get('LANGFUSE_INIT_ORG_NAME', 'N/A')}")
    print(f"  Project: {os.environ.get('LANGFUSE_INIT_PROJECT_NAME', 'N/A')}")

    # ── Run Prisma migrations ──
    print("  Running Prisma migrations...")
    migrate = subprocess.run(
        ["prisma", "migrate", "deploy",
         "--schema", "/app/packages/shared/prisma/schema.prisma"],
        cwd="/app",
        capture_output=True, text=True,
        timeout=120,
        env={**os.environ, "DATABASE_URL": database_url, "DIRECT_URL": database_url},
    )
    if migrate.returncode == 0:
        print("  Migrations applied successfully")
    else:
        print(f"  Migration output: {migrate.stdout[:300]}")
        print(f"  Migration stderr: {migrate.stderr[:300]}")

    # ── Start Langfuse Node.js server ──
    node_env = {**os.environ}
    node_env["PORT"] = "3000"
    node_env["HOSTNAME"] = "0.0.0.0"
    node_env["DATABASE_URL"] = database_url
    node_env["DIRECT_URL"] = database_url

    print("  Starting Langfuse web server on port 3000...")
    subprocess.Popen(
        ["node", "web/server.js", "--keepAliveTimeout", "110000"],
        cwd="/app",
        env=node_env,
    )


# ---------------------------------------------------------------------------
# LiteLLM Proxy
# ---------------------------------------------------------------------------
@app.function(
    image=litellm_image,
    secrets=[litellm_secret, llm_providers_secret, langfuse_secret],
    timeout=24 * 60 * MINUTES,
    min_containers=1,
)
@modal.concurrent(max_inputs=1000)
@modal.web_server(port=4000, startup_timeout=3 * MINUTES)
def litellm():
    """Run LiteLLM proxy -- stateless, Langfuse via HTTP callbacks."""
    # Ensure no DATABASE_URL leaks in (would trigger Prisma requirement)
    os.environ.pop("DATABASE_URL", None)

    # Langfuse: map the init secret keys to the env vars LiteLLM expects
    lf_pub = os.environ.get("LANGFUSE_INIT_PROJECT_PUBLIC_KEY", "")
    lf_sec = os.environ.get("LANGFUSE_INIT_PROJECT_SECRET_KEY", "")
    if lf_pub:
        os.environ["LANGFUSE_PUBLIC_KEY"] = lf_pub
    if lf_sec:
        os.environ["LANGFUSE_SECRET_KEY"] = lf_sec

    # Point Langfuse client at the internal Langfuse service
    langfuse_host = "http://goat-services-langfuse.modal.internal:3000"
    os.environ["LANGFUSE_HOST"] = langfuse_host
    print(f"  Langfuse: {langfuse_host}")

    providers = [k for k in ["AZURE_API_KEY", "GROQ_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY"] if os.environ.get(k)]
    print(f"LiteLLM starting (stateless, no DB)")
    print(f"  Config: /app/config.yaml")
    print(f"  Providers: {', '.join(providers)}")

    subprocess.Popen([
        "litellm",
        "--config", "/app/config.yaml",
        "--host", "0.0.0.0",
        "--port", "4000",
    ])
