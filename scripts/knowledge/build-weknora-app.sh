#!/usr/bin/env bash
# Build the WeKnora app image from upstream source plus the patches this repo carries.
#
# Why: we run WeKnora as published images and do not fork it. A bug we need fixed
# is carried as a patch in docker/knowledge/weknora/patches and applied here, so
# every rebuild gets the fix without a fork or a push to upstream.
#
# The patches apply to the commit in docker/knowledge/weknora/BASE_COMMIT. The
# image is built with the upstream Dockerfile (docker/Dockerfile.app) and its
# default build args, so it matches the published image apart from the patches.
#
# Usage:
#   scripts/knowledge/build-weknora-app.sh [--check]
#
#   --check   run the unit tests of the patched packages before building the image
#
# Environment:
#   WEKNORA_SRC       a WeKnora checkout that contains BASE_COMMIT (default: a
#                     clone of upstream under ~/.cache/multica/weknora-src)
#   WEKNORA_UPSTREAM  upstream URL used when the default clone is created
#                     (default: https://github.com/Tencent/WeKnora.git)
#   KNOWLEDGE_BASE    the published image the patches are built on
#                     (default: wechatopenai/weknora-app:v0.9.0)
#
# After the build, set KNOWLEDGE_VERSION to the printed tag in .env and recreate
# the knowledge-app service, so a later `make dev-full` keeps the patched image.
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
PATCH_DIR="$ROOT/docker/knowledge/weknora/patches"
BASE_COMMIT="$(tr -d '[:space:]' < "$ROOT/docker/knowledge/weknora/BASE_COMMIT")"
UPSTREAM="${WEKNORA_UPSTREAM:-https://github.com/Tencent/WeKnora.git}"
KNOWLEDGE_BASE="${KNOWLEDGE_BASE:-wechatopenai/weknora-app:v0.9.0}"
BASE_TAG="${KNOWLEDGE_BASE##*:}"
IMAGE="wechatopenai/weknora-app:${BASE_TAG}-patched"

CHECK=0
for arg in "$@"; do
  case "$arg" in
    --check) CHECK=1 ;;
    *) echo "unknown argument: $arg" >&2; exit 2 ;;
  esac
done

SRC="${WEKNORA_SRC:-$HOME/.cache/multica/weknora-src}"
if [ -z "${WEKNORA_SRC:-}" ] && [ ! -d "$SRC/.git" ]; then
  mkdir -p "$(dirname "$SRC")"
  git clone --quiet "$UPSTREAM" "$SRC"
fi
if ! git -C "$SRC" cat-file -e "${BASE_COMMIT}^{commit}" 2>/dev/null; then
  git -C "$SRC" fetch --quiet origin
fi
git -C "$SRC" cat-file -e "${BASE_COMMIT}^{commit}" || {
  echo "commit $BASE_COMMIT is not in $SRC" >&2; exit 1; }

BUILD="$(mktemp -d)"
trap 'rm -rf "$BUILD"' EXIT
git -C "$SRC" archive "$BASE_COMMIT" | tar -x -C "$BUILD"

shopt -s nullglob
patches=("$PATCH_DIR"/*.patch)
if [ "${#patches[@]}" -eq 0 ]; then
  echo "no patches in $PATCH_DIR" >&2; exit 1
fi
for patch in "${patches[@]}"; do
  echo "apply $(basename "$patch")"
  (cd "$BUILD" && git apply "$patch")
done

if [ "$CHECK" -eq 1 ]; then
  (cd "$BUILD" && go test -count=1 ./internal/application/repository/retriever/postgres/)
fi

BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
docker build \
  -f "$BUILD/docker/Dockerfile.app" \
  --build-arg VERSION_ARG="${BASE_TAG}-patched" \
  --build-arg COMMIT_ID_ARG="$(git -C "$SRC" rev-parse --short "$BASE_COMMIT")+patches" \
  --build-arg BUILD_TIME_ARG="$BUILD_TIME" \
  -t "$IMAGE" \
  "$BUILD"

echo "built $IMAGE"
echo "next: set KNOWLEDGE_VERSION=${BASE_TAG}-patched in .env, then recreate knowledge-app"
