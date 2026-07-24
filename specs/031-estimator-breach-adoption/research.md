# Research: Estimator Breach Adoption

All Technical Context entries were resolvable from the codebase and TASK-86's
recorded analysis; no external research required. Decisions below resolve every
design fork the plan depends on.

## R1 — Adoption value: window median

- **Decision**: on breach, adopt the **median** of the retained WindowSize sample
  values (spike and non-spike alike).
- **Rationale**: deterministic, no new tuning constant, and robust to mixed windows.
  Breach requires spike rate > 0.3, i.e. ≥ 7 of 20 samples beyond 3× the estimate.
  Under full saturation (world-01: spike rate 1.0) the median IS the new regime.
  Under a bimodal window (~7 slow / 13 normal) the median sits in the normal
  population and adoption barely moves the estimate — correct, since the majority
  regime is still the estimate's regime; the operator signal still fires.
- **Alternatives considered**:
  - *Mean* — rejected: dominated by extreme spikes (a single 60 s/pt outlier drags
    the whole window).
  - *Max / p90* — rejected: overshoots into permanent over-suppression; fails toward
    reflex too hard on one bad window.
  - *Clamped EWMA feed* (option B on TASK-86) — rejected as primary: moves the
    estimate +24% per clamped sample at alpha 0.2, trading away one-shot rejection;
    it's a tuning compromise, not a classification. Kept on the board task as
    fallback doctrine if median adoption proves unstable in practice.
  - *Rolling-median estimator* (option C) — rejected: discards the EWMA doctrine and
    changes steady-state convergence character for a problem that only exists at
    breach.

## R2 — Event shape: additive fields on cog.recalibration_recommended

- **Decision**: no new event type. `sim.RecalibrationPayload` gains two additive,
  `omitempty` fields: `prior_s_per_pt` and `adopted_s_per_pt`. When adoption occurs
  (which is exactly when the breach event fires), both are set; the existing
  `estimate_s_per_pt` field carries the post-adoption estimate, preserving its
  meaning of "the estimator's current estimate at emission".
- **Rationale**: breach and adoption are the same episode — one sample, one signal,
  one record. A second event type would double-count breach episodes for any
  consumer summing them, need a new reducer-whitelist entry (internal/sim/loop.go)
  and a new digest catalog entry (internal/tui/digest.go, guarded by
  TestCatalogSweep). Additive omitempty fields keep every historical event
  replay-identical and every existing consumer working; the digest renderer is
  extended to show `prior→adopted` when present.
- **Alternatives considered**: new `cog.estimator_adopted` type — rejected per above;
  also violates "smallest doctrine surface" since spec 007 already names the breach
  signal as THE recalibration record.

## R3 — API seam: Sample() adopts internally, returns the evidence

- **Decision**: `Estimator.Sample` keeps owning the breach decision and now performs
  the adoption itself under the same mutex hold (no check-then-act race). Its return
  changes from `(recalibrate bool)` to a nilable evidence value (e.g.
  `*Adoption{Prior, Adopted, SpikeRate float64}`) — nil means no breach. The
  orchestrator's `feedEstimate` forwards the evidence through the recalibrate hook,
  whose signature gains the prior/adopted values; `Mind.RecalibrateSignal` marshals
  them into the payload.
- **Rationale**: adoption must be atomic with breach detection (concurrent
  completions share one estimator per provider — edge case in spec). Returning the
  evidence keeps the package leaf (no event types, no imports) and lets the existing
  hook path carry everything to the event log. The window reset inside the same lock
  gives re-arm semantics for free: `breached` state and ring both restart, so US3
  scenario 2 (no repeat records while stable at the new level) holds structurally.
- **Alternatives considered**: separate `AdoptIfBreached()` method for the caller to
  invoke after Sample — rejected: check-then-act across two lock acquisitions; two
  callers (worker path and ObserveCognition path) would both need the dance.

## R4 — What resets at adoption

- **Decision**: adoption zeroes the ring (`wn=0, wi=0`), sets `estimate` to the
  median, and clears `breached`. Lifetime `samples`/`spikes` counters keep counting
  (they are telemetry, not decision state).
- **Rationale**: a fresh window must fill (20 samples) before any further breach —
  matching the existing "no breach verdicts until the window is full" doctrine and
  preventing adoption flapping. Clearing `breached` rather than leaving it armed is
  correct because the rate is definitionally 0 in an empty window; the existing
  re-arm behavior (`breached=false` when rate falls below threshold) converges to
  the same state.

## R5 — Downward drift needs no adoption path

- **Decision**: no change for the system-got-faster case.
- **Rationale**: samples below the estimate are never spikes
  (`secPerPoint > SpikeFactor*estimate` is the only spike test), so the EWMA follows
  improvements today already; the freeze is asymmetric by construction. Spec US1
  scenario 3 is covered by existing behavior plus a regression test.

## R6 — Test surface

- **Decision**: three new test families in `estimate_test.go`:
  (1) world-01 freeze regression — seed 0.52, sustained 12 s/pt, assert adoption at
  the window-completing sample and estimate == median;
  (2) one-shot preservation — 1–2 spikes per window leave the estimate bit-identical
  to the pre-change arithmetic (SC-002);
  (3) re-arm — post-adoption stability at the new level emits no second adoption;
  a second genuine step does.
  Existing tests asserting the freeze (if any assert estimate-never-moves under
  sustained spikes) are consciously retuned; TestEstimatorSampleCountUnderConcurrency
  must stay green (mutex path unchanged in shape).
- **Rationale**: SC-001/002/005 map one-to-one; TestCatalogSweep guards the digest
  change without new machinery.
