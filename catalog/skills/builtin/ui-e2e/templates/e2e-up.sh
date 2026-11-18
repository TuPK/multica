#!/usr/bin/env bash
# Skeleton for phase 0 path B/C: start the full stack for UI e2e tests.
# Replace every TODO with the project's real commands. Keep it idempotent.
# It must never touch the development database or the development env file.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TEST_DB="${TEST_DB_NAME:-app_test}"              # must end in _test
TEST_ENV="$ROOT/.env.e2e"                        # generated, never committed
API_PORT="${API_PORT:-18080}"                    # pick ports that do not collide with dev
WEB_PORT="${WEB_PORT:-13000}"

case "$TEST_DB" in *_test) ;; *) echo "refusing: test database must end in _test" >&2; exit 1 ;; esac

gen_secret() { head -c 32 /dev/urandom | base64 | tr -d '\n'; }

if [ ! -f "$TEST_ENV" ]; then
  # TODO: copy keys from the project's .env.example, then override:
  cat > "$TEST_ENV" <<ENV
APP_ENV=test
DATABASE_URL=TODO_test_database_url_using_${TEST_DB}
JWT_SECRET=$(gen_secret)
PORT=${API_PORT}
FRONTEND_PORT=${WEB_PORT}
ENV
  chmod 600 "$TEST_ENV"
fi

# 1. Dependencies the product needs (containers for stateful services only)
# TODO: docker compose -f "$ROOT/docker-compose.e2e.yml" up -d db redis

# 2. Migrations and seed (idempotent)
# TODO: run the project's migrate command against $TEST_DB, then the seed command

# 3. Start the backend and the frontend in the background, logging to files
mkdir -p "$ROOT/test-results/e2e-logs"
# TODO: (cd "$ROOT/server" && env $(cat "$TEST_ENV" | xargs) go run ./cmd/server > "$ROOT/test-results/e2e-logs/api.log" 2>&1 &)
# TODO: (cd "$ROOT/web" && pnpm dev --port "$WEB_PORT" > "$ROOT/test-results/e2e-logs/web.log" 2>&1 &)

# 4. Wait for health
wait_for() {
  local url="$1" tries=60
  until curl -fsS "$url" >/dev/null 2>&1; do
    tries=$((tries - 1)); [ "$tries" -le 0 ] && { echo "timed out: $url" >&2; exit 1; }
    sleep 2
  done
  echo "ready: $url"
}
# TODO: wait_for "http://127.0.0.1:${API_PORT}/healthz"
# TODO: wait_for "http://127.0.0.1:${WEB_PORT}/"
echo "e2e stack is up (TODO: replace the placeholders above first)"
