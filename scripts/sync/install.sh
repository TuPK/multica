#!/usr/bin/env bash
# Defines the merge drivers named in .gitattributes and turns on rerere.
# Idempotent. The config goes to the repository's shared config, so every worktree
# of this checkout uses it. Undo with: bash scripts/sync/install.sh --remove
set -euo pipefail
root="$(git rev-parse --show-toplevel)"
dir="$root/scripts/sync"

if [ "${1:-}" = "--remove" ]; then
  for name in brand jsonmerge ours; do
    git config --remove-section "merge.$name" 2>/dev/null || true
  done
  git config --unset rerere.enabled 2>/dev/null || true
  echo "sync merge drivers removed"
  exit 0
fi

git config merge.brand.name "Brand-aware text merge"
git config merge.brand.driver "node \"$dir/merge-brand.mjs\" %O %A %B %L %P"
git config merge.jsonmerge.name "Key-wise JSON merge"
git config merge.jsonmerge.driver "node \"$dir/merge-json.mjs\" %O %A %B %L"
git config merge.ours.name "Keep the fork's copy (regenerated afterwards)"
git config merge.ours.driver true
git config rerere.enabled true
echo "sync merge drivers installed (brand, jsonmerge, ours) and rerere enabled"
