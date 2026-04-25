#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -a
if [[ -f /home/admin/code/auth_db/postgres.env ]]; then
  source /home/admin/code/auth_db/postgres.env
fi
if [[ -f "$ROOT_DIR/.env" ]]; then
  source "$ROOT_DIR/.env"
fi
set +a

export JWT_SECRET="${JWT_SECRET:-change-this-in-production}"
export PORT="${PORT:-8080}"
export DEFAULT_EVENT_SLUG="${DEFAULT_EVENT_SLUG:-bohack-2026}"
export DEFAULT_EVENT_TITLE="${DEFAULT_EVENT_TITLE:-BoHack 2026}"
export ALLOWED_ORIGINS="${ALLOWED_ORIGINS:-*}"

exec go run ./cmd/server
