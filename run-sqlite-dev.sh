#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SQLITE_PATH_DEFAULT="$ROOT_DIR/storage/bohack-dev.sqlite"

mkdir -p "$ROOT_DIR/storage/registration_attachments"

set -a
if [[ -f "$ROOT_DIR/.env" ]]; then
  source "$ROOT_DIR/.env"
fi
set +a

export DB_DRIVER="sqlite"
export SQLITE_PATH="${SQLITE_PATH:-$SQLITE_PATH_DEFAULT}"
export DATABASE_URL="$SQLITE_PATH"
export JWT_SECRET="${JWT_SECRET:-change-this-in-production}"
export PORT="${PORT:-8080}"
export DEFAULT_EVENT_SLUG="${DEFAULT_EVENT_SLUG:-bohack-2026}"
export DEFAULT_EVENT_TITLE="${DEFAULT_EVENT_TITLE:-BoHack 2026}"
export FRONTEND_BASE_URL="${FRONTEND_BASE_URL:-http://127.0.0.1:5173}"
export ALLOWED_ORIGINS="${ALLOWED_ORIGINS:-http://127.0.0.1:5173,http://localhost:5173}"
export ATTACHMENT_DIR="${ATTACHMENT_DIR:-$ROOT_DIR/storage/registration_attachments}"
export MAIL_MODE="${MAIL_MODE:-console}"

exec go run ./cmd/server
