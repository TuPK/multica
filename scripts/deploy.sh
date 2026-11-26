#!/usr/bin/env bash
# Build Delegora's production images, push them to Docker Hub, then pull and
# start the full self-hosted stack on a server over SSH.
#
# Usage:
#   make deploy ENV=<server-ip>
#   scripts/deploy.sh <server-ip>
#
# Environment:
#   DEPLOY_NS    Docker Hub namespace for the images (default: tupk)
#   DEPLOY_DIR   directory on the server holding docker-compose.selfhost.yml and
#                .env (default: delegora, relative to the SSH user's home)
#   DEPLOY_USER  SSH user (default: the ssh config or the local user)
#   DEPLOY_TAG   image tag (default: git describe, e.g. 32fd0e4 or 32fd0e4-dirty)
#
# The server's .env must exist before the first deploy. It holds the server's own
# secrets and URLs, so this script never copies the local .env. The image names
# and tag are passed to compose as shell variables, which override .env.
set -euo pipefail

HOST="${1:?usage: scripts/deploy.sh <server-ip>}"
ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

NS="${DEPLOY_NS:-tupk}"
REMOTE_DIR="${DEPLOY_DIR:-delegora}"
TARGET="${DEPLOY_USER:+$DEPLOY_USER@}$HOST"
TAG="${DEPLOY_TAG:-$(git describe --always --dirty --abbrev=7)}"
COMMIT="$(git rev-parse --short HEAD)"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

echo "==> Checking SSH access and server architecture: $TARGET"
ARCH="$(ssh "$TARGET" uname -m)" || { echo "Cannot reach $TARGET over SSH." >&2; exit 1; }
case "$ARCH" in
  x86_64 | amd64) PLATFORM=linux/amd64 ;;
  aarch64 | arm64) PLATFORM=linux/arm64 ;;
  *) echo "Unsupported server architecture: $ARCH" >&2; exit 1 ;;
esac
if ! ssh "$TARGET" "test -f $REMOTE_DIR/.env"; then
  echo "$REMOTE_DIR/.env does not exist on $TARGET. Create it there first (see SELF_HOSTING.md), then run deploy again." >&2
  exit 1
fi

echo "==> Building and pushing $NS/*:$TAG for $PLATFORM"
# Builds can fail on transient network resets while fetching modules or base
# images, so each image gets up to three attempts.
build() {
  local name="$1" dockerfile="$2" attempt
  shift 2
  for attempt in 1 2 3; do
    if docker buildx build --platform "$PLATFORM" --push --network=host \
      -f "$dockerfile" -t "$NS/$name:$TAG" "$@" .; then
      return 0
    fi
    echo "Build of $name failed (attempt $attempt of 3)" >&2
  done
  echo "Giving up on $name" >&2
  return 1
}
build multica-backend Dockerfile \
  --build-arg VERSION="$TAG" --build-arg COMMIT="$COMMIT" --build-arg DATE="$BUILD_DATE"
build multica-web Dockerfile.web --build-arg NEXT_PUBLIC_APP_VERSION="$TAG"
build multica-collab Dockerfile.collab
build multica-landing Dockerfile.landing
build multica-platform platform-management/Dockerfile

echo "==> Copying docker-compose.selfhost.yml to $TARGET:$REMOTE_DIR"
ssh "$TARGET" "mkdir -p $REMOTE_DIR"
scp -q docker-compose.selfhost.yml "$TARGET:$REMOTE_DIR/"

echo "==> Pulling and starting the stack on $TARGET"
ssh "$TARGET" bash -s <<EOF
set -euo pipefail
cd "$REMOTE_DIR"
export MULTICA_IMAGE_TAG="$TAG"
export MULTICA_BACKEND_IMAGE="$NS/multica-backend"
export MULTICA_WEB_IMAGE="$NS/multica-web"
export MULTICA_COLLAB_IMAGE="$NS/multica-collab"
export MULTICA_LANDING_IMAGE="$NS/multica-landing"
export MULTICA_PLATFORM_IMAGE="$NS/multica-platform"
export KNOWLEDGE_IMAGE_NAMESPACE="$NS"
COMPOSE="docker compose -f docker-compose.selfhost.yml --env-file .env"
\$COMPOSE pull
\$COMPOSE up -d
\$COMPOSE ps
EOF

echo "==> Deployed $NS/*:$TAG to $TARGET"
