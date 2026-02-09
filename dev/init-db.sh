#!/bin/bash
# Create separate databases for LiteLLM and Langfuse.
# Runs once on first postgres boot via docker-entrypoint-initdb.d.
set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    CREATE DATABASE litellm;
    CREATE DATABASE langfuse;
EOSQL
