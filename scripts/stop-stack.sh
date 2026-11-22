#!/usr/bin/env bash
# Stops every local process of this checkout's Multica stack, so the next make dev-full starts clean:
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

# Processes whose command line names this checkout or the installed daemon. Skip this script, its
# parent shell, and `make` itself, which carries the repo path in its own arguments.
while read -r pid args; do
  case "$pid" in "$self" | "$parent") continue ;; esac
  case "$args" in make\ * | */make\ *) continue ;; esac
  # Shell wrappers such as `bash -c "cd <repo> && ..."` name the repo without running the stack.
  case "$args" in *sh\ -c\ *) continue ;; esac
  case "$args" in
    *"$root/"* | "$install daemon"*) add_pid "$pid" ;;
  esac
done < <(ps -eo pid=,args=)

for port in "$@"; do
  for pid in $(lsof -nP -iTCP:"$port" -sTCP:LISTEN -t 2>/dev/null); do
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
