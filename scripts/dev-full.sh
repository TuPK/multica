#!/usr/bin/env bash
# Fresh-machine bootstrap: installs and configures Hermes, starts the WeKnora
# knowledge services and the SeaweedFS Hub object store, fills in the secrets that
# are still empty, then runs `make dev` (migrations, API, web) and starts the daemon
# when a Multica login exists. Re-running is safe: values already in the env file are
# kept, existing config files are backed up before they are changed, and containers
# that already exist are not stopped.
#
# The Multica account, the first workspace and the CLI login are browser steps.
# They are described in docs/engineering/runbooks/fresh-machine-install.md.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

GUIDE="docs/engineering/runbooks/fresh-machine-install.md"
COMPOSE_FILE="docker-compose.selfhost.yml"
KNOWLEDGE_SERVICES=(knowledge-postgres knowledge-redis knowledge-docreader knowledge-app knowledge-ui)
# The Hub object store (SeaweedFS). It has no compose profile, and it is shared by
# every checkout that uses the same compose project, as the WeKnora services are.
HUB_SERVICES=(seaweedfs)
# Ports SeaweedFS listens on inside its container: the S3 API and the master, whose
# /cluster/status is the health URL and whose page is the console.
HUB_DEFAULT_API_PORT=8333
HUB_DEFAULT_UI_PORT=9333
REQUIRED_KNOWLEDGE=(
  KNOWLEDGE_SERVICE_EMAIL
  KNOWLEDGE_SERVICE_PASSWORD
  KNOWLEDGE_LLM_BASE_URL
  KNOWLEDGE_LLM_API_KEY
  KNOWLEDGE_LLM_MODEL
  KNOWLEDGE_VISION_MODEL
  KNOWLEDGE_EMBEDDING_URL
  KNOWLEDGE_EMBEDDING_MODEL
)
HERMES_KEYS=(HERMES_BASE_URL HERMES_MODEL HERMES_API_KEY)
# The official installer, pinned to the Hermes version this repo was verified with
# (Hermes Agent v0.22.5). The installer takes --commit, so the pin is exact.
HERMES_INSTALL_URL="https://hermes-agent.nousresearch.com/install.sh"
HERMES_PIN_COMMIT="bf0203a794a07aac3ff764972ab9087baa4e4f4e"
# Polls of 3 seconds each. A first run on a fresh clone installs node modules, downloads
# the Go toolchain and builds the API before /health answers, which can take 20 minutes.
WAIT_TRIES=480
HERMES_STATUS="not installed"
HUB_STORE_STATUS="not started"
DAEMON_STATUS="not started"

info() { echo "==> $*"; }
ok() { echo "✓ $*"; }
fail() { echo "✗ $*" >&2; exit 1; }

# ---------- Prerequisites (same checks as scripts/dev.sh, plus compose) ----------
check_prerequisites() {
  local missing=() tool
  for tool in node pnpm go docker openssl curl; do
    command -v "$tool" >/dev/null 2>&1 || missing+=("$tool")
  done
  if [ ${#missing[@]} -gt 0 ]; then
    fail "Missing prerequisites: ${missing[*]}
  Please install: Node.js 22, pnpm 10.28.2, Go 1.26.9, Docker with the compose plugin
  Install steps: $GUIDE#prerequisites"
  fi
  docker compose version >/dev/null 2>&1 \
    || fail "Docker Compose ('docker compose') was not found. Install the compose plugin: $GUIDE#prerequisites"
  docker info >/dev/null 2>&1 || fail "The Docker daemon is not running. Start Docker and run make dev-full again."
  ok "Prerequisites found"
}

# ---------- Env file: same resolution as scripts/dev.sh ----------
resolve_env_file() {
  if [ -f .git ]; then
    # A linked worktree. init-worktree-env.sh gives it its own database and ports;
    # copying .env.example would reuse the main checkout's database.
    ENV_FILE=".env.worktree"
    if [ ! -f "$ENV_FILE" ]; then
      info "Worktree detected. Generating $ENV_FILE..."
      bash scripts/init-worktree-env.sh "$ENV_FILE"
    fi
  else
    ENV_FILE=".env"
    if [ ! -f "$ENV_FILE" ]; then
      info "Creating $ENV_FILE from .env.example..."
      cp .env.example "$ENV_FILE"
    fi
  fi
  ok "Using $ENV_FILE"
}

# Value of the last uncommented KEY= line in a file; empty when unset or commented out.
env_value() {
  local line
  line=$(grep -E "^$2=" "$1" | tail -n 1 || true)
  printf '%s' "${line#"$2="}"
}

# Sets KEY to VALUE, replacing the line when KEY is present, and appending it otherwise.
# awk reads the key and value from the environment, so a value with sed's special
# characters (such as & or \) is written as it is. The file keeps its mode.
set_env_value() {
  local key=$1 value=$2 tmp
  if grep -qE "^${key}=" "$ENV_FILE"; then
    tmp=$(mktemp "${ENV_FILE}.XXXXXX")
    KEY="$key" VALUE="$value" awk \
      'index($0, ENVIRON["KEY"] "=") == 1 { print ENVIRON["KEY"] "=" ENVIRON["VALUE"]; next } { print }' \
      "$ENV_FILE" >"$tmp"
    cat "$tmp" >"$ENV_FILE"
    rm -f "$tmp"
  else
    if [ -s "$ENV_FILE" ] && [ -n "$(tail -c 1 "$ENV_FILE")" ]; then
      printf '\n' >> "$ENV_FILE"
    fi
    printf '%s=%s\n' "$key" "$value" >> "$ENV_FILE"
  fi
}

# Sets KEY to VALUE only when KEY is empty or absent. Values are never printed.
set_if_empty() {
  local key=$1 value=$2
  if [ -n "$(env_value "$ENV_FILE" "$key")" ]; then
    return 0
  fi
  set_env_value "$key" "$value"
  ok "Set $key in $ENV_FILE"
}

# Every compose call goes through here. Compose refuses to parse the file unless the
# backend's JWT_SECRET is set (a `:?` check), even though the backend never starts
# here. When the env file has no JWT_SECRET, a throwaway value is passed to this one
# compose process; it is not written to the env file, because Multica's own secret
# must not change on a machine that already runs.
compose() {
  local jwt
  jwt=$(env_value "$ENV_FILE" JWT_SECRET)
  JWT_SECRET="${jwt:-$(openssl rand -hex 32)}" \
    docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" --profile knowledge "$@"
}

# ---------- Knowledge secrets: generated only where the env file is empty ----------
fill_knowledge_secrets() {
  set_if_empty KNOWLEDGE_SYSTEM_AES_KEY "$(openssl rand -hex 16)"
  set_if_empty KNOWLEDGE_JWT_SECRET "$(openssl rand -hex 32)"
  set_if_empty MULTICA_KNOWLEDGE_SECRET_KEY "$(openssl rand -base64 32)"
  set_if_empty KNOWLEDGE_REDIS_PASSWORD "$(openssl rand -hex 16)"
  set_if_empty KNOWLEDGE_DB_PASSWORD "$(openssl rand -hex 16)"
}

# The Hub store's credentials are written once and kept. SeaweedFS reads them when its
# container starts, and the host-run backend reads the same values from this file.
# The compose file refuses to parse without HUB_S3_SECRET_KEY, so this runs before any
# compose call.
fill_hub_secrets() {
  set_if_empty HUB_S3_ACCESS_KEY "multica-hub"
  set_if_empty HUB_S3_SECRET_KEY "$(openssl rand -hex 24)"
  set_if_empty HUB_S3_BUCKET "multica-hub"
  set_if_empty HUB_S3_REGION "us-east-1"
  set_if_empty HUB_S3_USE_PATH_STYLE "true"
  # Encrypts the access tokens of private Hub sources at rest (base64, 32 bytes).
  set_if_empty MULTICA_HUB_SECRET_KEY "$(openssl rand -base64 32)"
}

# Postgres applies POSTGRES_PASSWORD only when its data volume is created, and the
# volume outlives containers. Set the role's password to KNOWLEDGE_DB_PASSWORD so
# the volume and the env file agree. The local socket is trust-authenticated inside
# the container, so the password is sent on stdin, never on a command line.
sync_knowledge_db_password() {
  local cid pw i
  cid=$(compose ps -q knowledge-postgres)
  [ -n "$cid" ] || fail "knowledge-postgres is not running, so its password cannot be set."
  pw=$(env_value "$ENV_FILE" KNOWLEDGE_DB_PASSWORD)
  pw=${pw//\'/\'\'}
  for i in $(seq 1 60); do
    docker exec "$cid" pg_isready -q && break
    sleep 2
  done
  printf "ALTER USER weknora WITH PASSWORD '%s';\n" "$pw" \
    | docker exec -i "$cid" sh -c 'psql -v ON_ERROR_STOP=1 -q -U "$POSTGRES_USER" -d postgres -f -' >/dev/null \
    || fail "Could not set the WeKnora database password. Logs: $(compose_cli_hint) logs --tail 100 knowledge-postgres"
}

# ---------- Values this script cannot invent ----------
explain_knowledge_key() {
  case "$1" in
    KNOWLEDGE_SERVICE_EMAIL) echo "WeKnora login that Multica signs in as to create one WeKnora workspace per Multica workspace" ;;
    KNOWLEDGE_SERVICE_PASSWORD) echo "Password for that WeKnora login (8 to 32 letters and digits)" ;;
    KNOWLEDGE_LLM_BASE_URL) echo "OpenAI-compatible chat endpoint for summaries, Wiki and reranking, such as the 9router gateway" ;;
    KNOWLEDGE_LLM_API_KEY) echo "Key for the chat endpoint" ;;
    KNOWLEDGE_LLM_MODEL) echo "Chat model name that the endpoint serves" ;;
    KNOWLEDGE_VISION_MODEL) echo "Vision model name that the endpoint serves; Setup > Models tests it" ;;
    KNOWLEDGE_EMBEDDING_URL) echo "Embedding endpoint WeKnora calls; Ollama on the Docker host is http://host.docker.internal:11434" ;;
    KNOWLEDGE_EMBEDDING_MODEL) echo "Embedding model, fixed for each knowledge base (for example bge-m3)" ;;
    HERMES_BASE_URL) echo "OpenAI-compatible endpoint Hermes sends its chat requests to, usually ending in /v1" ;;
    HERMES_MODEL) echo "Model name that endpoint serves, written as Hermes's default model" ;;
    HERMES_API_KEY) echo "Key for the Hermes endpoint; it is stored in ~/.hermes/.env, not in config.yaml" ;;
  esac
}

check_required_knowledge() {
  local key missing=()
  for key in "${REQUIRED_KNOWLEDGE[@]}"; do
    if [ -z "$(env_value "$ENV_FILE" "$key")" ]; then
      missing+=("$key")
    fi
  done
  if [ ${#missing[@]} -eq 0 ]; then
    return 0
  fi
  echo "✗ Nothing was started. These values in $ENV_FILE are empty:" >&2
  for key in "${missing[@]}"; do
    echo "    $key" >&2
    echo "        $(explain_knowledge_key "$key")" >&2
  done
  echo "  Fill them in and run make dev-full again." >&2
  echo "  Variables: $GUIDE#environment-variables" >&2
  echo "  Gateway and endpoints: $GUIDE#the-9router-gateway" >&2
  exit 1
}

# Derives the URLs the host-run backend and the summary use. KNOWLEDGE_URL must be
# the published port: make dev runs on this machine, where the compose service name
# does not resolve.
resolve_knowledge_urls() {
  local api_port ui_port url
  api_port=$(env_value "$ENV_FILE" KNOWLEDGE_API_PORT)
  ui_port=$(env_value "$ENV_FILE" KNOWLEDGE_UI_PORT)
  set_if_empty KNOWLEDGE_URL "http://localhost:${api_port:-8090}"
  set_if_empty KNOWLEDGE_UI_URL "http://localhost:${ui_port:-8085}"
  url=$(env_value "$ENV_FILE" KNOWLEDGE_URL)
  case "$url" in
    *knowledge-app*)
      fail "KNOWLEDGE_URL=$url uses the Docker network name. make dev runs Multica on this machine, so set KNOWLEDGE_URL=http://localhost:${api_port:-8090} in $ENV_FILE and run make dev-full again."
      ;;
  esac
}

load_env() {
  set -a
  # shellcheck disable=SC1090
  . "$ENV_FILE"
  set +a
  # shellcheck disable=SC1091
  . scripts/local-env.sh
}

# Polls a URL until it answers. Returns 1 on timeout, or when the process given as
# the optional third argument has exited.
wait_for_url() {
  local label=$1 url=$2 pid=${3:-} tries=0
  until curl -fsS --max-time 3 -o /dev/null "$url" 2>/dev/null; do
    if [ -n "$pid" ] && ! kill -0 "$pid" 2>/dev/null; then
      echo "! $label: the process stopped before $url answered" >&2
      return 1
    fi
    tries=$((tries + 1))
    if [ "$tries" -ge "$WAIT_TRIES" ]; then
      echo "! $label: $url did not answer after about $((WAIT_TRIES * 3 / 60)) minutes" >&2
      return 1
    fi
    sleep 3
  done
  ok "$label answers at $url"
}

# JSON string body for a value from the env file. Quotes and backslashes are escaped.
json_string() {
  printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'
}

# Registers the WeKnora login that Multica signs in as. WeKnora answers 201 for a new user
# and 400 "user with this email already exists" for one that is already there. A new
# user becomes a system administrator only when knowledge-app starts (the bootstrap reads
# WEKNORA_BOOTSTRAP_SYSTEM_ADMIN_EMAIL then), so a new user is followed by one restart.
register_knowledge_service_user() {
  local url email password body status reply
  url=$(env_value "$ENV_FILE" KNOWLEDGE_URL)
  email=$(env_value "$ENV_FILE" KNOWLEDGE_SERVICE_EMAIL)
  password=$(env_value "$ENV_FILE" KNOWLEDGE_SERVICE_PASSWORD)
  body="{\"username\":\"multica-service\",\"email\":\"$(json_string "$email")\",\"password\":\"$(json_string "$password")\"}"
  reply=$(mktemp)
  status=$(curl -s -m 30 -o "$reply" -w '%{http_code}' -X POST "$url/api/v1/auth/register" \
    -H 'Content-Type: application/json' -d "$body" || echo 000)
  if [ "$status" = "201" ] || [ "$status" = "200" ]; then
    rm -f "$reply"
    ok "WeKnora service user created: $email"
    info "Restarting knowledge-app once so WeKnora makes the service user a system administrator"
    compose restart knowledge-app >/dev/null
    wait_for_url "WeKnora" "$url/health" || fail "WeKnora is not healthy after the restart. Logs: $(compose_cli_hint) logs --tail 100 knowledge-app"
  elif grep -q "already exists" "$reply"; then
    rm -f "$reply"
    ok "WeKnora service user already exists: $email"
  else
    echo "! Could not register the WeKnora service user (HTTP $status). Knowledge will say the service account was rejected. Check KNOWLEDGE_SERVICE_EMAIL and KNOWLEDGE_SERVICE_PASSWORD, then run make dev-full again." >&2
    rm -f "$reply"
  fi
}

start_knowledge() {
  local url
  url=$(env_value "$ENV_FILE" KNOWLEDGE_URL)
  info "Starting WeKnora: ${KNOWLEDGE_SERVICES[*]}"
  info "The first start pulls the pinned WeKnora images, which can take several minutes."
  # The database goes first so its password is in place before knowledge-app connects.
  compose up -d knowledge-postgres knowledge-redis knowledge-docreader
  sync_knowledge_db_password
  compose up -d "${KNOWLEDGE_SERVICES[@]}"
  wait_for_url "WeKnora" "$url/health" || fail "WeKnora is not healthy. Logs: $(compose_cli_hint) logs --tail 100 knowledge-app. Troubleshooting: $GUIDE#troubleshooting"
  register_knowledge_service_user
}

# The command a person types by hand. Compose needs JWT_SECRET to parse the file; the
# value is unused for the knowledge services, so any placeholder works when the env
# file leaves it empty.
compose_cli_hint() {
  echo "JWT_SECRET=unused docker compose --env-file $ENV_FILE -f $COMPOSE_FILE --profile knowledge"
}

### Hub object store: SeaweedFS, which the host-run backend reaches on a published port.
### Ports are free-by-default and persisted; a running container's ports always win.

# Host port that a running container publishes for CONTAINER_PORT, or nothing.
published_port() {
  local out
  out=$(compose port "$1" "$2" 2>/dev/null) || return 0
  printf '%s' "${out##*:}"
}

# True when nothing listens on 127.0.0.1:PORT.
port_is_free() {
  ! (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null
}

# First free port from START, skipping SKIP. Prints nothing and fails when there is none.
free_port() {
  local port=$1 skip=${2:-} end
  end=$((port + 100))
  while [ "$port" -lt "$end" ]; do
    if [ "$port" != "$skip" ] && port_is_free "$port"; then
      printf '%s' "$port"
      return 0
    fi
    port=$((port + 1))
  done
  return 1
}

# A value of a running container's environment, or nothing. Read with docker inspect, so the
# value is never printed.
container_env() {
  docker inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "$1" \
    | sed -n "s/^$2=//p" | tail -n 1
}

# Sets KEY to VALUE when VALUE is known and differs. The value is never printed.
adopt_env_value() {
  local key=$1 value=$2
  if [ -z "$value" ] || [ "$(env_value "$ENV_FILE" "$key")" = "$value" ]; then
    return 0
  fi
  set_env_value "$key" "$value"
  ok "Set $key in $ENV_FILE to the value the running SeaweedFS uses"
}

# Writes the store's ports and credentials to the env file. A running SeaweedFS container
# wins: this checkout and the other worktrees share one compose project, so a container that
# is already running is adopted, not recreated. Recreating it with this checkout's ports or
# key would stop the store for the other checkout. Otherwise an explicit value is kept (and
# must be free), and an empty one takes the first free port. Re-runs keep the same values.
resolve_hub_ports() {
  local api ui cid
  api=$(published_port seaweedfs "$HUB_DEFAULT_API_PORT")
  ui=$(published_port seaweedfs "$HUB_DEFAULT_UI_PORT")
  if [ -n "$api" ] && [ -n "$ui" ]; then
    cid=$(compose ps -q seaweedfs)
    adopt_env_value HUB_S3_ACCESS_KEY "$(container_env "$cid" HUB_S3_ACCESS_KEY)"
    adopt_env_value HUB_S3_SECRET_KEY "$(container_env "$cid" HUB_S3_SECRET_KEY)"
    info "SeaweedFS is already running on ports $api and $ui; using its ports and credentials"
  else
    api=$(env_value "$ENV_FILE" HUB_S3_API_PORT)
    if [ -z "$api" ]; then
      if ! api=$(free_port "$HUB_DEFAULT_API_PORT"); then
        fail "No free port from $HUB_DEFAULT_API_PORT for the Hub store. Set HUB_S3_API_PORT in $ENV_FILE and run make dev-full again."
      fi
    elif ! port_is_free "$api"; then
      fail "HUB_S3_API_PORT=$api in $ENV_FILE is already in use. Choose another port there and run make dev-full again."
    fi
    ui=$(env_value "$ENV_FILE" HUB_S3_UI_PORT)
    if [ -z "$ui" ]; then
      if ! ui=$(free_port "$HUB_DEFAULT_UI_PORT" "$api"); then
        fail "No free port from $HUB_DEFAULT_UI_PORT for the SeaweedFS UI. Set HUB_S3_UI_PORT in $ENV_FILE and run make dev-full again."
      fi
    elif ! port_is_free "$ui"; then
      fail "HUB_S3_UI_PORT=$ui in $ENV_FILE is already in use. Choose another port there and run make dev-full again."
    fi
  fi
  if [ "$(env_value "$ENV_FILE" HUB_S3_API_PORT)" != "$api" ]; then
    set_env_value HUB_S3_API_PORT "$api"
    ok "Set HUB_S3_API_PORT to $api in $ENV_FILE"
  fi
  if [ "$(env_value "$ENV_FILE" HUB_S3_UI_PORT)" != "$ui" ]; then
    set_env_value HUB_S3_UI_PORT "$ui"
    ok "Set HUB_S3_UI_PORT to $ui in $ENV_FILE"
  fi
}

# The endpoint the host-run backend uses. Multica runs on this machine, where the compose
# service name does not resolve, so the published port is used (the same rule as
# KNOWLEDGE_URL). A value that names the compose service is refused. A loopback value on
# another port is moved to the published port, because the store may have been recreated
# on other ports since the env file was written. Other hosts are kept as they are.
resolve_hub_urls() {
  local api endpoint
  api=$(env_value "$ENV_FILE" HUB_S3_API_PORT)
  set_if_empty HUB_S3_ENDPOINT "http://localhost:${api}"
  endpoint=$(env_value "$ENV_FILE" HUB_S3_ENDPOINT)
  case "$endpoint" in
    http://seaweedfs|http://seaweedfs:*|http://seaweedfs/*)
      fail "HUB_S3_ENDPOINT=$endpoint uses the Docker network name. make dev runs Multica on this machine, so set HUB_S3_ENDPOINT=http://localhost:${api} in $ENV_FILE and run make dev-full again."
      ;;
    http://localhost:"$api"|http://127.0.0.1:"$api")
      ;;
    http://localhost:*|http://127.0.0.1:*)
      set_env_value HUB_S3_ENDPOINT "http://localhost:${api}"
      ok "Set HUB_S3_ENDPOINT to http://localhost:${api} to match the published SeaweedFS port"
      ;;
  esac
}

# One S3 request signed with the Hub credentials. Prints the HTTP status, 000 when the
# endpoint does not answer. The request is passed to curl as a config file on stdin, so
# the secret never reaches the process list.
hub_s3_status() {
  local method=$1 path=$2 ak sk region endpoint
  ak=$(env_value "$ENV_FILE" HUB_S3_ACCESS_KEY)
  sk=$(env_value "$ENV_FILE" HUB_S3_SECRET_KEY)
  region=$(env_value "$ENV_FILE" HUB_S3_REGION)
  endpoint=$(env_value "$ENV_FILE" HUB_S3_ENDPOINT)
  printf 'url = "%s%s"\nrequest = "%s"\nuser = "%s:%s"\naws-sigv4 = "aws:amz:%s:s3"\nsilent\nmax-time = 10\noutput = /dev/null\nwrite-out = "%%{http_code}"\n' \
    "$(json_string "$endpoint")" "$(json_string "$path")" "$method" \
    "$(json_string "$ak")" "$(json_string "$sk")" "$(json_string "$region")" \
    | curl -K - 2>/dev/null || true
}

# Confirms that the backend's bucket exists, and creates it when it does not. The backend
# creates the bucket at startup too; this check says so before Multica starts.
ensure_hub_bucket() {
  local bucket endpoint code i
  bucket=$(env_value "$ENV_FILE" HUB_S3_BUCKET)
  endpoint=$(env_value "$ENV_FILE" HUB_S3_ENDPOINT)
  code=000
  for i in $(seq 1 60); do
    code=$(hub_s3_status HEAD "/$bucket")
    [ "$code" != 000 ] && break
    sleep 3
  done
  case "$code" in
    200)
      HUB_STORE_STATUS="running; bucket $bucket exists"
      ok "S3 answers at $endpoint; bucket $bucket exists"
      ;;
    404)
      code=$(hub_s3_status PUT "/$bucket")
      if [ "$code" != 200 ]; then
        fail "Could not create bucket $bucket at $endpoint (HTTP $code). Logs: $(compose_cli_hint) logs --tail 100 seaweedfs"
      fi
      HUB_STORE_STATUS="running; bucket $bucket created"
      ok "S3 answers at $endpoint; bucket $bucket created"
      ;;
    403)
      fail "SeaweedFS rejected HUB_S3_ACCESS_KEY and HUB_S3_SECRET_KEY in $ENV_FILE (HTTP 403). The container keeps the credentials it started with. See the credential rotation steps in docs/engineering/runbooks/hub-object-storage.md, section 5."
      ;;
    000)
      fail "SeaweedFS did not answer its S3 API at $endpoint after about 3 minutes. Logs: $(compose_cli_hint) logs --tail 100 seaweedfs. Check that HUB_S3_API_PORT is published: docker ps"
      ;;
    *)
      fail "SeaweedFS answered bucket $bucket with HTTP $code. Logs: $(compose_cli_hint) logs --tail 100 seaweedfs"
      ;;
  esac
}

start_hub_store() {
  local ui
  ui=$(env_value "$ENV_FILE" HUB_S3_UI_PORT)
  info "Starting the Hub object store: ${HUB_SERVICES[*]}"
  info "The first start pulls the pinned SeaweedFS image."
  compose up -d "${HUB_SERVICES[@]}"
  wait_for_url "SeaweedFS" "http://localhost:${ui}/cluster/status" \
    || fail "SeaweedFS is not healthy. Logs: $(compose_cli_hint) logs --tail 100 seaweedfs. Troubleshooting: $GUIDE#troubleshooting"
  ensure_hub_bucket
}

### Hermes: the agent CLI that Multica's Hermes runtime runs. Installed only when it is
### missing from PATH; configured from HERMES_* in the env file.

# Makes the installer's target directory visible to this run, so a Hermes installed
# by an earlier run (or by the installer above) is found without a new shell.
hermes_on_path() {
  if ! command -v hermes >/dev/null 2>&1 && [ -x "$HOME/.local/bin/hermes" ]; then
    export PATH="$HOME/.local/bin:$PATH"
  fi
  command -v hermes >/dev/null 2>&1
}

install_hermes() {
  if hermes_on_path; then
    ok "Hermes found: $(hermes --version 2>/dev/null | head -n 1)"
    return 0
  fi
  info "Installing Hermes ${HERMES_PIN_COMMIT:0:12} with the official installer. The first install can take several minutes."
  if ! curl -fsSL "$HERMES_INSTALL_URL" | bash -s -- --commit "$HERMES_PIN_COMMIT" --non-interactive; then
    echo "! The Hermes installer failed. Install steps: $GUIDE#hermes" >&2
    return 1
  fi
  hermes_on_path || {
    echo "! Hermes installed, but 'hermes' is not on PATH. Open a new shell (or add ~/.local/bin to PATH) and run make dev-full again." >&2
    return 1
  }
  # The first --version starts Hermes's Python runtime, which takes 30 to 40 seconds.
  # Running it here keeps the daemon's own 60-second probe from timing out later.
  ok "Hermes installed: $(hermes --version 2>/dev/null | head -n 1)"
}

# The env name Hermes gives a custom endpoint's key. Mirrors
# custom_endpoint_key_env() and its caller in hermes_cli (config.py, model_setup_flows_custom.py):
# HERMES_CUSTOM_ + the host and port, upper-cased, with every other character replaced by _.
hermes_key_env() {
  local rest hostport host port identity slug
  rest=${1#*://}
  hostport=${rest%%/*}
  hostport=${hostport##*@}
  host=${hostport%%:*}
  port=""
  case "$hostport" in
    *:*) port=${hostport##*:} ;;
  esac
  identity=$host
  if [ -n "$port" ]; then
    identity="${host}_${port}"
  fi
  slug=$(printf '%s' "$identity" | tr '[:lower:]' '[:upper:]' | sed -E 's/[^A-Z0-9]+/_/g; s/^_+//; s/_+$//')
  printf 'HERMES_CUSTOM_%s_API_KEY' "$slug"
}

# Sets KEY=VALUE in FILE: replaces the line when present, keeps every other line,
# and leaves the file mode 600. The value is never printed.
upsert_env_line() {
  local file=$1 key=$2 value=$3 tmp
  tmp=$(mktemp "${file}.XXXXXX")
  grep -v -E "^${key}=" "$file" > "$tmp" || true
  if [ -s "$tmp" ] && [ -n "$(tail -c 1 "$tmp")" ]; then
    printf '\n' >> "$tmp"
  fi
  printf '%s=%s\n' "$key" "$value" >> "$tmp"
  chmod 600 "$tmp"
  mv "$tmp" "$file"
}

# Writes the custom endpoint into Hermes's own config. An existing ~/.hermes/config.yaml
# and .env are copied to a timestamped .bak first. Changes go through `hermes config set`,
# so every other setting in config.yaml is kept.
configure_hermes() {
  local base model key key_env hermes_home stamp env_file cfg
  base=$(env_value "$ENV_FILE" HERMES_BASE_URL)
  model=$(env_value "$ENV_FILE" HERMES_MODEL)
  key=$(env_value "$ENV_FILE" HERMES_API_KEY)
  if [ -z "$base" ] || [ -z "$model" ] || [ -z "$key" ]; then
    local missing=() name
    for name in "${HERMES_KEYS[@]}"; do
      [ -n "$(env_value "$ENV_FILE" "$name")" ] || missing+=("$name")
    done
    echo "! Hermes is installed but not configured. Fill in these values in $ENV_FILE, then run make dev-full again:" >&2
    for name in "${missing[@]}"; do
      echo "    $name" >&2
      echo "        $(explain_knowledge_key "$name")" >&2
    done
    echo "  Hermes setup: $GUIDE#hermes" >&2
    return 1
  fi

  hermes_home="${HERMES_HOME:-$HOME/.hermes}"
  stamp=$(date +%Y%m%d%H%M%S)
  mkdir -p "$hermes_home"
  env_file="$hermes_home/.env"
  cfg="$hermes_home/config.yaml"
  key_env=$(hermes_key_env "$base")

  if [ -f "$env_file" ]; then
    cp -p "$env_file" "$env_file.bak-$stamp"
  else
    (umask 077 && : > "$env_file")
  fi
  upsert_env_line "$env_file" "$key_env" "$key"

  if [ -f "$cfg" ]; then
    cp -p "$cfg" "$cfg.bak-$stamp"
  fi
  # The key is referenced, not stored: model.api_key names the variable in .env. This
  # is the form Hermes's own custom-endpoint setup writes.
  hermes config set model.provider custom >/dev/null
  hermes config set model.base_url "$base" >/dev/null
  hermes config set model.default "$model" >/dev/null
  hermes config set model.api_key "\${$key_env}" >/dev/null
  ok "Hermes configured: custom endpoint $base, model $model, key in $env_file as $key_env"
}

# One-shot prompt through the configured endpoint. Prints the first line of the reply.
# A one-shot starts Hermes's background gateway, which can miss its startup deadline on a
# cold start, so a call without a reply is retried.
verify_hermes() {
  local attempt reply err
  err=$(mktemp)
  for attempt in 1 2 3; do
    if command -v timeout >/dev/null 2>&1; then
      reply=$(timeout 180 hermes -z "Reply with the word ok." 2>"$err" || true)
    else
      reply=$(hermes -z "Reply with the word ok." 2>"$err" || true)
    fi
    if [ -n "$reply" ]; then
      ok "hermes -z replied (attempt $attempt): $(printf '%s' "$reply" | head -n 1)"
      rm -f "$err"
      return 0
    fi
    echo "  hermes -z attempt $attempt gave no reply" >&2
  done
  echo "! hermes -z returned no reply after 3 attempts. Last stderr lines:" >&2
  tail -n 3 "$err" >&2
  rm -f "$err"
  return 1
}

setup_hermes() {
  if ! install_hermes; then
    HERMES_STATUS="not installed (see above)"
    return 0
  fi
  if ! configure_hermes; then
    HERMES_STATUS="installed, not configured (see above)"
    return 0
  fi
  if verify_hermes; then
    HERMES_STATUS="installed and replying; the daemon registers it when the daemon starts"
  else
    HERMES_STATUS="configured, but hermes -z did not reply (see above)"
  fi
}

### Daemon: started only when the default CLI profile has a login. The default profile
### is the one the web app's CLI and the daemon already use on this machine, so the
### daemon serves the same workspaces the web app shows.

MULTICA_PROFILE_CONFIG="$HOME/.multica/config.json"

multica_logged_in() {
  [ -f "$MULTICA_PROFILE_CONFIG" ] && grep -q -E '"token"[[:space:]]*:[[:space:]]*"[^"]' "$MULTICA_PROFILE_CONFIG"
}

# Writes a profile address only where the profile has none, so an address that already
# works (for example a LAN address) is kept. `config set` does not sign in;
# `setup self-host` is not used because it opens a browser for a login.
set_profile_value_if_absent() {
  local key="$1" value="$2"
  if grep -q -E "\"$key\"[[:space:]]*:[[:space:]]*\"[^\"]" "$MULTICA_PROFILE_CONFIG" 2>/dev/null; then
    return 0
  fi
  if ! make cli ARGS="config set $key $value" >/dev/null; then
    echo "! Could not set $key in the CLI profile. Run make cli ARGS=\"config set $key $value\" by hand." >&2
  fi
}

setup_multica_profile() {
  set_profile_value_if_absent server_url "http://localhost:${PORT}"
  set_profile_value_if_absent app_url "http://localhost:${FRONTEND_PORT}"
}

start_daemon() {
  if ! multica_logged_in; then
    DAEMON_STATUS="not started: no Multica login yet (see the next steps)"
    return 0
  fi
  info "Starting the daemon: make cli ARGS=\"daemon restart\""
  if make cli ARGS="daemon restart"; then
    DAEMON_STATUS="running (default profile); status: make cli ARGS=\"daemon status\""
  else
    DAEMON_STATUS="failed to start (see the output above)"
  fi
}

print_summary() {
  echo ""
  echo "Multica, WeKnora and the Hub store are up."
  echo ""
  echo "  Web app       http://localhost:${FRONTEND_PORT}"
  echo "  Multica API   http://localhost:${PORT}  (health: /health)"
  echo "  WeKnora API   ${KNOWLEDGE_URL}  (health: /health)"
  echo "  WeKnora UI    ${KNOWLEDGE_UI_URL}"
  echo "  Hub store     ${HUB_S3_ENDPOINT}  (S3 API, bucket ${HUB_S3_BUCKET})"
  echo "  SeaweedFS UI  http://localhost:${HUB_S3_UI_PORT}  (cluster status: /cluster/status)"
  echo ""
  echo "  Running       Multica API, Multica web, WeKnora (${KNOWLEDGE_SERVICES[*]}), SeaweedFS (${HUB_SERVICES[*]})"
  echo "  Hub store     $HUB_STORE_STATUS"
  echo "  Hermes        $HERMES_STATUS"
  echo "  Daemon        $DAEMON_STATUS"
  echo ""
  if ! multica_logged_in; then
    echo "Next steps. Full steps: $GUIDE#first-run-in-the-ui"
    echo "  1. In the web app, create the account and the first workspace."
    echo "  2. Sign the CLI in, then start the daemon (it registers Hermes):"
    echo "       make cli ARGS=\"login\""
    echo "       make cli ARGS=\"daemon restart\""
    echo "     Running make dev-full again also starts the daemon once the login exists."
    echo "  3. Knowledge: enable knowledge, then check Setup > Models."
    echo "  4. Office: Set up your company, then Build your company."
    echo ""
  fi
  echo "Ctrl-C stops Multica. The daemon, WeKnora and the Hub store keep running. To stop them:"
  echo "  make cli ARGS=\"daemon stop\""
  echo "  $(compose_cli_hint) stop ${KNOWLEDGE_SERVICES[*]} ${HUB_SERVICES[*]}"
  echo ""
}

start_multica() {
  info "Starting Multica: make dev (migrations, API, web)"
  make dev &
  DEV_PID=$!
  trap 'kill "$DEV_PID" 2>/dev/null || true' EXIT
  wait_for_url "Multica API" "http://localhost:${PORT}/health" "$DEV_PID" \
    || fail "Multica API did not answer. Read the make dev output above."
  wait_for_url "Multica web" "http://localhost:${FRONTEND_PORT}/" "$DEV_PID" \
    || fail "Multica web did not answer. Read the make dev output above."
  setup_multica_profile
  start_daemon
  print_summary
  # make dev runs until it is stopped; keep this script attached to it.
  wait "$DEV_PID"
}

main() {
  check_prerequisites
  resolve_env_file
  fill_knowledge_secrets
  fill_hub_secrets
  check_required_knowledge
  resolve_knowledge_urls
  resolve_hub_ports
  resolve_hub_urls
  load_env
  start_knowledge
  start_hub_store
  setup_hermes
  start_multica
}

# DEV_FULL_LIB=1 loads the functions without running the bootstrap (scripts/dev-full.test.sh).
if [ "${DEV_FULL_LIB:-0}" != 1 ]; then
  main "$@"
fi
