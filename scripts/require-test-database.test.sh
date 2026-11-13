#!/usr/bin/env bash
# Tests for scripts/require-test-database.sh (the DATABASE_URL guard in front of
# every database-backed test command).
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
guard="$root_dir/scripts/require-test-database.sh"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

# Stubs that fail loudly and leave a marker: the guard must never call them.
stub_dir="$tmp_dir/bin"
marker="$tmp_dir/stub-called"
mkdir -p "$stub_dir"
for tool in psql docker pg_isready createdb; do
  cat >"$stub_dir/$tool" <<STUB
#!/usr/bin/env bash
echo "$tool \$*" >>"$marker"
exit 99
STUB
  chmod +x "$stub_dir/$tool"
done

# A working directory with a decoy .env that holds a valid test DSN. The guard
# only reads the process environment, so the decoy must not make it pass.
work_dir="$tmp_dir/work"
mkdir -p "$work_dir"
printf 'DATABASE_URL=postgres://u:p@localhost:5432/decoy_test\n' >"$work_dir/.env"

out="$tmp_dir/out"

# run_guard <url|__unset__>: prints the exit status.
run_guard() {
  local url=$1 status=0
  if [ "$url" = "__unset__" ]; then
    (cd "$work_dir" && env -i PATH="$stub_dir:/usr/bin:/bin" bash "$guard") >"$out" 2>&1 || status=$?
  else
    (cd "$work_dir" && env -i PATH="$stub_dir:/usr/bin:/bin" DATABASE_URL="$url" bash "$guard") >"$out" 2>&1 || status=$?
  fi
  echo "$status"
}

expect_rejected() {
  local label=$1 url=$2 want=${3:-}
  local status
  status=$(run_guard "$url")
  if [ "$status" -ne 1 ]; then
    fail "$label: exit status = $status, want 1 (output: $(cat "$out"))"
  fi
  if [ -z "$(cat "$out")" ]; then
    fail "$label: rejected without a reason"
  fi
  if [ -n "$want" ] && ! grep -Fqi "$want" "$out"; then
    fail "$label: reason does not mention '$want' (output: $(cat "$out"))"
  fi
}

expect_accepted() {
  local label=$1 url=$2 name=$3
  local status
  status=$(run_guard "$url")
  if [ "$status" -ne 0 ]; then
    fail "$label: exit status = $status, want 0 (output: $(cat "$out"))"
  fi
  if ! grep -Fxq "test database: $name" "$out"; then
    fail "$label: output is not 'test database: $name' (output: $(cat "$out"))"
  fi
}

expect_rejected "unset variable" "__unset__" "DATABASE_URL"
expect_rejected "empty variable" "" "DATABASE_URL"
expect_rejected "default database name" "postgres://u:p@localhost:5432/multica" "multica"
expect_rejected "default name with query" "postgres://u:p@localhost:5432/multica?sslmode=disable" "multica"
expect_rejected "default name upper case" "postgres://u:p@localhost:5432/MULTICA" "multica"
expect_rejected "name not ending in _test" "postgres://u:p@localhost:5432/multica_test_old" "_test"
expect_rejected "name with no prefix before _test" "postgres://u:p@localhost:5432/_test" "_test"
expect_rejected "empty name" "postgres://u:p@localhost:5432/" "name"
expect_rejected "no path" "postgres://u:p@localhost:5432" "name"
expect_rejected "key=value DSN" "host=localhost port=5432 dbname=app_test" "postgres"
expect_rejected "dbname override in query" "postgres://u:p@localhost:5432/app_test?dbname=multica" "dbname"
expect_rejected "database override in query" "postgres://u:p@localhost:5432/app_test?sslmode=disable&database=multica" "database"
expect_rejected "dbname override upper case" "postgres://u:p@localhost:5432/app_test?DBNAME=multica" "dbname"
expect_rejected "odd characters in name" "postgres://u:p@localhost:5432/app-test_test" "character"
expect_rejected "encoded characters in name" "postgres://u:p@localhost:5432/app%5Ftest" "character"
expect_rejected "extra path segment" "postgres://u:p@localhost:5432/app/app_test" "character"
expect_rejected "other scheme" "mysql://u:p@localhost:3306/app_test" "postgres"

expect_accepted "plain test name" "postgres://u:p@localhost:5432/app_test" "app_test"
expect_accepted "postgresql scheme with query" "postgresql://u:p@localhost:5432/app_test?sslmode=disable" "app_test"
expect_accepted "multica prefix test name" "postgres://u:p@localhost:5432/multica_test" "multica_test"

if [ -e "$marker" ]; then
  fail "the guard called an external database tool: $(cat "$marker")"
fi

echo "PASS: require-test-database"
