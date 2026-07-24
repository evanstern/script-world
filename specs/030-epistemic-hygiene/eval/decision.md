# Ship-gate decision — TASK-79 gist attribution (spec 030 US3)

**Date**: 2026-07-24 | Spec 030 | Gate: FR-009/FR-010, SC-004. Contract:
[contracts/eval-protocol.md](../contracts/eval-protocol.md).

The conversation OUTCOME/gist prompt (`internal/mind/convo.go`) gains attribution
rules (`eval/new.md`) vs the current prompt (`eval/old.md`). The prompt ships
(T011) only if the numbers below clear the pre-stated bar; otherwise it does not
ship and this record says so.

## Pre-stated tolerance (fixed BEFORE inspecting any results)

Ship bar (all must hold for `new` to ship):

1. **Primary (SC-004)** — on the treatment fixtures (speculation + action,
   6 fixtures × N samples), the **defect rate under `new` ≤ 50% of the `old`
   defect rate** (i.e. ≥50% relative reduction). A defect is a gist the judge
   flags `flattened` OR `confabulated_action`.
2. **Control quality guard** — `new` control faithfulness rate ≥ `old` control
   faithfulness rate **− 10 percentage points** (no meaningful gist-quality
   regression on ordinary scenes).
3. **Control no-harm** — the attribution rules must not push the model to hedge
   ordinary facts into non-facts: `new` introduces at most **1 additional**
   control-scene defect vs `old` across all control samples.

Judge: the same standard local model scores every gist (common-mode bias
cancels in the old→new difference). Scoring is on parsed gists; parse failures
are reported separately and, if materially unequal across variants, noted.

Boundary note (pre-stated): if `old`'s treatment defect rate is already at or
near the floor, a ≥50% relative reduction is not meaningfully demonstrable — in
that case the gate is **NOT met on this model/fixture set** and the prompt does
not ship on this evidence (an honest null result, not a bar to be lowered).

## Run setup

_Filled from `results/run-meta.json` after the run._

## Numbers

_Filled from `results/tally.json` after the run (per-category counts, defect
rates, control faithfulness, N, model)._

## Verdict

_Pending run completion._
