# Research: Governor Accrued-Drift Debt

All unknowns resolved from the codebase and TASK-87's recorded analysis.

## R1 — The overdue arm: max(PredictedSec, ElapsedSec)

- **Decision**: per-thought remaining-work seconds become
  `max(PredictedSec, ElapsedSec)` (was `max(0, PredictedSec − ElapsedSec)`).
- **Rationale**: the accrued drift (elapsed × tps) is a MEASUREMENT — the minimum
  staleness the reply will land with — so counting it grounds the debt in fact, not
  invention; the doctrine sentence flips from "an overdue thought invents no debt it
  cannot ground" to "an overdue thought's elapsed time IS its grounded debt". Within
  prediction the two formulas agree… **no — they do not**, and this is the key
  subtlety: the OLD formula counts *remaining* work (predicted − elapsed) so a
  healthy thought's contribution DRAINS as it progresses; `max(predicted, elapsed)`
  counts *total landing drift* and would inflate debt for healthy in-flight
  thoughts, changing shed behavior in the non-defective case and violating SC-003.
  The correct arm preserving both properties is:

  `remaining = max(PredictedSec − ElapsedSec, 0)` for elapsed ≤ predicted (today's
  arithmetic, drains to zero) **plus** the overdue arm: when elapsed > predicted,
  the thought contributes `ElapsedSec − PredictedSec`… which grows from zero — but
  that discards the accrued-minimum insight and undercounts a thought stuck since
  before its prediction elapsed.

  Resolution: the spec's acceptance scenarios are the arbiter. US1-AC1 requires a
  2 s-predicted / 30 s-elapsed thought to contribute "30 s-worth of drift"; US1-AC4
  and SC-003 require bit-identical output only "where every thought lands within
  its prediction" — i.e. sets with elapsed ≤ predicted. For such sets,
  `max(PredictedSec, ElapsedSec)` yields `PredictedSec`, NOT the draining
  `PredictedSec − ElapsedSec`, so plain max over TOTAL drift breaks SC-003.
  Therefore the implemented arm is piecewise, both halves grounded:

  ```
  remaining(job) = PredictedSec − ElapsedSec   if ElapsedSec < PredictedSec  (drains, as today)
                 = ElapsedSec                   if ElapsedSec ≥ PredictedSec  (accrued drift, grows)
  ```

  A healthy thought drains exactly as today (SC-003 holds bit-identically); the
  moment it overruns, its contribution jumps to its full accrued drift and grows
  monotonically (US1-AC1: 30 s counts as 30 s; US1-AC2: 45 s > 30 s; SC-002 holds).
  The jump at the boundary (from ~0 remaining to full elapsed) is deliberate: an
  overrun is evidence the prediction was wrong, so the reply's landing staleness is
  unknown-but-at-least-accrued — the honest floor switches from "almost done" to
  "already this stale".
- **Alternatives considered**:
  - *Plain `max(Predicted, Elapsed)` everywhere* (TASK-87's shorthand) — rejected:
    silently changes healthy-set debt from remaining-work to total-drift semantics,
    inflating debt at high concurrency and breaking SC-003/US1-AC4; the shorthand's
    intent (overdue jobs count their accrued drift) is preserved by the piecewise
    form.
  - *Overrun-only growth `max(0, Elapsed − Predicted)`* — rejected: a thought
    30 s into a 2 s prediction would count only 28 s and, worse, a thought stuck at
    exactly its prediction counts zero; undercounts the measured minimum.
  - *Capped contribution (e.g. at 1.0 budget-fraction)* — rejected: a
    dead-on-arrival thought legitimately holds debt over threshold (spec edge
    case); capping re-introduces blindness for fleets of stuck thoughts.

## R2 — Tick-rate approximation for accrued drift

- **Decision**: elapsed seconds convert to ticks at the CURRENT sampled
  `ticksPerSecond`, same as the predicted arm.
- **Rationale**: per-thought speed history does not exist in the pending snapshot
  and adding it would violate FR-003 (no new inputs). The approximation errs toward
  protection only when speed was recently RAISED (elapsed seconds at the old lower
  speed count as if at the new higher speed) — precisely when protection matters;
  after a shed it errs toward faster recovery-blocking, which the asymmetric
  hysteresis already dampens.

## R3 — The jobs counter

- **Decision**: unchanged rule — a thought counts iff it contributes a positive
  fraction. Overdue thoughts now contribute, so they count.
- **Rationale**: the visible jobs figure had the same blindness (world-01 status
  showed 0 jobs while thoughts drowned); the fix falls out of the arithmetic change
  with no separate logic.

## R4 — What is NOT changing

- **Decision**: `Governor.Sample` (hysteresis state machine), all doctrine
  constants, `Decision`/event shapes, the daemon sampler, and the pending registry
  are untouched. Doctrine text updates land in spec 028 (debt definition,
  the "invents no debt" rationale) and `governor.go`'s doc comments.
- **Rationale**: the controller works; it was fed a lie. Verified: the sampler
  (`internal/daemon/governor.go` sample()) passes PredictedSec/ElapsedSec through
  verbatim, and `internal/llm/pending.go` computes both at READ time from live
  state — no staleness in the inputs themselves.

## R5 — Verification strategy (US2)

- **Decision**: three layers. (1) Red-first unit regression in
  `internal/cognition/governor_test.go` encoding the world-01 shape (thoughts
  predicted 1.573 s, elapsed 20–50 s, 8 t/s, planner budget 1200 → debt >> 1.0,
  shed at the 5th consecutive sample). (2) Sampler-level scenario in
  `internal/daemon/governor_test.go` (existing harness: pending set + status stub →
  shed decision → Govern call within breachSamples). (3) Operational: rebuild the
  daemon binary, restart world-01, confirm the governor is present and — under
  deliberate saturation — a `clock.governor_shed` event lands; result recorded on
  TASK-87 (the binary built 19:23 Jul 23 predates the 21:53 task-33 merge, so this
  check is mandatory regardless).
- **Rationale**: world-01's zero-shed evidence has two candidate causes (debt floor
  + possibly stale binary); FR-006 demands positive live evidence, not just green
  units.

## R6 — Relationship to spec 031 (estimator breach-adoption)

- **Decision**: no code dependency; PRs are independent (disjoint files). The
  world-01 regression test deliberately uses frozen optimistic predictions to prove
  the governor now protects even when the estimator is wrong.
- **Rationale**: defense in depth — 031 fixes the predictions, 033 makes the
  throttle robust to bad predictions; either alone would have caught world-01, both
  together close the episode.
