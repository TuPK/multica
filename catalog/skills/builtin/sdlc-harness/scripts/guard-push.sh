#!/bin/sh
# Git pre-push hook for an SDLC squad's repository (W5.a of harness-optimization-plan).
# Refuses any push that would change refs/heads/main (or delete it) unless SDLC_ROLE=qa is set: the QA landing
# step is the only role that moves main. Installed by install-guard.sh. A developer's `git push --no-verify`
# skips this hook, so the remote must refuse the same push too (W5.b).
#
# git runs this with the remote name and URL as arguments and one line per ref on stdin:
#   <local ref> <local sha> <remote ref> <remote sha>

remote="$1"
while read -r local_ref local_sha remote_ref remote_sha; do
  if [ "$remote_ref" = "refs/heads/main" ] && [ "$SDLC_ROLE" != "qa" ]; then
    echo "guard: push to refs/heads/main refused for remote $remote. Only the QA landing step may move main (SDLC_ROLE=qa)." >&2
    echo "guard: push your branch instead: git push origin HEAD:<branch>" >&2
    exit 1
  fi
done
exit 0
