# Feature Specification: Governor Accrued-Drift Debt — overdue thoughts count what they've already cost

**Feature Branch**: `033-governor-accrued-debt`

**Created**: 2026-07-24

**Status**: Draft

**Input**: User description: "Governor accrued-drift debt (TASK-87): the adaptive-throttle governor must see overdue in-flight thoughts instead of flooring them to zero debt. Today Debt sums max(0, PredictedSec − ElapsedSec) × ticksPerSecond / BudgetTicks per pending thought, so a job whose elapsed time exceeds its prediction contributes ZERO — the moments of worst drift produce the least debt, and the throttle never sheds exactly when the system is drowning. Chosen fix (option A on TASK-87): per-job fraction becomes max(PredictedSec, ElapsedSec) × ticksPerSecond / BudgetTicks. Verification story: saturation red test + verify the deployed daemon binary contains the governor and clock.governor_shed fires in a deliberately saturated run."

## The defect this fixes (world-01, 2026-07-23)

The adaptive throttle (spec 028) computes staleness debt over pending model-bound
thoughts as *predicted remaining work*, floored at zero: a thought whose elapsed
time exceeds its prediction contributes nothing. The doctrine sentence — "an overdue
thought invents no debt it cannot ground" — treats the overrun as unknowable. But
the overrun is a **measurement**, not an invention: the drift a thought has already
accrued while in flight is real staleness its reply will land with *at minimum*.

The inversion this produces is exactly backwards: the worse the system is drowning,
the sooner every in-flight thought goes "overdue" and vanishes from the debt sum, so
the moments of maximum drift register minimum debt and the governor never sheds.
World-01 evidence: at 8x–32x with optimistic predictions (the spec-031 estimator
freeze), 17 of 31 planner thoughts landed rejected-stale while the event log shows
**zero** governor shed events — the throttle built to protect the run was blind
through the entire episode. The defect stands alone even with a healthy estimator:
any single stuck call (endpoint hang, model swap-in) disappears from debt the moment
its elapsed time passes its prediction.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Overdue thoughts contribute their true, growing drift (Priority: P1)

An operator runs a world faster than the serving models can keep up. Each in-flight
thought's debt contribution is the larger of what was predicted and what has already
elapsed — so a thought that blows past its prediction keeps counting, and its
contribution grows as it languishes. Aggregate debt now rises exactly when the
system is drowning, and the governor sheds speed within its normal breach window
instead of sitting blind at the ceiling.

**Why this priority**: this is the defect itself — the debt signal inverts under
overload. Everything spec 028 built (shed, recover, status surface) already works;
it is fed a lie.

**Independent Test**: unit-level — a pending set with thoughts whose elapsed time
exceeds their predictions yields debt that is positive, grounded in elapsed time,
and non-decreasing as elapsed grows; a saturation scenario drives debt past the shed
threshold and the governor sheds within one breach window (written as a red test
against the current arithmetic first).

**Acceptance Scenarios**:

1. **Given** a thought predicted at 2 s that has been in flight 30 s, **When** debt
   is computed at any positive tick rate, **Then** the thought contributes
   30 s-worth of drift against its class budget (not zero).
2. **Given** the same thought sampled again later at 45 s elapsed, **When** debt is
   computed, **Then** its contribution is strictly larger than at 30 s — a stuck
   thought's debt grows, never evaporates.
3. **Given** the world-01 saturation shape (sustained multi-thought planner load,
   ~20–50 s true cost, optimistic ~1.6 s predictions, 8 ticks/sec), **When** the
   governor samples at its normal cadence, **Then** debt exceeds the shed threshold
   and a shed decision fires within one breach window — the scenario that produced
   zero sheds in world-01 now sheds.
4. **Given** a healthy system where every thought lands within its prediction,
   **When** debt is computed, **Then** every contribution equals today's arithmetic
   exactly — no behavior change inside prediction.

---

### User Story 2 - The fix is provably live in a real run (Priority: P2)

The operator can confirm the governor actually protects a running world: the
deployed daemon contains the governor at all (world-01's running binary predates
the throttle's merge), and a deliberately saturated live or end-to-end run shows
shed events landing in the event log and the effective speed stepping down the
ladder, visible in status.

**Why this priority**: world-01's zero-shed evidence has two candidate causes —
the debt floor (US1) and possibly a stale deployed binary. Closing the defect
requires positive evidence of a shed in a real saturated run, not just green unit
tests.

**Independent Test**: rebuild and restart the world-01 daemon from the merged
branch; drive saturation (high speed + slow provider); observe at least one
governor shed event in the event log and the governed state in status.

**Acceptance Scenarios**:

1. **Given** a freshly built daemon from this feature's merge, **When** the binary
   is checked against the source it was built from, **Then** the governor machinery
   is present and sampling (recorded on the board task).
2. **Given** a deliberately saturated run (sustained slow thoughts at high speed),
   **When** the breach window completes, **Then** a governor shed event lands in
   the event log with its debt arithmetic, and the status surface shows the
   governed (effective < requested) state.

---

### Edge Cases

- **Thought crossing its prediction** (elapsed reaching predicted): its
  contribution deliberately JUMPS from the drained remaining-work (~0) to its full
  accrued drift — the overrun is the moment the prediction is proven wrong, so the
  honest floor switches from "almost done" to "already this stale". This
  discontinuity is doctrine, not an artifact.
- **Queued thoughts** (elapsed 0): contribute predicted drift, exactly as today.
- **Dead-on-arrival thought** (accrued drift alone already exceeds its class
  budget): contributes a fraction above 1.0 by itself — a single such thought can
  legitimately hold the debt over the shed threshold.
- **Speed changes mid-flight**: elapsed seconds are converted to drift at the
  CURRENT sampled tick rate, same approximation the predicted arm already uses —
  no per-thought speed history is introduced.
- **Paused world**: governor sampling already resets on pause (spec 028 FR-013);
  elapsed wall time during a pause does not advance game ticks, and the existing
  pause semantics are unchanged by this feature.
- **Unknown kinds / non-positive tick rate**: skipped / zero debt, exactly as
  today (spec 028 FR-002).
- **Jobs counter**: a thought counts toward the governor's jobs figure iff it
  contributes a positive fraction — overdue thoughts now count (they contribute),
  which corrects the same blindness in the visible jobs number.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: A pending thought that has overrun its prediction (elapsed ≥
  predicted) MUST contribute its full already-elapsed work — expressed as drift at
  the sampled tick rate against its class's staleness budget — so an overdue
  thought counts its accrued, growing drift rather than zero. (An overrun is
  evidence the prediction was wrong; the accrued drift is the measured minimum
  staleness its reply will land with.)
- **FR-002**: For thoughts within their prediction (elapsed ≤ predicted), the
  contribution MUST be unchanged from the current doctrine — the fix alters only
  the overdue arm.
- **FR-003**: Debt MUST remain pure, deterministic arithmetic over the sampled
  inputs: no wall-clock reads, no randomness, no new input sources (the same
  pending-set snapshot the governor already receives).
- **FR-004**: The spec 028 doctrine (debt definition, FR-001/FR-002 there, and the
  "overdue thought invents no debt" rationale) MUST be updated to the accrued-drift
  definition, as a reviewed doctrine change; governor hysteresis constants (shed
  threshold, breach/recovery windows, cadence) are NOT changed by this feature.
- **FR-005**: The world-01 zero-shed saturation shape MUST exist as a regression
  test written red-first against the old arithmetic: optimistic predictions plus
  long-elapsed in-flight thoughts at high speed drive debt over the shed threshold
  and produce a shed decision within one breach window.
- **FR-006**: A deliberately saturated live or end-to-end run MUST demonstrate at
  least one governor shed event landing in the event log after this fix, and the
  check that the deployed world-01 daemon binary contains the governor MUST be
  performed and recorded on the board task (rebuild + restart if it does not).
- **FR-007**: The wiki notes whose sources include the changed files MUST be
  re-pinned after merge (grounding-freshness rule).

### Key Entities

- **Pending thought (debt input)**: one model-bound thought's kind, predicted
  seconds, and elapsed seconds — unchanged shape; only how the two seconds figures
  combine changes.
- **Debt**: the dimensionless budget-fraction sum the governor samples; its unit
  and threshold semantics are unchanged — 1.0 still means one thought consuming
  exactly its whole staleness budget.
- **Shed decision / governor event**: unchanged shape; expected to actually occur
  under saturation now.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In the world-01 saturation shape (multi-thought planner load,
  predictions ~1.6 s, elapsed 20–50 s, 8 ticks/sec), debt exceeds the shed
  threshold at the first sample and a shed fires within one breach window — where
  the same shape today yields debt ≈ 0 and no shed ever (red test first).
- **SC-002**: A single stuck thought's debt contribution is non-decreasing over
  successive samples while it remains pending — it can never fall to zero by
  overrunning its prediction.
- **SC-003**: For every pending set in which no thought exceeds its prediction,
  computed debt is bit-identical to the current implementation's output.
- **SC-004**: A deliberately saturated run (live world or end-to-end harness)
  produces at least one governor shed event in the event log with its debt
  arithmetic recorded, and the status surface shows effective speed below
  requested during the governed interval.
- **SC-005**: All existing governor and daemon tests pass unmodified except any
  that assert the zero-floor behavior itself, which are consciously retuned to the
  new doctrine.

## Assumptions

- The accrued arm uses the CURRENT sampled tick rate for elapsed seconds (as the
  predicted arm already does); reconstructing per-thought historical tick rates is
  deliberately out of scope — the approximation errs toward protection only when
  speed was recently raised, which is exactly when protection matters.
- Governor hysteresis constants and the ladder semantics (spec 028) are correct
  as-is; this feature fixes the debt input, not the controller.
- Rejection-grounded breach (feeding rejected-stale landings to the governor as
  ground-truth breach samples — option B on TASK-87) is OUT OF SCOPE: it adds a new
  input path from the landing door and is recorded on the board task as future
  hardening if accrued-drift debt proves insufficient in practice.
- Spec 031 (estimator breach-adoption) improves this feature's *predicted* inputs
  but is not a dependency: accrued-drift debt is correct and testable with any
  prediction quality, and the world-01 regression shape deliberately uses frozen
  optimistic predictions.
- The live-run verification (US2) may be satisfied by an end-to-end harness
  scenario if deliberately saturating a real world is impractical in the merge
  window; the deployed-binary check on world-01 is performed either way.
