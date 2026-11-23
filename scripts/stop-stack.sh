#!/usr/bin/env bash
# Stops every local process of this checkout's Delegora stack, so the next make dev-full starts clean:
#   - anything listening on the given app ports,
#   - any process started from this checkout (go run, next, turbo, server/bin/multica),
#   - the installed daemon binary (<install-path> daemon ...).
# Docker containers (PostgreSQL, WeKnora, Hub, Ollama) are not touched.
#
# Usage: scripts/stop-stack.sh <repo-root> <install-path> <port>...
set -uo pipefail

root=${1:?repo root required}
install=${2:?install path required}
shift 2

self=$$
parent=$PPID
pids=""

add_pid() {
  case " $pids " in *" $1 "*) ;; *) pids="$pids $1" ;; esac
}

# Listening PIDs on a port. `next start` runs with relative arguments, and lsof misses some sockets
# on overlay mounts, so ss (Linux) is read first.
listeners() {
  if command -v ss >/dev/null 2>&1; then
    ss -Hltnp "sport = :$1" 2>/dev/null | grep -o 'pid=[0-9]*' | cut -d= -f2
  else
    lsof -nP -iTCP:"$1" -sTCP:LISTEN -t 2>/dev/null
  fi
}

# Processes started from this checkout or running the installed daemon. Matched by command line, or by
# working directory on Linux, since `next start --port` runs with relative arguments. Skipped: this
# script, its parent, `make` itself, shell wrappers (`bash -c "cd <repo> && ..."`), interactive
# shells, and Claude Code, which often runs in the repo directory and must not be stopped.
while read -r pid args; do
  case "$pid" in "$self" | "$parent") continue ;; esac
  case "$args" in make\ * | */make\ *) continue ;; esac
  case "$args" in *sh\ -c\ *) continue ;; esac
  case "$args" in -*sh | sh | bash | zsh | fish | */bash | */zsh | */sh | */fish) continue ;; esac
  case "$args" in *claude* | *Claude*) continue ;; esac
  case "$args" in
    *"$root/"* | "$install daemon"*) add_pid "$pid"; continue ;;
  esac
  # By working directory, only for the stack's own runtimes: editor and search helpers (gopls, ugrep)
  # also run in the repo and must stay up.
  cwd=$(readlink "/proc/$pid/cwd" 2>/dev/null)
  exe=${args%% *}
  exe=${exe##*/}
  case "$cwd" in
    "$root" | "$root"/*)
      case "$exe" in
        node | pnpm | next-server | turbo | esbuild | go | server | multica | dotenvx) add_pid "$pid" ;;
        *) case "$args" in *scripts/dev*) add_pid "$pid" ;; esac ;;
      esac ;;
  esac
done < <(ps -eo pid=,args=)

for port in "$@"; do
  for pid in $(listeners "$port"); do
    add_pid "$pid"
  done
done

pids=$(echo $pids)
if [ -z "$pids" ]; then
  echo "✓ No stack processes found."
  exit 0
fi

echo "Stopping stack processes:"
for pid in $pids; do
  ps -p "$pid" -o pid=,args= 2>/dev/null | cut -c1-160 | sed 's/^/  /'
done

kill -TERM $pids 2>/dev/null
sleep 2
for pid in $pids; do
  if kill -0 "$pid" 2>/dev/null; then
    kill -KILL "$pid" 2>/dev/null
  fi
done
echo "✓ Stack processes stopped."
