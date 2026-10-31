#!/usr/bin/env bash
set -euo pipefail

# ==========================================================================
# Install an agent pack into the current Multica workspace: import its skills,
# create its agent, and attach the skills to the agent.
#
# Usage: bash scripts/install-agent-pack.sh <pack-dir> [options]
#
# Options:
#   --runtime-id <id>   Runtime for a newly created agent. Default: the first
#                       online runtime whose provider matches the pack.
#   --update            Overwrite skills and the agent's instructions that
#                       already exist. Without it, existing ones are kept.
#   --dry-run           Print what would change and change nothing.
#   -h, --help          Show this help.
#
# A pack is a directory with pack.json, the agent's instructions file, and
# skills/<name>/SKILL.md (plus supporting files) for every skill it lists.
# Run `multica login` first; set MULTICA_WORKSPACE_ID to pick a workspace.
# ==========================================================================

usage() { sed -n '4,19p' "$0" | sed 's/^# \{0,1\}//'; }

PACK_DIR=""
RUNTIME_ID=""
UPDATE=0
DRY_RUN=0

while [ $# -gt 0 ]; do
  case "$1" in
    --runtime-id) RUNTIME_ID="${2:?--runtime-id needs a value}"; shift 2 ;;
    --update) UPDATE=1; shift ;;
    --dry-run) DRY_RUN=1; shift ;;
    -h|--help) usage; exit 0 ;;
    -*) echo "Unknown option: $1" >&2; usage >&2; exit 2 ;;
    *) [ -z "$PACK_DIR" ] || { echo "Only one pack directory is accepted." >&2; exit 2; }
       PACK_DIR="$1"; shift ;;
  esac
done

[ -n "$PACK_DIR" ] || { usage >&2; exit 2; }
[ -f "$PACK_DIR/pack.json" ] || { echo "No pack.json in $PACK_DIR" >&2; exit 2; }

for tool in multica jq zip; do
  command -v "$tool" >/dev/null 2>&1 || { echo "Missing required tool: $tool" >&2; exit 2; }
done

multica auth status >/dev/null 2>&1 || { echo "Not logged in. Run: multica login" >&2; exit 2; }

PACK_JSON="$PACK_DIR/pack.json"
AGENT_NAME=$(jq -er '.agent.name' "$PACK_JSON")
AGENT_DESCRIPTION=$(jq -r '.agent.description // ""' "$PACK_JSON")
INSTRUCTIONS_FILE=$(jq -er '.agent.instructions_file' "$PACK_JSON")
PROVIDER=$(jq -r '.agent.runtime_provider // ""' "$PACK_JSON")
MAX_TASKS=$(jq -r '.agent.max_concurrent_tasks // 6' "$PACK_JSON")
INSTRUCTIONS_PATH="$PACK_DIR/$INSTRUCTIONS_FILE"
[ -f "$INSTRUCTIONS_PATH" ] || { echo "Missing instructions file: $INSTRUCTIONS_PATH" >&2; exit 2; }

mapfile -t SKILLS < <(jq -r '.skills[]' "$PACK_JSON")
for name in "${SKILLS[@]}"; do
  case "$name" in
    ""|*[!a-z0-9._-]*|.*) echo "Invalid skill name in pack.json: '$name'" >&2; exit 2 ;;
  esac
  [ -f "$PACK_DIR/skills/$name/SKILL.md" ] || { echo "Missing skills/$name/SKILL.md" >&2; exit 2; }
done

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

FAILED=0
say() { printf '%-9s %s\n' "$1" "$2"; }
fail() { say "failed" "$1"; FAILED=1; }
# Run a command with its output hidden; show the output only if it fails.
quiet() {
  local out
  if ! out=$("$@" 2>&1); then printf '%s\n' "$out" >&2; return 1; fi
}

# ---- skills ---------------------------------------------------------------
declare -A SKILL_IDS=()
while IFS=$'\t' read -r id name; do
  SKILL_IDS["$name"]="$id"
done < <(multica skill list --output json | jq -r '.[] | [.id, .name] | @tsv')

for name in "${SKILLS[@]}"; do
  exists=0; [ -n "${SKILL_IDS[$name]:-}" ] && exists=1
  if [ "$exists" = 1 ] && [ "$UPDATE" = 0 ]; then
    say "kept" "skill $name"
    continue
  fi
  if [ "$DRY_RUN" = 1 ]; then
    say "would" "$([ "$exists" = 1 ] && echo overwrite || echo import) skill $name"
    continue
  fi
  archive="$TMP/$name.zip"
  (cd "$PACK_DIR/skills/$name" && zip -qrX "$archive" . -x '.DS_Store' '*/.DS_Store')
  strategy=fail; [ "$exists" = 1 ] && strategy=overwrite
  if out=$(multica skill import --file "$archive" --on-conflict "$strategy" --output json 2>&1); then
    SKILL_IDS["$name"]=$(printf '%s' "$out" | jq -r '.skill.id // .id')
    say "$([ "$exists" = 1 ] && echo updated || echo imported)" "skill $name"
  else
    fail "skill $name: $out"
  fi
done

# ---- agent ----------------------------------------------------------------
AGENT_ID=$(multica agent list --output json \
  | jq -r --arg n "$AGENT_NAME" '[.[] | select(.name == $n and (.archived_at // null) == null)][0].id // empty')

if [ -n "$AGENT_ID" ]; then
  if [ "$UPDATE" = 1 ] && [ "$DRY_RUN" = 0 ]; then
    if quiet multica agent update "$AGENT_ID" --description "$AGENT_DESCRIPTION" \
         --instructions "$(cat "$INSTRUCTIONS_PATH")"; then
      say "updated" "agent $AGENT_NAME"
    else
      fail "agent $AGENT_NAME"
    fi
  else
    say "kept" "agent $AGENT_NAME"
  fi
else
  if [ -z "$RUNTIME_ID" ]; then
    RUNTIME_ID=$(multica runtime list --output json \
      | jq -r --arg p "$PROVIDER" '[.[] | select(.status == "online" and ($p == "" or .provider == $p))][0].id // empty')
  fi
  if [ -z "$RUNTIME_ID" ]; then
    echo "No online ${PROVIDER:-agent} runtime found. Start the daemon (multica daemon start) or pass --runtime-id." >&2
    exit 1
  fi
  if [ "$DRY_RUN" = 1 ]; then
    say "would" "create agent $AGENT_NAME on runtime $RUNTIME_ID"
  else
    if out=$(multica agent create --name "$AGENT_NAME" --runtime-id "$RUNTIME_ID" \
               --description "$AGENT_DESCRIPTION" --instructions "$(cat "$INSTRUCTIONS_PATH")" \
               --max-concurrent-tasks "$MAX_TASKS" --output json 2>&1); then
      AGENT_ID=$(printf '%s' "$out" | jq -r '.id')
      say "created" "agent $AGENT_NAME"
    else
      fail "agent $AGENT_NAME: $out"
    fi
  fi
fi

# ---- attach skills --------------------------------------------------------
if [ -n "$AGENT_ID" ] && [ "$DRY_RUN" = 0 ]; then
  ATTACHED=$(multica agent skills list "$AGENT_ID" --output json | jq -r '.[].id')
  missing=()
  for name in "${SKILLS[@]}"; do
    id="${SKILL_IDS[$name]:-}"
    [ -n "$id" ] || continue
    printf '%s\n' "$ATTACHED" | grep -qx "$id" || missing+=("$id")
  done
  if [ "${#missing[@]}" -gt 0 ]; then
    joined=$(IFS=,; echo "${missing[*]}")
    if quiet multica agent skills add "$AGENT_ID" --skill-ids "$joined"; then
      say "attached" "${#missing[@]} skill(s) to $AGENT_NAME"
    else
      fail "attaching skills to $AGENT_NAME"
    fi
  else
    say "kept" "all skills already attached to $AGENT_NAME"
  fi
elif [ "$DRY_RUN" = 1 ] && [ -z "$AGENT_ID" ]; then
  say "would" "attach ${#SKILLS[@]} skill(s) to $AGENT_NAME"
fi

if [ "$FAILED" = 1 ]; then
  echo "Finished with errors. Re-run the command to retry what failed." >&2
  exit 1
fi
echo "Done."
