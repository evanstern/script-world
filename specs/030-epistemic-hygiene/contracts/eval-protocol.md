# Contract: Gist-Attribution Eval Protocol (TASK-73 precedent)

## What changes

The conversation OUTCOME prompt (the scene-summary call in internal/mind/convo.go) gains attribution rules:

- An unverified claim made in the scene stays attributed: "Rowan claimed he saw glowing tendrils" — never "the
  glowing tendrils" as communal fact.
- The gist never asserts an action was performed unless a participant performed it in the scene ("discussed
  searching" is fine; "after investigating" is not).
- Output SHAPE unchanged: gist, topics, per-participant tones, retold. Downstream parsing untouched.

## Eval gate (must complete BEFORE the prompt ships; FR-010, SC-004)

Artifacts under `specs/030-epistemic-hygiene/eval/`, runner in repo `scripts/eval-prompt-79.sh` (modeled on
`eval-prompt-73.sh`; same judge pattern):

1. **Fixtures** (`eval/fixtures/`): scripted scene transcripts with known ground truth — ≥3 speculation scenes
   (one speaker invents a claim; Thornspire-shaped), ≥3 action-discussed-not-done scenes, ≥4 control scenes
   (ordinary gists: chores, quarrels, trades) to catch quality regression.
2. **Variants**: `eval/old.md` (current prompt verbatim) and `eval/new.md` (attribution prompt).
3. **Runs**: each fixture × each variant × N≥3 samples on the standard local model; judge scores each gist:
   flattened? confabulated-action? faithful-and-useful (controls)?
4. **Ship bar**: flattening+confabulation rate reduced ≥50% vs old; control faithfulness within recorded
   tolerance (state it in decision.md before judging). Numbers + verdict in `eval/decision.md` AND recorded on
   TASK-79 (AC #3).
5. **Live sample**: after shipping, a live multi-scene sample inspected for the "after investigating" shape
   (SC-004/AC #3's live half); recorded in quickstart-results.md.

## Non-goals guard

The eval judges the GIST only. No grounding of claims against world state (perception-of-absence task); no rumor
mechanics changes; fixtures must not reward suppressing invention — a gist that keeps the myth WITH attribution
scores as success.
