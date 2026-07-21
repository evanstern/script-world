#!/usr/bin/env bash
# probe-mlx-reasoning.sh — FR-008 / R6 investigation (TASK-42).
#
# Question: does the local MLX endpoint honor reasoning_effort under the
# utterance token cap (max_tokens=128), or does a thinking model burn the whole
# budget on hidden chain-of-thought and return empty text?
#
# Method: fire an utterance-shaped chat/completions request N times per config
# (reasoning_effort none | unset | low), measure content length and the
# empty-content rate. NOT shipped code — its durable output is a board note on
# TASK-42.
#
# Usage: sh specs/011-conversation-robustness/probe-mlx-reasoning.sh
# Env:   ENDPOINT (default http://localhost:11434/v1), MODEL (gemma4:12b-mlx), N (10)

set -u

ENDPOINT="${ENDPOINT:-http://localhost:11434/v1}"
MODEL="${MODEL:-gemma4:12b-mlx}"
N="${N:-10}"

# An utterance-shaped request: the same system/user shape convo.go sends, at
# the same 128-token cap.
SYSTEM='You are Rowan, a villager. Blunt, practical. You are talking with Hazel. Reply with ONLY {"say": "<one or two short sentences in your voice>"}'
USER='The conversation so far:
Hazel: Cold morning.

Your turn.'

# body <reasoning_json_or_empty> — assemble the request JSON. The reasoning
# fragment is spliced in verbatim (a trailing "..., " field or nothing).
body() {
  local reasoning="$1"
  cat <<JSON
{
  "model": "${MODEL}",
  "max_tokens": 128,
  ${reasoning}
  "messages": [
    {"role": "system", "content": $(printf '%s' "$SYSTEM" | json_str)},
    {"role": "user", "content": $(printf '%s' "$USER" | json_str)}
  ]
}
JSON
}

# json_str — minimal JSON string encoder for the prompt fields (escapes " \ and
# newlines) so the heredoc stays valid without a jq dependency.
json_str() {
  sed -e 's/\\/\\\\/g' -e 's/"/\\"/g' | awk 'BEGIN{printf "\""} {printf "%s%s", sep, $0; sep="\\n"} END{printf "\""}'
}

run_config() {
  local label="$1" reasoning="$2"
  local empties=0 total_len=0
  local lens=""
  echo "== config: ${label} (reasoning fragment: ${reasoning:-<none>}) =="
  for i in $(seq 1 "$N"); do
    resp=$(curl -sS "${ENDPOINT}/chat/completions" \
      -H 'Content-Type: application/json' \
      -d "$(body "$reasoning")")
    # Extract the assistant content length without jq: python is the portable
    # fallback that is always present on the dev host.
    len=$(printf '%s' "$resp" | python3 -c '
import sys, json
try:
    d = json.load(sys.stdin)
    c = d["choices"][0]["message"]["content"] or ""
except Exception:
    c = ""
print(len(c.strip()))
')
    lens="${lens} ${len}"
    total_len=$((total_len + len))
    [ "$len" -eq 0 ] && empties=$((empties + 1))
    printf '  run %2d: content_len=%s\n' "$i" "$len"
  done
  # median
  median=$(printf '%s\n' $lens | sort -n | awk '{a[NR]=$1} END{ if(NR%2){print a[(NR+1)/2]} else {print int((a[NR/2]+a[NR/2+1])/2)} }')
  printf '  -> empty-rate=%d/%d  mean_len=%d  median_len=%s\n\n' "$empties" "$N" "$((total_len / N))" "$median"
}

echo "endpoint=${ENDPOINT} model=${MODEL} N=${N} max_tokens=128"
echo

run_config "none"  '"reasoning_effort": "none",'
run_config "unset" ''
run_config "low"   '"reasoning_effort": "low",'
