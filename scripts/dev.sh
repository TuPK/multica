#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

# ---------- Check prerequisites ----------
missing=()
command -v node >/dev/null 2>&1 || missing+=("node")
command -v pnpm >/dev/null 2>&1 || missing+=("pnpm")
command -v go >/dev/null 2>&1 || missing+=("go")
command -v docker >/dev/null 2>&1 || missing+=("docker")

if [ ${#missing[@]} -gt 0 ]; then
  echo "✗ Missing prerequisites: ${missing[*]}"
  echo "  Please install: Node.js 22, pnpm 10.28.2, Go 1.26.9, Docker"
  exit 1
fi

# ---------- Environment file ----------
if [ -f .git ]; then
  # Inside a git worktree (.git is a file, not a directory)
  ENV_FILE=".env.worktree"
  if [ ! -f "$ENV_FILE" ]; then
    echo "==> Worktree detected. Generating $ENV_FILE..."
    bash scripts/init-worktree-env.sh "$ENV_FILE"
  fi
else
  ENV_FILE=".env"
  if [ ! -f "$ENV_FILE" ]; then
    echo "==> Creating $ENV_FILE from .env.example..."
    cp .env.example "$ENV_FILE"
  fi
fi

echo "==> Using $ENV_FILE"

set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a

# shellcheck disable=SC1091
. scripts/local-env.sh

# ---------- Install dependencies ----------
# Always run: pnpm is incremental, and a node_modules left half-linked by an
# earlier install (a dangling next symlink) passes a directory check but fails
# at `pnpm dev:web`.
echo "==> Installing dependencies..."
pnpm install

# ---------- Database ----------
bash scripts/ensure-postgres.sh "$ENV_FILE"

echo "==> Generating the catalog data the Go server embeds..."
node scripts/catalog/cli.mjs build --target go

echo "==> Running migrations..."
(cd server && go run ./cmd/migrate up)

# ---------- Start services ----------
# WEB_MODE=dev runs the hot-reloading dev server, which compiles a page the first time
# it is requested. WEB_MODE=build builds the web app once and serves the build, so page loads and
# navigation do not wait for compilation. A build is needed again after the web code changes.
WEB_MODE="${WEB_MODE:-build}"
case "$WEB_MODE" in
  dev)
    WEB_CMD=(pnpm dev:web)
    ;;
  build)
    echo "==> Building the web app (WEB_MODE=build)..."
    # The web build imports the catalog's generated TypeScript, which turbo would build first.
    pnpm catalog:build
    pnpm --filter @multica/web build
    WEB_CMD=(pnpm --filter @multica/web exec next start --port "${FRONTEND_PORT:-3000}")
    ;;
  *)
    echo "WEB_MODE must be dev or build, not '$WEB_MODE'" >&2
    exit 1
    ;;
esac

echo ""
echo "✓ Ready. Starting services..."
echo "  Backend:  http://localhost:${PORT:-8080}"
echo "  Frontend: http://localhost:${FRONTEND_PORT:-3000} (web: $WEB_MODE)"
echo ""

trap 'kill 0' EXIT
(cd server && go run ./cmd/server) &
"${WEB_CMD[@]}" &
wait
