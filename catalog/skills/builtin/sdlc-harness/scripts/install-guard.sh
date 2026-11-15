#!/bin/sh
# Installs or removes the push guard (guard-push.sh) in one clone's hooks folder (W5.a of harness-optimization-plan).
#
#   sh install-guard.sh --repo <dir>            install the pre-push hook
#   sh install-guard.sh --repo <dir> --remove   remove it, if it is the guard (one-command rollback)
#
# It never overwrites a pre-push hook it did not write.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
marker="Git pre-push hook for an SDLC squad"
repo=""
remove=0
while [ $# -gt 0 ]; do
  case "$1" in
    --repo) repo="${2:-}"; shift 2 ;;
    --remove) remove=1; shift ;;
    *) echo "usage: install-guard.sh --repo <dir> [--remove]" >&2; exit 2 ;;
  esac
done
if [ -z "$repo" ]; then
  echo "usage: install-guard.sh --repo <dir> [--remove]" >&2
  exit 2
fi

hooks=$(git -C "$repo" rev-parse --git-path hooks)
case "$hooks" in
  /*) ;;
  *) hooks="$repo/$hooks" ;;
esac
hook="$hooks/pre-push"

if [ "$remove" = 1 ]; then
  if [ -f "$hook" ] && grep -q "$marker" "$hook"; then
    rm -f "$hook"
    echo "removed $hook"
  else
    echo "nothing to remove at $hook"
  fi
  exit 0
fi

if [ -f "$hook" ] && ! grep -q "$marker" "$hook"; then
  echo "refusing: $hook exists and is not the SDLC guard" >&2
  exit 1
fi
mkdir -p "$hooks"
cp "$here/guard-push.sh" "$hook"
chmod +x "$hook"
echo "installed $hook"
