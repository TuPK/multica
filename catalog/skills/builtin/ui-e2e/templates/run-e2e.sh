#!/usr/bin/env bash
# Runs the UI e2e suite and writes a log with an exit line.
# Usage: run-e2e.sh <logFile> [specPath] [--project=chromium]
# Needs: the test database URL in the environment or a dotenv file, read by the test setup.
set -u
LOG="${1:?usage: run-e2e.sh <logFile> [specPath]}"
SPEC="${2:-e2e}"
mkdir -p "$(dirname "$LOG")"
# Playwright's own binary, called directly: the npx wrapper hung under load in one past run.
node node_modules/@playwright/test/cli.js test "$SPEC" --project=chromium --reporter=list > "$LOG" 2>&1
echo "PLAYWRIGHT_EXIT=$?" >> "$LOG"
# Summary lines for the report
sed 's/\x1b\[[0-9;]*[A-Za-z]//g' "$LOG" | grep -E ' passed| failed| skipped|PLAYWRIGHT_EXIT' | tail -n 5
