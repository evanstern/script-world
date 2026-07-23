#!/usr/bin/env bash
#
# eval-prompt-73.sh — the ship-gate soak driver for TASK-73 (spec
# 027-villager-prompt-quality), research D1–D3.
#
# Builds the promptworld daemon from a named git ref into a temp dir (never
# disturbs the working tree), creates a fresh world with a fixed seed under an
# isolated PROMPTWORLD_HOME, points the local planner tier at the eval model,
# starts the daemon, runs until the world clock passes a fixed game-time window,
# stops it, and tallies villager-planner cog.tool_call verdicts + the acting-tool
# selection distribution from the durable event log. Writes one record per
# variant to specs/027-villager-prompt-quality/eval/<variant>.md (data-model §2).
#
# Villager planner jobs are isolated by joining cog.tool_call events to
# cog.thought events with class "planner" (same job id) — metatron/conversation
# jobs are excluded (research D1).
#
# Usage:
#   scripts/eval-prompt-73.sh <variant> <git-ref>
#     <variant>  eval label + record filename stem (old | new | new-exemplar)
#     <git-ref>  the ref to build the prompt from (e.g. origin/main, HEAD, a SHA)
#
# Env overrides (must be IDENTICAL across every variant compared):
#   SEED   world seed              (default 4242)
#   HOURS  game-time window hours  (default 6)
#   SPEED  daemon speed multiplier (default 16x; LLM worlds refuse "max")
#   MODEL  local Ollama model      (default cogito:3b)
#   ENDPOINT  local OpenAI-compat endpoint (default http://localhost:11434/v1)
#
# Prerequisites: go, git, jq; Ollama up with $MODEL pulled and warm.

set -euo pipefail

# ---- args ----------------------------------------------------------------
if [[ $# -ne 2 ]]; then
  echo "usage: $0 <variant> <git-ref>" >&2
  echo "  <variant>  old | new | new-exemplar" >&2
  echo "  <git-ref>  e.g. origin/main, HEAD, a SHA" >&2
  exit 2
fi
VARIANT="$1"
GITREF="$2"

SEED="${SEED:-4242}"
HOURS="${HOURS:-6}"
SPEED="${SPEED:-16x}"
MODEL="${MODEL:-cogito:3b}"
ENDPOINT="${ENDPOINT:-http://localhost:11434/v1}"
# cogito:3b's native OpenAI-compat function-calling is unreliable; the
# schema-constrained JSON envelope (tool_mode "json") is the documented mode for
# it (docs/wiki llm-providers). Pinned identically for every variant.
TOOL_MODE="${TOOL_MODE:-json}"

# TickGameSeconds == 1 (internal/clock), so the target tick is HOURS*3600.
TARGET_TICK=$(( HOURS * 3600 ))
# A generous real-time ceiling so a wedged daemon can't hang the run forever:
# game window / (0.5x of requested speed floor) + slack.
POLL_SECS=10
MAX_WALL_SECS=$(( TARGET_TICK / 4 + 1800 ))

# ---- repo + tool checks --------------------------------------------------
REPO_ROOT="$(git rev-parse --show-toplevel)"
EVAL_DIR="$REPO_ROOT/specs/027-villager-prompt-quality/eval"
mkdir -p "$EVAL_DIR"
RECORD="$EVAL_DIR/${VARIANT}.md"

for bin in go git jq; do
  command -v "$bin" >/dev/null 2>&1 || { echo "eval73: missing required tool: $bin" >&2; exit 1; }
done

RESOLVED_SHA="$(git rev-parse "$GITREF")"

echo "eval73: variant=$VARIANT ref=$GITREF ($RESOLVED_SHA) seed=$SEED hours=$HOURS speed=$SPEED model=$MODEL"

# ---- temp workspace ------------------------------------------------------
WORK="$(mktemp -d "${TMPDIR:-/tmp}/eval73-${VARIANT}.XXXXXX")"
BUILD_DIR="$WORK/src"
export PROMPTWORLD_HOME="$WORK/home"
mkdir -p "$BUILD_DIR" "$PROMPTWORLD_HOME"
BIN="$WORK/promptworld"
WORLD="eval73-${VARIANT}"
# Name-form worlds live at <PROMPTWORLD_HOME>/worlds/<name> (worlds.WorldsHome).
WORLD_DIR="$PROMPTWORLD_HOME/worlds/$WORLD"

cleanup() {
  # Best-effort: stop the daemon and remove the temp workspace.
  if [[ -n "${BIN:-}" && -x "$BIN" ]]; then
    "$BIN" stop "$WORLD" >/dev/null 2>&1 || true
  fi
  rm -rf "$WORK"
}
trap cleanup EXIT

# ---- build the binary from the ref (no working-tree disturbance) ---------
echo "eval73: exporting $RESOLVED_SHA -> $BUILD_DIR"
git archive "$RESOLVED_SHA" | tar -x -C "$BUILD_DIR"
echo "eval73: building promptworld"
( cd "$BUILD_DIR" && go build -o "$BIN" ./cmd/promptworld )

# ---- create + configure the world ---------------------------------------
echo "eval73: creating world $WORLD (seed $SEED)"
"$BIN" new "$WORLD" --seed "$SEED" >/dev/null

# Point the local planner tier at the eval model. The default config declares an
# uninstalled local model; every variant is pinned to the SAME model/endpoint so
# the only variable across soaks is the system prompt (research D3).
LLM_JSON="$WORLD_DIR/llm.json"
[[ -f "$LLM_JSON" ]] || { echo "eval73: no llm.json at $LLM_JSON" >&2; exit 1; }
tmp_json="$(mktemp)"
jq --arg model "$MODEL" --arg ep "$ENDPOINT" --arg tm "$TOOL_MODE" \
   '.providers.local.model = $model | .providers.local.endpoint = $ep | .providers.local.tool_mode = $tm' \
   "$LLM_JSON" > "$tmp_json"
mv "$tmp_json" "$LLM_JSON"
echo "eval73: local tier pinned -> $(jq -c '.providers.local' "$LLM_JSON")"

# ---- run the soak --------------------------------------------------------
# No cloud spend: strip any Anthropic key from the daemon's environment so the
# cloud-routed kinds (narrator/drama/metatron/consolidation) fail fast instead
# of billing. Planner is local-only and is all we measure.
echo "eval73: starting daemon"
env -u ANTHROPIC_API_KEY "$BIN" start "$WORLD" >/dev/null

# Ensure running and at the requested speed.
env -u ANTHROPIC_API_KEY "$BIN" resume "$WORLD" >/dev/null 2>&1 || true
"$BIN" speed "$WORLD" "$SPEED" >/dev/null

echo "eval73: soaking until world clock passes tick $TARGET_TICK (${HOURS} game-hours)"
start_wall=$(date +%s)
while :; do
  tick="$("$BIN" status "$WORLD" --json 2>/dev/null | jq -r '.clock.tick // 0')"
  now_wall=$(date +%s)
  elapsed=$(( now_wall - start_wall ))
  printf '\reval73: tick %-8s / %s  (%ds wall)   ' "$tick" "$TARGET_TICK" "$elapsed"
  if [[ "$tick" =~ ^[0-9]+$ ]] && (( tick >= TARGET_TICK )); then
    echo
    break
  fi
  if (( elapsed > MAX_WALL_SECS )); then
    echo
    echo "eval73: WARNING hit real-time ceiling ${MAX_WALL_SECS}s at tick $tick before the window closed" >&2
    break
  fi
  sleep "$POLL_SECS"
done

FINAL_TICK="$("$BIN" status "$WORLD" --json 2>/dev/null | jq -r '.clock.tick // 0')"
echo "eval73: stopping daemon (final tick $FINAL_TICK)"
"$BIN" stop "$WORLD" >/dev/null

# ---- tally from the durable log ------------------------------------------
TAIL="$WORK/tail.txt"
"$BIN" tail "$WORLD" --since 0 > "$TAIL"

# eventLine format: "#SEQ tTICK <day N HH:MM> TYPE PAYLOAD-JSON"; the game-time
# label is always 3 whitespace tokens, so the event type is field 6 and the JSON
# payload begins at the first '{'. Reproject to "TYPE<TAB>PAYLOAD".
TYPED="$WORK/typed.tsv"
awk 'index($0,"{")>0 { print $6 "\t" substr($0, index($0,"{")) }' "$TAIL" > "$TYPED"

# Diagnostic: event-type histogram (a soak that collected no planner cognitions
# is a setup failure, not a zero-rejection win).
echo "eval73: event-type counts (top 12):"
cut -f1 "$TYPED" | sort | uniq -c | sort -rn | head -12 | sed 's/^/  /'

# grep may legitimately match nothing (empty log); guard against pipefail. cut
# on tab keeps only the JSON payload.
PLANNER_JOBS="$WORK/planner_jobs.json"
{ grep -F 'cog.thought' "$TYPED" || true; } | cut -f2- \
  | jq -c 'select(.class=="planner") | .job' \
  | jq -s 'unique' > "$PLANNER_JOBS"

CALLS="$WORK/calls.json"
{ grep -F 'cog.tool_call' "$TYPED" || true; } | cut -f2- \
  | jq -s '.' > "$CALLS"

TALLY="$WORK/tally.json"
jq -n \
  --slurpfile pj "$PLANNER_JOBS" \
  --slurpfile calls "$CALLS" \
  '
  ($pj[0] // []) as $jobs
  | (reduce $jobs[] as $j ({}; .[$j] = true)) as $set
  | [ ($calls[0] // [])[] | select($set[.job] == true) ] as $p
  | ($p | length) as $denom
  | ($p | map(.verdict) | group_by(.) | map({key: .[0], value: length}) | from_entries) as $verdicts
  | ([ $p[] | select(.verdict=="landed") ] | map(.tool) | group_by(.) | map({key: .[0], value: length}) | from_entries) as $dist
  | {
      planner_tool_calls: $denom,
      rejected_malformed: ($verdicts.rejected_malformed // 0),
      rejected_cardinality: ($verdicts.rejected_cardinality // 0),
      landed: ($verdicts.landed // 0),
      verdicts: $verdicts,
      landed_distribution: $dist,
      landed_total: ([ $p[] | select(.verdict=="landed") ] | length)
    }
  ' > "$TALLY"

DENOM=$(jq -r '.planner_tool_calls' "$TALLY")
RM=$(jq -r '.rejected_malformed' "$TALLY")
RC=$(jq -r '.rejected_cardinality' "$TALLY")

pct() { # numerator denominator -> percent with 2 decimals (0.00 if denom 0)
  awk -v n="$1" -v d="$2" 'BEGIN{ if (d==0) printf "0.00"; else printf "%.2f", 100*n/d }'
}
RM_RATE=$(pct "$RM" "$DENOM")
RC_RATE=$(pct "$RC" "$DENOM")

# ---- write the record ----------------------------------------------------
{
  echo "# Eval record — variant \`$VARIANT\`"
  echo
  echo "Produced by \`scripts/eval-prompt-73.sh $VARIANT $GITREF\` (research D1–D3)."
  echo
  echo "| field | value |"
  echo "|-------|-------|"
  echo "| variant | \`$VARIANT\` |"
  echo "| git_ref | \`$GITREF\` |"
  echo "| git_sha | \`$RESOLVED_SHA\` |"
  echo "| seed | $SEED |"
  echo "| game_hours | $HOURS (target tick $TARGET_TICK; final tick $FINAL_TICK) |"
  echo "| provider/model | ollama \`$MODEL\` @ \`$ENDPOINT\` |"
  echo "| speed | $SPEED |"
  echo "| planner_tool_calls (denominator) | $DENOM |"
  echo "| rejected_malformed | $RM ($RM_RATE%) |"
  echo "| rejected_cardinality | $RC ($RC_RATE%) |"
  echo
  echo "## Verdict tally (villager planner jobs only)"
  echo
  echo "| verdict | count |"
  echo "|---------|-------|"
  jq -r '.verdicts | to_entries | sort_by(.key)[] | "| \(.key) | \(.value) |"' "$TALLY"
  echo
  echo "## Acting-tool selection distribution (landed calls)"
  echo
  landed_total=$(jq -r '.landed_total' "$TALLY")
  echo "Landed acting calls: $landed_total"
  echo
  echo "| tool | count | share |"
  echo "|------|-------|-------|"
  jq -r --argjson tot "$landed_total" '
    .landed_distribution | to_entries | sort_by(-.value)[]
    | "| \(.key) | \(.value) | " + (if $tot>0 then ((.value*10000/$tot|floor)/100|tostring) else "0" end) + "% |"
  ' "$TALLY"
  echo
  echo "## Token counts (research D4)"
  echo
  echo "_Filled by T011 from \`go test ./internal/mind/ -run TestPromptFrameReport -v\` at ref \`$GITREF\`._"
  echo
  echo "| metric | value |"
  echo "|--------|-------|"
  echo "| prompt_bytes | _pending_ |"
  echo "| prompt_words | _pending_ |"
  echo "| prompt_tokens_approx | _pending_ |"
  echo
  if (( DENOM < 200 )); then
    echo "> ⚠️ **Under the 200-decision floor** (research D3): $DENOM < 200. Extend the"
    echo "> game-time window equally for ALL variants (raise \`HOURS\`) and rerun."
  fi
} > "$RECORD"

echo "eval73: wrote $RECORD"
echo "eval73: denom=$DENOM rejected_malformed=$RM ($RM_RATE%) rejected_cardinality=$RC ($RC_RATE%)"
if (( DENOM < 200 )); then
  echo "eval73: WARNING only $DENOM villager acting decisions (< 200 floor); extend HOURS for ALL variants and rerun" >&2
fi
