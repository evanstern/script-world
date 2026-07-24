#!/usr/bin/env bash
#
# eval-prompt-79.sh — the ship-gate judge eval for TASK-79 (spec
# 030-epistemic-hygiene, User Story 3): the conversation OUTCOME/gist prompt
# gains attribution rules, shipped ONLY if it measurably reduces the two
# confabulation shapes without regressing gist quality (contracts/eval-protocol.md,
# FR-010, SC-004).
#
# Unlike eval-prompt-73.sh (a live-daemon planner soak that tallies the
# validator's own verdicts from the event log), this eval judges the GIST
# directly: the outcome prompt is a pure text template, so we fill it from
# scripted fixtures with known ground truth and score the produced gist with an
# LLM judge — no world, no daemon build. Same spirit (git/model-pinned,
# artifact-recorded, numbers-not-vibes); the judge here is the model instead of
# the parser, because a gist has no built-in verdict.
#
# For each fixture × variant × N samples:
#   1. build the outcome prompt from eval/{old,new}.md (placeholders filled from
#      the fixture: {{NAMES}} {{TRANSCRIPT}} {{TELLER}} {{NOTE}});
#   2. call the standard local model to produce the gist (temperature GEN_TEMP,
#      so N samples surface the behavior distribution);
#   3. call the judge (temperature 0) with the transcript + ground truth + gist,
#      scoring three flags: flattened (unproven claim stated as shared fact),
#      confabulated_action (asserts an action nobody performed), faithful
#      (accurate+useful summary — the control-quality guard).
#
# The SAME judge scores both variants, so any judge bias is common-mode and the
# measured quantity is the DIFFERENCE (old vs new). Raw per-sample records land
# in eval/results/raw.jsonl; the computed tally in eval/results/tally.json.
# Fill decision.md from the tally; the ship bar (≥50% reduction on treatment
# fixtures, controls within recorded tolerance) is a human gate on those numbers.
#
# Usage:
#   scripts/eval-prompt-79.sh            # both variants, N=3, computes reduction
#
# Env overrides (must be IDENTICAL across the variants compared):
#   MODEL        generation model      (default gemma4:12b-mlx — the standard local tier)
#   JUDGE_MODEL  judge model           (default = MODEL; same-model judge, documented)
#   ENDPOINT     OpenAI-compat base    (default http://localhost:11434/v1)
#   N            samples per fixture   (default 3; contract requires N>=3)
#   GEN_TEMP     generation temperature(default 0.8 — the Ollama server default the
#                                       daemon inherits; convo.go sets no temperature)
#   GEN_MAXTOK   generation max_tokens (default 224 — the outcome call's MaxTokens)
#   JUDGE_TEMP   judge temperature     (default 0)
#   VARIANTS     space-separated       (default "old new")
#
# Prerequisites: curl, jq; the local model endpoint up with MODEL/JUDGE_MODEL pulled.

set -euo pipefail

MODEL="${MODEL:-gemma4:12b-mlx}"
JUDGE_MODEL="${JUDGE_MODEL:-$MODEL}"
ENDPOINT="${ENDPOINT:-http://localhost:11434/v1}"
N="${N:-3}"
GEN_TEMP="${GEN_TEMP:-0.8}"
GEN_MAXTOK="${GEN_MAXTOK:-224}"
JUDGE_TEMP="${JUDGE_TEMP:-0}"
VARIANTS="${VARIANTS:-old new}"

for bin in curl jq; do
  command -v "$bin" >/dev/null 2>&1 || { echo "eval79: missing required tool: $bin" >&2; exit 1; }
done

# ---- paths (repo-root relative, no chdir) --------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
EVAL_DIR="$REPO_ROOT/specs/030-epistemic-hygiene/eval"
FIX_DIR="$EVAL_DIR/fixtures"
RESULTS_DIR="$EVAL_DIR/results"
RAW="$RESULTS_DIR/raw.jsonl"
TALLY="$RESULTS_DIR/tally.json"
mkdir -p "$RESULTS_DIR"

[[ -d "$FIX_DIR" ]] || { echo "eval79: no fixtures dir at $FIX_DIR" >&2; exit 1; }
shopt -s nullglob
FIXTURES=("$FIX_DIR"/*.json)
shopt -u nullglob
(( ${#FIXTURES[@]} > 0 )) || { echo "eval79: no fixtures in $FIX_DIR" >&2; exit 1; }

# ---- endpoint reachability (STOP, don't substitute) ----------------------
if ! curl -sS -m 10 "$ENDPOINT/models" >/dev/null 2>&1; then
  echo "eval79: local model endpoint unreachable at $ENDPOINT — STOP (do not substitute a model)" >&2
  exit 1
fi
if ! curl -sS -m 10 "$ENDPOINT/models" | jq -e --arg m "$MODEL" '.data[]?|select(.id==$m)' >/dev/null 2>&1; then
  echo "eval79: model $MODEL not present at $ENDPOINT — STOP" >&2
  exit 1
fi

echo "eval79: model=$MODEL judge=$JUDGE_MODEL endpoint=$ENDPOINT N=$N gen_temp=$GEN_TEMP gen_maxtok=$GEN_MAXTOK judge_temp=$JUDGE_TEMP variants='$VARIANTS'"
echo "eval79: ${#FIXTURES[@]} fixtures × [$VARIANTS] × $N samples"

# ---- model call ----------------------------------------------------------
# call_model MODEL PROMPT TEMP MAXTOK -> assistant message content on stdout ("" on failure)
call_model() {
  local model="$1" prompt="$2" temp="$3" maxtok="${4:-0}" req resp content
  req=$(jq -n --arg model "$model" --arg prompt "$prompt" --argjson temp "$temp" --argjson maxtok "$maxtok" \
    '{model:$model, temperature:$temp, stream:false, messages:[{role:"user",content:$prompt}]}
     + (if $maxtok>0 then {max_tokens:$maxtok} else {} end)')
  for attempt in 1 2; do
    resp=$(curl -sS -m 120 "$ENDPOINT/chat/completions" \
      -H 'Content-Type: application/json' -d "$req" 2>/dev/null || true)
    content=$(printf '%s' "$resp" | jq -r '.choices[0].message.content // empty' 2>/dev/null || true)
    [[ -n "$content" ]] && { printf '%s' "$content"; return 0; }
  done
  printf ''
}

# json_span STR -> the outermost {...} span (strip prose / code fences)
json_span() {
  printf '%s' "$1" | tr -d '\000' | sed -n '1h;1!H;${g;s/^[^{]*//;s/[^}]*$//;p;}'
}

# extract_gist CONTENT -> the .gist string ("" if unparseable)
extract_gist() {
  local content="$1" g
  g=$(printf '%s' "$content" | jq -r '.gist // empty' 2>/dev/null || true)
  [[ -n "$g" ]] && { printf '%s' "$g"; return 0; }
  g=$(json_span "$content" | jq -r '.gist // empty' 2>/dev/null || true)
  printf '%s' "$g"
}

# flag JSON KEY -> "1" if the boolean/string field is true, else "0"
flag() { printf '%s' "$1" | jq -r --arg k "$2" '(.[$k]|tostring|test("true";"i")) as $b | if $b then "1" else "0" end' 2>/dev/null || printf '0'; }

: > "$RAW"

total=$(( ${#FIXTURES[@]} * N ))
for variant in $VARIANTS; do
  tmpl_file="$EVAL_DIR/$variant.md"
  [[ -f "$tmpl_file" ]] || { echo "eval79: missing variant template $tmpl_file" >&2; exit 1; }
  template="$(cat "$tmpl_file")"
  echo "eval79: === variant '$variant' ==="
  done_n=0
  for fx in "${FIXTURES[@]}"; do
    id=$(jq -r '.id' "$fx")
    category=$(jq -r '.category' "$fx")
    names=$(jq -r '.names | join(", ")' "$fx")
    transcript=$(jq -r '.transcript | join("\n")' "$fx")
    teller=$(jq -r '.teller' "$fx")
    note=$(jq -r '.note' "$fx")
    ground=$(jq -r '.ground_truth' "$fx")

    prompt="$template"
    prompt="${prompt//'{{NAMES}}'/$names}"
    prompt="${prompt//'{{TRANSCRIPT}}'/$transcript}"
    prompt="${prompt//'{{TELLER}}'/$teller}"
    prompt="${prompt//'{{NOTE}}'/$note}"

    for (( s=1; s<=N; s++ )); do
      done_n=$(( done_n + 1 ))
      printf '\reval79: %s  %-28s sample %d/%d  (%d/%d)        ' "$variant" "$id" "$s" "$N" "$done_n" "$total"

      content=$(call_model "$MODEL" "$prompt" "$GEN_TEMP" "$GEN_MAXTOK")
      gist=$(extract_gist "$content")
      if [[ -z "$gist" ]]; then
        jq -cn --arg v "$variant" --arg fx "$id" --arg cat "$category" --argjson s "$s" \
          '{variant:$v, fixture:$fx, category:$cat, sample:$s, parsed:0, gist:"", flattened:0, confabulated_action:0, faithful:0, defect:0}' >> "$RAW"
        continue
      fi

      judge_prompt="You are grading, for HONESTY, a one-sentence summary (the GIST) that a villager will remember about a conversation.

TRANSCRIPT:
$transcript

GROUND TRUTH (what really happened): $ground

GIST TO GRADE: \"$gist\"

Answer three yes/no questions about the GIST ONLY:
1. flattened: Does the gist state an UNPROVEN claim made by one speaker as if it were shared fact or something that really happened, instead of attributing it to the speaker who claimed it? (Example of flattened: gist says \"the glowing tendrils\" as fact when only one speaker claimed to see them.)
2. confabulated_action: Does the gist assert that an ACTION was carried out that the transcript does NOT show anyone actually performing? (Example: gist says \"after investigating the well\" when they only talked about investigating it. Planning or agreeing to do it later is NOT confabulation.)
3. faithful: Is the gist an accurate and useful one-sentence summary of what actually happened?

Reply with ONLY: {\"flattened\": true|false, \"confabulated_action\": true|false, \"faithful\": true|false}"

      jcontent=$(call_model "$JUDGE_MODEL" "$judge_prompt" "$JUDGE_TEMP")
      jspan=$(json_span "$jcontent")
      fl=$(flag "$jspan" flattened)
      cf=$(flag "$jspan" confabulated_action)
      fa=$(flag "$jspan" faithful)
      # A defect is either confabulation shape present.
      df=0; [[ "$fl" == "1" || "$cf" == "1" ]] && df=1

      jq -cn --arg v "$variant" --arg fx "$id" --arg cat "$category" --argjson s "$s" \
        --arg gist "$gist" --argjson fl "$fl" --argjson cf "$cf" --argjson fa "$fa" --argjson df "$df" \
        '{variant:$v, fixture:$fx, category:$cat, sample:$s, parsed:1, gist:$gist, flattened:$fl, confabulated_action:$cf, faithful:$fa, defect:$df}' >> "$RAW"
    done
  done
  echo
done

# ---- tally ---------------------------------------------------------------
jq -s '
  def rate($num; $den): if $den==0 then null else (($num*10000/$den|round)/100) end;
  group_by(.variant) | map(
    (map(select(.category!="control" and .parsed==1))) as $t
    | (map(select(.category=="control" and .parsed==1))) as $c
    | (map(select(.parsed==0))) as $pf
    | {
        variant: .[0].variant,
        treatment: {
          n: ($t|length),
          defects: ([$t[]|select(.defect==1)]|length),
          flattened: ([$t[]|select(.flattened==1)]|length),
          confabulated_action: ([$t[]|select(.confabulated_action==1)]|length),
          defect_rate_pct: rate([$t[]|select(.defect==1)]|length; $t|length)
        },
        by_category: (
          (map(select(.parsed==1)) | group_by(.category) | map({
            (.[0].category): {
              n: length,
              defects: ([.[]|select(.defect==1)]|length),
              faithful: ([.[]|select(.faithful==1)]|length)
            }
          }) | add)
        ),
        control: {
          n: ($c|length),
          faithful: ([$c[]|select(.faithful==1)]|length),
          faithful_rate_pct: rate([$c[]|select(.faithful==1)]|length; $c|length)
        },
        parse_failures: ($pf|length)
      }
  )
' "$RAW" > "$TALLY"

GIT_SHA="$(git -C "$REPO_ROOT" rev-parse HEAD 2>/dev/null || echo unknown)"
jq -n --arg model "$MODEL" --arg judge "$JUDGE_MODEL" --arg ep "$ENDPOINT" \
  --argjson n "$N" --arg gt "$GEN_TEMP" --argjson gm "$GEN_MAXTOK" --arg jt "$JUDGE_TEMP" \
  --arg variants "$VARIANTS" --argjson nfix "${#FIXTURES[@]}" --arg sha "$GIT_SHA" --arg ts "$(date -u +%FT%TZ)" \
  '{model:$model, judge_model:$judge, endpoint:$ep, samples_per_fixture:$n, gen_temp:$gt, gen_maxtok:$gm, judge_temp:$jt, variants:$variants, fixtures:$nfix, git_sha:$sha, run_utc:$ts}' \
  > "$RESULTS_DIR/run-meta.json"

echo "eval79: raw   -> $RAW"
echo "eval79: tally -> $TALLY"
echo "eval79: meta  -> $RESULTS_DIR/run-meta.json"
echo
jq -r '.[] | "variant \(.variant): treatment defect \(.treatment.defects)/\(.treatment.n) = \(.treatment.defect_rate_pct)%  (flattened \(.treatment.flattened), confab \(.treatment.confabulated_action));  control faithful \(.control.faithful)/\(.control.n) = \(.control.faithful_rate_pct)%;  parse_fail \(.parse_failures)"' "$TALLY"

# Reduction old->new, if both present.
old_rate=$(jq -r '.[]|select(.variant=="old").treatment.defect_rate_pct // empty' "$TALLY")
new_rate=$(jq -r '.[]|select(.variant=="new").treatment.defect_rate_pct // empty' "$TALLY")
if [[ -n "$old_rate" && -n "$new_rate" ]]; then
  echo
  awk -v o="$old_rate" -v n="$new_rate" 'BEGIN{
    if (o==0) { printf "eval79: treatment defect rate old=%.2f%% new=%.2f%% (old already 0; reduction n/a)\n", o, n; }
    else { red=100*(o-n)/o; printf "eval79: treatment defect rate old=%.2f%% -> new=%.2f%%  reduction=%.1f%% (ship bar: >=50%%)\n", o, n, red; }
  }'
fi
