#!/usr/bin/env bash
# Behaviour of the Hub object store section of scripts/dev-full.sh, with no containers started.
#
# docker and curl are stubbed on PATH. The script is sourced with DEV_FULL_LIB=1, which loads
# its functions without running the bootstrap. Port availability is replaced by a list, so
# the tests do not depend on what this machine has listening.
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

require_contains() {
  local file=$1 expected=$2
  if ! grep -Fq -- "$expected" "$file"; then
    echo "Expected output to contain: $expected" >&2
    echo "Observed:" >&2
    sed 's/^/  /' "$file" >&2
    exit 1
  fi
}

# Runs a command that must succeed, and shows what it printed when it does not. The command
# runs in a subshell because the script's fail() exits, and that must not end this test silently.
must_succeed() {
  local out="$tmp_dir/last-output"
  if ! ( "$@" ) >"$out" 2>&1; then
    echo "Command failed: $*" >&2
    sed 's/^/  /' "$out" >&2
    exit 1
  fi
}

require_absent() {
  local file=$1 unwanted=$2
  if grep -Fq -- "$unwanted" "$file"; then
    echo "Output must not contain: $unwanted" >&2
    sed 's/^/  /' "$file" >&2
    exit 1
  fi
}

stub_dir="$tmp_dir/bin"
mkdir -p "$stub_dir"
docker_log="$tmp_dir/docker.log"
curl_log="$tmp_dir/curl.log"
bucket_state="$tmp_dir/bucket-exists"

# Records each call. `compose port seaweedfs <port>` answers with the host port a running
# container publishes (DOCKER_PUBLISHED_API / _UI), and fails when none is set. `compose ps -q
# seaweedfs` names a running container when one is published, and `docker inspect` prints the
# environment lines in DOCKER_CONTAINER_ENV.
cat >"$stub_dir/docker" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$DOCKER_LOG"
if [ "${1:-}" = inspect ]; then
  cat "$DOCKER_CONTAINER_ENV"
  exit 0
fi
case " $* " in
  *" port seaweedfs 8333 "*)
    [ -n "${DOCKER_PUBLISHED_API:-}" ] || exit 1
    printf '127.0.0.1:%s\n' "$DOCKER_PUBLISHED_API"
    ;;
  *" port seaweedfs 9333 "*)
    [ -n "${DOCKER_PUBLISHED_UI:-}" ] || exit 1
    printf '127.0.0.1:%s\n' "$DOCKER_PUBLISHED_UI"
    ;;
  *" ps -q seaweedfs "*)
    [ -n "${DOCKER_PUBLISHED_API:-}" ] && printf 'cid-running\n'
    ;;
esac
exit 0
STUB
container_env="$tmp_dir/container-env"
printf 'HUB_S3_ACCESS_KEY=multica-hub\n' >"$container_env"

# Health checks are plain GETs with no -K config and always succeed. S3 requests arrive as a
# curl config on stdin. The stub logs argv and the config, answers HEAD/PUT from BUCKET_STATE,
# and returns CURL_FORCE as the status when it is set.
cat >"$stub_dir/curl" <<'STUB'
#!/usr/bin/env bash
case " $* " in
  *" -K "*) ;;
  *) exit 0 ;;
esac
config=$(cat)
printf 'ARGV %s\n%s\n' "$*" "$config" >>"$CURL_LOG"
if [ -n "${CURL_FORCE:-}" ]; then
  printf '%s' "$CURL_FORCE"
  [ "$CURL_FORCE" = 000 ] && exit 7
  exit 0
fi
method=$(sed -n 's/^request = "\(.*\)"$/\1/p' <<<"$config")
case "$method" in
  PUT)
    : >"$BUCKET_STATE"
    printf '200'
    ;;
  *)
    if [ -f "$BUCKET_STATE" ]; then printf '200'; else printf '404'; fi
    ;;
esac
STUB
chmod +x "$stub_dir/docker" "$stub_dir/curl"

export PATH="$stub_dir:$PATH"
export DOCKER_LOG="$docker_log" CURL_LOG="$curl_log" BUCKET_STATE="$bucket_state"
export DOCKER_CONTAINER_ENV="$container_env"

new_env() {
  ENV_FILE="$tmp_dir/env-$1"
  printf 'KEEP_ME=1\n' >"$ENV_FILE"
  : >"$docker_log"
  : >"$curl_log"
  rm -f "$bucket_state"
  unset DOCKER_PUBLISHED_API DOCKER_PUBLISHED_UI CURL_FORCE
}

# Loads the functions only. The script changes into the repository root when sourced, so the
# test sets its own working directory afterwards.
export DEV_FULL_LIB=1
# shellcheck disable=SC1091
. "$root_dir/scripts/dev-full.sh"
cd "$tmp_dir"
MULTICA_PROFILE_CONFIG="$tmp_dir/no-profile.json"
sleep() { :; }

[ ! -s "$docker_log" ] || fail "sourcing the script must not call docker"

# ---------- Secrets ----------
new_env secrets
fill_hub_secrets >"$tmp_dir/out-secrets" 2>&1
secret=$(env_value "$ENV_FILE" HUB_S3_SECRET_KEY)
[[ "$secret" =~ ^[0-9a-f]{48}$ ]] || fail "HUB_S3_SECRET_KEY should be 48 hex characters, got ${#secret} characters"
[ "$(env_value "$ENV_FILE" HUB_S3_ACCESS_KEY)" = multica-hub ] || fail "HUB_S3_ACCESS_KEY default missing"
[ "$(env_value "$ENV_FILE" HUB_S3_BUCKET)" = multica-hub ] || fail "HUB_S3_BUCKET default missing"
[ "$(env_value "$ENV_FILE" HUB_S3_REGION)" = us-east-1 ] || fail "HUB_S3_REGION default missing"
[ "$(env_value "$ENV_FILE" HUB_S3_USE_PATH_STYLE)" = true ] || fail "HUB_S3_USE_PATH_STYLE default missing"
require_absent "$tmp_dir/out-secrets" "$secret"
require_contains "$ENV_FILE" "KEEP_ME=1"

fill_hub_secrets >/dev/null 2>&1
[ "$(env_value "$ENV_FILE" HUB_S3_SECRET_KEY)" = "$secret" ] || fail "a re-run must keep the generated secret"

new_env existing
printf 'HUB_S3_SECRET_KEY=set-by-hand\n' >>"$ENV_FILE"
fill_hub_secrets >/dev/null 2>&1
[ "$(env_value "$ENV_FILE" HUB_S3_SECRET_KEY)" = set-by-hand ] || fail "a value already in the env file must not be replaced"

# ---------- Ports ----------
# Pretend 8333 and 8334 are taken by other programs.
port_is_free() {
  case "$1" in
    8333|8334) return 1 ;;
    *) return 0 ;;
  esac
}

new_env ports
must_succeed resolve_hub_ports
[ "$(env_value "$ENV_FILE" HUB_S3_API_PORT)" = 8335 ] || fail "API port should be the first free port (8335), got $(env_value "$ENV_FILE" HUB_S3_API_PORT)"
[ "$(env_value "$ENV_FILE" HUB_S3_UI_PORT)" = 9333 ] || fail "UI port should be 9333 when free, got $(env_value "$ENV_FILE" HUB_S3_UI_PORT)"
require_contains "$ENV_FILE" "KEEP_ME=1"
must_succeed resolve_hub_ports
[ "$(env_value "$ENV_FILE" HUB_S3_API_PORT)" = 8335 ] || fail "a re-run must keep the API port"
[ "$(grep -c '^HUB_S3_API_PORT=' "$ENV_FILE")" = 1 ] || fail "a re-run must not duplicate HUB_S3_API_PORT"

new_env ports-explicit-busy
printf 'HUB_S3_API_PORT=8333\n' >>"$ENV_FILE"
set +e
(resolve_hub_ports) >"$tmp_dir/out-busy" 2>&1
status=$?
set -e
[ "$status" -ne 0 ] || fail "an explicit busy HUB_S3_API_PORT must stop the run"
require_contains "$tmp_dir/out-busy" "HUB_S3_API_PORT=8333 in $ENV_FILE is already in use"

new_env ports-none-free
set +e
(port_is_free() { return 1; }; free_port 8333 >/dev/null)
status=$?
set -e
[ "$status" -ne 0 ] || fail "free_port must fail when no port in its range is free"

# A container that already publishes ports is adopted, and the env file follows it. Recreating
# it on other ports would stop the store for the other checkout that shares the project.
new_env ports-adopt
printf 'HUB_S3_API_PORT=8333\nHUB_S3_UI_PORT=9333\n' >>"$ENV_FILE"
DOCKER_PUBLISHED_API=18444 DOCKER_PUBLISHED_UI=19444 must_succeed resolve_hub_ports
[ "$(env_value "$ENV_FILE" HUB_S3_API_PORT)" = 18444 ] || fail "a running container's API port must be adopted"
[ "$(env_value "$ENV_FILE" HUB_S3_UI_PORT)" = 19444 ] || fail "a running container's UI port must be adopted"
require_contains "$tmp_dir/last-output" "already running on ports 18444 and 19444"
if grep -Eq ' up |up -d' "$docker_log"; then
  fail "resolving ports must not start or recreate containers"
fi

# A second checkout must take the running store's credentials as well, or its S3 requests
# would fail after the first checkout's container is in use. The store's key contains sed's
# special characters, which the env file must keep as they are.
store_key='store&key\with-chars'
printf 'HUB_S3_SECRET_KEY=%s\n' "$store_key" >"$container_env"
new_env ports-adopt-credentials
printf 'HUB_S3_API_PORT=8333\nHUB_S3_UI_PORT=9333\nHUB_S3_ACCESS_KEY=multica-hub\nHUB_S3_SECRET_KEY=this-checkout-generated\n' >>"$ENV_FILE"
printf 'HUB_S3_ACCESS_KEY=multica-hub\nHUB_S3_SECRET_KEY=%s\n' "$store_key" >"$container_env"
DOCKER_PUBLISHED_API=18444 DOCKER_PUBLISHED_UI=19444 must_succeed resolve_hub_ports
[ "$(env_value "$ENV_FILE" HUB_S3_SECRET_KEY)" = "$store_key" ] || fail "the running store's secret must be adopted, byte for byte"
[ "$(env_value "$ENV_FILE" HUB_S3_ACCESS_KEY)" = multica-hub ] || fail "the access key must be kept"
[ "$(grep -c '^HUB_S3_SECRET_KEY=' "$ENV_FILE")" = 1 ] || fail "adoption must replace the secret line, not add one"
require_contains "$ENV_FILE" "KEEP_ME=1"
require_absent "$tmp_dir/last-output" "$store_key"
require_contains "$tmp_dir/last-output" "Set HUB_S3_SECRET_KEY in $ENV_FILE to the value the running SeaweedFS uses"
printf 'HUB_S3_ACCESS_KEY=multica-hub\n' >"$container_env"

# ---------- Endpoint ----------
new_env endpoint
printf 'HUB_S3_API_PORT=8335\n' >>"$ENV_FILE"
resolve_hub_urls >/dev/null 2>&1
[ "$(env_value "$ENV_FILE" HUB_S3_ENDPOINT)" = http://localhost:8335 ] || fail "endpoint should follow the published port"

new_env endpoint-compose-name
printf 'HUB_S3_API_PORT=8335\nHUB_S3_ENDPOINT=http://seaweedfs:8333\n' >>"$ENV_FILE"
set +e
(resolve_hub_urls) >"$tmp_dir/out-endpoint" 2>&1
status=$?
set -e
[ "$status" -ne 0 ] || fail "an endpoint that names the compose service must be refused"
require_contains "$tmp_dir/out-endpoint" "uses the Docker network name"

new_env endpoint-external
printf 'HUB_S3_API_PORT=8335\nHUB_S3_ENDPOINT=https://seaweedfs.example.com\n' >>"$ENV_FILE"
resolve_hub_urls >/dev/null 2>&1
[ "$(env_value "$ENV_FILE" HUB_S3_ENDPOINT)" = https://seaweedfs.example.com ] || fail "an external host that contains seaweedfs must be kept"

# A loopback endpoint on a stale port follows the published port, so the backend and the
# bucket check reach the store that is running.
new_env endpoint-stale-loopback
printf 'HUB_S3_API_PORT=8336\nHUB_S3_ENDPOINT=http://127.0.0.1:8333\n' >>"$ENV_FILE"
must_succeed resolve_hub_urls
[ "$(env_value "$ENV_FILE" HUB_S3_ENDPOINT)" = http://localhost:8336 ] || fail "a stale loopback endpoint must follow HUB_S3_API_PORT"
require_contains "$tmp_dir/last-output" "Set HUB_S3_ENDPOINT to http://localhost:8336"

# ---------- Bucket and S3 requests ----------
new_env bucket-missing
fill_hub_secrets >/dev/null 2>&1
resolve_hub_ports >/dev/null 2>&1
resolve_hub_urls >/dev/null 2>&1
secret=$(env_value "$ENV_FILE" HUB_S3_SECRET_KEY)
ensure_hub_bucket >"$tmp_dir/out-bucket" 2>&1
require_contains "$tmp_dir/out-bucket" "bucket multica-hub created"
[ "$HUB_STORE_STATUS" = "running; bucket multica-hub created" ] || fail "status should say the bucket was created, got: $HUB_STORE_STATUS"
require_contains "$curl_log" 'request = "PUT"'
require_absent "$tmp_dir/out-bucket" "$secret"
if grep '^ARGV' "$curl_log" | grep -Fq -- "$secret"; then
  fail "the secret must not appear in curl's argument list"
fi
require_contains "$curl_log" 'aws-sigv4 = "aws:amz:us-east-1:s3"'
require_contains "$curl_log" "user = \"multica-hub:$secret\""

new_env bucket-exists
fill_hub_secrets >/dev/null 2>&1
resolve_hub_ports >/dev/null 2>&1
resolve_hub_urls >/dev/null 2>&1
: >"$bucket_state"
ensure_hub_bucket >"$tmp_dir/out-exists" 2>&1
require_contains "$tmp_dir/out-exists" "bucket multica-hub exists"
if grep -Fq 'request = "PUT"' "$curl_log"; then
  fail "an existing bucket must not be created again"
fi

new_env bucket-forbidden
fill_hub_secrets >/dev/null 2>&1
resolve_hub_ports >/dev/null 2>&1
resolve_hub_urls >/dev/null 2>&1
set +e
(export CURL_FORCE=403; ensure_hub_bucket) >"$tmp_dir/out-403" 2>&1
status=$?
set -e
[ "$status" -ne 0 ] || fail "HTTP 403 must stop the run"
require_contains "$tmp_dir/out-403" "rejected HUB_S3_ACCESS_KEY and HUB_S3_SECRET_KEY"

new_env bucket-unreachable
fill_hub_secrets >/dev/null 2>&1
resolve_hub_ports >/dev/null 2>&1
resolve_hub_urls >/dev/null 2>&1
set +e
(export CURL_FORCE=000; ensure_hub_bucket) >"$tmp_dir/out-000" 2>&1
status=$?
set -e
[ "$status" -ne 0 ] || fail "an unreachable endpoint must stop the run"
require_contains "$tmp_dir/out-000" "did not answer its S3 API"

# A secret with a quote and a backslash must reach curl's config escaped, not broken.
new_env escaping
printf 'HUB_S3_SECRET_KEY=a"b\\c\n' >>"$ENV_FILE"
fill_hub_secrets >/dev/null 2>&1
resolve_hub_ports >/dev/null 2>&1
resolve_hub_urls >/dev/null 2>&1
: >"$bucket_state"
ensure_hub_bucket >/dev/null 2>&1
require_contains "$curl_log" 'user = "multica-hub:a\"b\\c"'

# ---------- Start ----------
new_env start
fill_hub_secrets >/dev/null 2>&1
resolve_hub_ports >/dev/null 2>&1
resolve_hub_urls >/dev/null 2>&1
: >"$bucket_state"
start_hub_store >"$tmp_dir/out-start" 2>&1
require_contains "$docker_log" "--profile knowledge up -d seaweedfs"
require_contains "$tmp_dir/out-start" "SeaweedFS answers at http://localhost:9333/cluster/status"
require_contains "$tmp_dir/out-start" "bucket multica-hub exists"

# ---------- Summary ----------
new_env summary
printf 'HUB_S3_API_PORT=18333\nHUB_S3_UI_PORT=19333\n' >>"$ENV_FILE"
fill_hub_secrets >/dev/null 2>&1
resolve_hub_ports >/dev/null 2>&1
resolve_hub_urls >/dev/null 2>&1
set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a
PORT=18080 FRONTEND_PORT=13000 KNOWLEDGE_URL=http://localhost:8090 KNOWLEDGE_UI_URL=http://localhost:8085
HUB_STORE_STATUS="running; bucket multica-hub exists"
print_summary >"$tmp_dir/out-summary" 2>&1
require_contains "$tmp_dir/out-summary" "Hub store     http://localhost:18333  (S3 API, bucket multica-hub)"
require_contains "$tmp_dir/out-summary" "SeaweedFS UI  http://localhost:19333  (cluster status: /cluster/status)"
require_contains "$tmp_dir/out-summary" "Running       Multica API, Multica web, WeKnora (knowledge-postgres knowledge-redis knowledge-docreader knowledge-app knowledge-ui), SeaweedFS (seaweedfs)"
require_contains "$tmp_dir/out-summary" "compose --env-file $ENV_FILE -f docker-compose.selfhost.yml --profile knowledge stop knowledge-postgres knowledge-redis knowledge-docreader knowledge-app knowledge-ui seaweedfs"
require_absent "$tmp_dir/out-summary" "$(env_value "$ENV_FILE" HUB_S3_SECRET_KEY)"

echo "dev-full.test.sh: all checks passed"
