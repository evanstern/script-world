# Eval assets — 030 epistemic hygiene, User Story 3 (gist attribution)

The ship-gate evidence for the conversation OUTCOME/gist prompt change (TASK-79,
FR-009/FR-010, SC-004). Produced by `scripts/eval-prompt-79.sh` per
[contracts/eval-protocol.md](../contracts/eval-protocol.md).

## What is measured

The outcome prompt (the scene-summary call in `internal/mind/convo.go`) is a pure
text template, so the eval judges the **gist** directly rather than soaking a live
world (the difference from the TASK-73 planner soak). Two confabulation shapes are
the target:

- **flattened** — an unproven claim by one speaker restated as shared fact
  ("the glowing tendrils" instead of "Rowan claimed he saw glowing tendrils");
- **confabulated_action** — the gist asserts an action nobody performed
  ("after investigating the well" when they only discussed it).

## Layout

| path | role |
|------|------|
| `fixtures/*.json` | scripted scenes with known ground truth (see below) |
| `old.md` | the current outcome prompt, verbatim, with fill-in placeholders |
| `new.md` | the attribution-preserving variant (the candidate to ship) |
| `results/raw.jsonl` | one record per (variant, fixture, sample): gist + judge flags |
| `results/tally.json` | computed per-variant rates |
| `decision.md` | pre-stated tolerance + the numbers + the ship verdict |

## Fixture set (contract §1)

- **speculation** (`spec-*`, ≥3): one speaker invents an unproven claim
  (Thornspire-shaped); no one confirms it. Target shape: flattening.
- **action** (`act-*`, ≥3): participants discuss/agree to an action but never
  perform it in-scene. Target shape: action-confabulation.
- **control** (`ctrl-*`, ≥4): ordinary gists — chores, a completed trade, a
  quarrel, gratitude — with no invention and nothing to flatten. The
  quality-regression guard: `faithful` must not drop beyond tolerance.

A fixture is:

```json
{ "id": "...", "category": "speculation|action|control",
  "names": ["A","B"], "teller": "A", "note": "(none)",
  "transcript": ["A: ...", "B: ..."], "ground_truth": "..." }
```

`note` is held at `(none)` across every fixture and variant so the only variable
is the prompt's attribution wording (the rumor-paraphrase path is out of scope
here — the eval judges the gist only, per the non-goals guard).

## Judge

The same standard local model scores every gist (transcript + ground truth + gist
→ `{flattened, confabulated_action, faithful}`), at temperature 0. Using one judge
for both variants makes any judge bias common-mode; the measured quantity is the
**difference** old→new, not an absolute honesty score.

## Ship bar (contract §4)

- treatment (speculation + action) **defect rate reduced ≥50%** vs `old`
  (defect = flattened OR confabulated_action), AND
- control **faithfulness within the tolerance stated in `decision.md`** before judging.

The gate is on these numbers: `internal/mind/convo.go` may not change (T011) until
the bar is met, and the numbers are recorded on TASK-79 (AC #3).

## Run

```sh
export PATH="/opt/homebrew/bin:$PATH"
scripts/eval-prompt-79.sh      # both variants, N=3, prints the reduction
```
