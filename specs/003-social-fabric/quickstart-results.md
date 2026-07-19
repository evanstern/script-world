# Quickstart validation results — 2026-07-19

Live-verified against Ollama `gemma4:12b-mlx` (the operator's always-on local
model); automated suites green under `-race` (12 packages).

## Scenario A — edges and debts ✅ (automated + live)

Edge rules, ledger lifecycle (open→kept / open→broken), reputation math, and
the executor's give/due-check all unit-proven. Live runs showed talks moving
edges (+5/+5) with the deltas visible in soul.md Bonds sections.

## Scenario B — conversations and rumors ✅ (live)

First landed conversation (4 turns, Birch ↔ Cedar) — **authored persona
dynamics performed**: Birch ("finds Cedar's silences unbearable") opened with
"Say something already! Your silence is driving me crazy"; Cedar (quiet,
watchful) answered in evasions. Outcome gist landed as a 4★ memory in BOTH
souls; model-judged tones (−2/−2) moved the edges exactly per the rules
(Birch→Cedar trust −24, affection −45 net of the talk bump) — the village's
first grudge. Secrets seeded 8/8 at genesis. Rumor provenance/mutation
unit-proven (3-hop chain); live rumor spread awaits juicier material (no
deaths yet — by design, gossip needs grist).

## Scenario C — model-free world ✅ (automated)

Talks, gives, debts, verbatim rumor fallback all run with no llm.json;
conversation events absent; replay model-free (state-hash equality).

## Engineering findings from live runs (fixed in this branch)

1. **Mutual-idleness starved the fabric**: planner-era agents are always
   tasked; talks now happen while working (cooldown-bounded).
2. **Planner-trigger feedback loop** saturated the local tier; added a
   5-game-min per-agent debounce.
3. **Conversations starved behind planner traffic**: added a priority lane
   in the orchestrator (dialogue is interactive; thoughts tolerate
   staleness) + a worker-side 2-min hard cap so no hung transport can wedge
   a tier.
4. **Right-sized to local physics**: ~45s/utterance at 12B ⇒ 2 turns/side
   (within the "~5 cap") and a 6-min conversation deadline.
5. **Float tones**: models emit `"tone_a": -0.5`; parser accepts and rounds.

## Pace expectations (documented)

At 4x with gemma4:12b-mlx: a conversation completes in ~4 minutes wall; one
at a time. Faster local models (or cogito:3b perma-loaded) shorten everything.
