# Feature Specification: Estimator Breach Adoption — follow sustained slowdown instead of freezing

**Feature Branch**: `031-estimator-breach-adoption`

**Created**: 2026-07-24

**Status**: Draft

**Input**: User description: "Adaptive estimator re-seed (TASK-86): the cognition live seconds-per-point Estimator must follow sustained load-induced slowdown instead of freezing. Today a step change larger than SpikeFactor(3.0)x the current estimate causes 100% of samples to be excluded as spikes, freezing the estimate at its seed forever (world-01 evidence: gemma spike_rate 1.0, estimate pinned 0.524 s/pt while actual ran 7-17 s/pt; router admitted everything; 17/31 planner thoughts rejected-stale at landing). Chosen approach (breach-adoption, option A on TASK-86): store sample VALUES in the existing WindowSize ring alongside spike flags; when the rolling spike rate first breaches BreachRate over a full window, re-seed the estimate to the window median, reset the window, and emit telemetry for the adoption. One-shot spikes (1-2 in 20) must still be rejected with the estimate barely moving. The cog.recalibration_recommended signal keeps firing but now has an actor."

## The defect this fixes (world-01, 2026-07-23)

The live seconds-per-point estimator rejects any sample larger than a fixed factor
(3.0×) of its current estimate. A **step change** larger than that factor — exactly
what load-induced slowdown produces — is therefore indistinguishable from an endless
run of one-shot spikes: every sample is excluded, the estimate freezes at its seed,
and the recalibration signal fires with no actor. Everything downstream trusts the
frozen number: the router admits every thought (predicted drift ~13 ticks vs a
1200-tick budget), the landing door then rejects what the router admitted (17 of 31
planner thoughts rejected-stale, predicted 1.6 s vs actual 21–50 s), and the
adaptive-throttle governor computes debt from the same fiction. The doctrine sentence
"one-shot lag spikes are rejected while systemic drift is followed" is violated in
precisely the systemic case it promises to cover.

The insight: lag spikes and step changes cannot be distinguished by **magnitude** on
one sample — only by **persistence** across samples. The estimator already owns the
persistence classifier (the rolling spike-rate window and its breach threshold); this
feature makes that classifier the **actor** instead of an unread signal.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - The estimate follows a sustained slowdown (Priority: P1)

An operator runs a world at high speed while the serving provider is under real
concurrent load, so each cognition genuinely costs many times what calibration
measured on an idle endpoint. Within one observation window the live estimate adopts
the new reality, and from then on the router's admission arithmetic is truthful:
thoughts that cannot land inside their staleness budget are suppressed to their
deterministic floor **before** dispatch, instead of being admitted and then rejected
stale at the landing door after burning a full model call.

**Why this priority**: this is the defect itself — the frozen estimate disarms every
protection layer at once. Nothing else in this feature matters if the estimate cannot
move.

**Independent Test**: feed an estimator seeded at the world-01 value (0.52 s/pt) a
sustained stream of ~12 s/pt samples and assert the estimate converges to the new
regime within one full window; then assert the router's verdict for the world-01
planner shape flips from blind admission to honest arithmetic.

**Acceptance Scenarios**:

1. **Given** an estimator seeded at 0.52 s/pt, **When** it observes 20 consecutive
   samples of ~12 s/pt (every one beyond the spike factor), **Then** by the sample
   that completes the window the estimate equals the window median (~12 s/pt), not
   the seed.
2. **Given** the adopted estimate, **When** the router predicts drift for a planner
   thought at any ladder speed, **Then** the prediction reflects the adopted rate —
   at speeds where true drift exceeds the class budget the thought is suppressed
   pre-dispatch rather than rejected post-landing.
3. **Given** an adoption has occurred and load later returns to normal, **When**
   subsequent samples fall at or below the adopted estimate, **Then** the estimate
   follows back down through ordinary weighted averaging (fast samples are never
   spikes) — no freeze in the downward direction either.

---

### User Story 2 - One-shot lag spikes are still rejected (Priority: P2)

A single slow call — a model being swapped into memory, a one-off network stall —
must not inflate the estimate. The existing spike-rejection behavior is preserved:
isolated spikes are excluded from the estimate entirely and merely counted.

**Why this priority**: the fix must not trade one failure mode for its mirror image.
Adoption is only legitimate when the window proves persistence.

**Independent Test**: feed a healthy estimator occasional spikes (1–2 per window)
between normal samples and assert the estimate is unchanged by the spikes.

**Acceptance Scenarios**:

1. **Given** a stable estimate, **When** 1–2 samples in a window exceed the spike
   factor while the rest are normal, **Then** the spike samples contribute nothing to
   the estimate and no adoption occurs (spike rate stays under the breach rate).
2. **Given** a stable estimate, **When** spikes arrive at a rate that never fills the
   breach fraction of a full window, **Then** the estimate never jumps — it moves
   only by ordinary weighted averaging over non-spike samples.

---

### User Story 3 - Adoption is auditable (Priority: P3)

When the estimator re-seeds itself, an operator reading the event stream can see
that it happened, when, and by how much: the prior estimate, the adopted estimate,
and the window evidence that justified it. The existing recalibration-recommended
signal keeps firing under the same conditions as today, so external dashboards and
habits keep working.

**Why this priority**: the project's doctrine is that every automatic decision must
leave an auditable record; an estimator that silently re-seeds would be a new kind of
invisible state.

**Independent Test**: drive an estimator through a breach and assert exactly one
adoption record is emitted carrying prior estimate, adopted estimate, and window
statistics, alongside the existing recalibration signal.

**Acceptance Scenarios**:

1. **Given** a breach-and-adoption, **When** the operator inspects the event stream,
   **Then** one record shows the prior estimate, the adopted estimate, the observed
   spike rate, and the window size.
2. **Given** an adoption, **When** the window refills and drift persists at the new
   level without breaching against the adopted estimate, **Then** no further adoption
   records appear (re-arm semantics match the existing breach signal).

---

### Edge Cases

- **Window not yet full**: no breach verdicts — and therefore no adoption — until a
  full window of samples exists (existing semantics preserved).
- **Repeated breaches**: after adoption the window resets and breach detection
  re-arms against the adopted estimate; a second genuine step (e.g. load doubling
  again) produces a second breach-and-adoption after another full window.
- **Bimodal load (only ~a third of samples slow)**: the window median then sits in
  the normal-sample population, so adoption moves the estimate little — correct,
  because the majority regime IS roughly the current estimate; the breach signal
  still fires for the operator.
- **Downward step (system got faster)**: samples below the estimate are never
  classified as spikes, so the ordinary weighted average follows down; adoption never
  fires on improvement.
- **Concurrent observation**: multiple in-flight cognitions completing at once must
  not corrupt the window or double-adopt (the estimator is shared per provider).
- **Zero/absent seed**: estimators are always constructed with a positive seed
  (calibration profile or bootstrap default); adoption preserves that invariant —
  the adopted value is a median of observed positive durations.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The estimator MUST retain the observed per-point duration value of each
  of the most recent WindowSize samples, alongside its spike classification.
- **FR-002**: On the sample that first drives the rolling spike rate over BreachRate
  across a full window (the same condition that raises the existing recalibration
  signal), the estimator MUST adopt the median of the retained window values as its
  new estimate.
- **FR-003**: Adoption MUST reset the sample window and re-arm breach detection, so
  subsequent samples are judged against the adopted estimate and a fresh window must
  fill before any further breach or adoption.
- **FR-004**: Samples beyond the spike factor that do NOT complete a breach MUST
  continue to be excluded from the estimate exactly as today — isolated spikes never
  move the estimate.
- **FR-005**: Every adoption MUST be observable in the world's event stream as a
  record carrying at minimum: the prior estimate, the adopted estimate, the observed
  spike rate, and the window size. The existing recalibration-recommended signal MUST
  continue to fire under its current conditions.
- **FR-006**: Adoption arithmetic MUST be deterministic — pure over the retained
  samples, with no wall-clock reads and no randomness (same doctrine as routing).
- **FR-007**: The estimator's tuning constants (spike factor, window size, breach
  rate, smoothing weight) remain doctrine: unchanged by this feature and never
  runtime-tunable.
- **FR-008**: The calibration contract in specs/007-cognition-horizon MUST be updated
  to document the adoption semantics, and the wiki cognition note re-pinned after
  implementation (grounding-freshness rule).

### Key Entities

- **Estimator**: one provider's live seconds-per-point estimate; now carries a ring
  of the last WindowSize observed values with their spike flags, an armed/breached
  state, and the adoption behavior.
- **Sample window**: the ring of retained {value, spike} pairs; the persistence
  classifier. Its median is the adoption value.
- **Adoption record**: the audit artifact of one re-seed — prior estimate, adopted
  estimate, spike rate, window size; lands in the world event stream beside the
  existing recalibration signal.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An estimator seeded at 0.52 s/pt fed sustained ~12 s/pt samples (the
  world-01 freeze shape) reaches the new regime within one full window — 20 samples —
  where today it never converges at all.
- **SC-002**: With ≤2 spike samples per window, the estimate after the window equals
  what the pre-change behavior produces (bit-identical arithmetic on the non-spike
  path) — zero regression in one-shot rejection.
- **SC-003**: In a saturated-load scenario, admission verdicts computed from the
  adopted estimate track reality: thoughts whose true drift exceeds their staleness
  budget are suppressed before dispatch, and the landing door's stale-rejection count
  for admitted thoughts drops accordingly (world-01 replay shape: 17/31 planner
  thoughts rejected-stale → ~0 once predictions are honest at the same speed).
- **SC-004**: Every adoption in a run is visible in the event stream with its
  arithmetic; a run with no sustained shift emits zero adoption records.
- **SC-005**: All existing estimator tests pass unmodified except those asserting the
  freeze behavior itself, which are consciously retuned to the new doctrine.

## Assumptions

- The doctrine constants (SpikeFactor 3.0, WindowSize 20, BreachRate 0.3, EWMA weight
  0.2) are correct as-is; this feature changes what happens at breach, not when a
  breach is declared.
- The window **median** is the right adoption value: robust to a mixed window (some
  normal samples, some slow), deterministic, and requiring no new tuning knob. A
  mean was rejected as spike-sensitive; a max as overshooting.
- Adoption scope is the estimator only. The governor's debt formula (TASK-87) is a
  separate, independently shippable fix; this feature improves the governor's inputs
  but does not touch its arithmetic.
- The calibration profile file on disk remains human-written only; adoption changes
  the process-lifetime estimate, never the recorded baseline.
- The existing recalibration-recommended event's payload shape stays
  backward-compatible; the adoption record is additive (new event type or additive
  fields), so existing consumers (TUI digest catalog, tests) keep working with at
  most catalog registration.
