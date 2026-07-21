# Feature Specification: Parallel Local Tier

**Feature Branch**: `009-parallel-local-tier`

**Created**: 2026-07-21

**Status**: Implemented

**Input**: User description: "Parallel local LLM tier: the daemon's LLM orchestrator currently runs exactly one worker per tier, serializing every local-model call. Evidence from live session 2026-07-21: 130s queue waits behind 19s calls caused rejected-stale planner intents; best-effort musings dropped as 'tier busy' herds; conversation scenes starved. The local server already parallelizes natively (measured: 4 concurrent cogito:3b calls complete in 0.98s wall vs 3.8s for one cold call). Feature: a `parallel` setting in llm.json's local tier config (default 1) that runs N concurrent workers against the local tier; best-effort admission accounts for free slots; the cognition estimator's samples reflect true concurrent-rate latency; queue/prio semantics, health/breaker behavior, and the monthly spend meter remain correct under concurrency. Out of scope: per-class/per-provider model routing (TASK-35), cloud tier parallelism. Board task: TASK-45."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Timely village cognition at speed (Priority: P1)

A world operator runs an 8-agent village at 8×–32× game speed on a single local model.
Today every agent thought waits in a single-file line: a planner answer that took 19
seconds of thinking spent 130 seconds waiting behind other agents' thoughts, landed
stale, and was discarded. With local-tier concurrency configured, several thoughts are
in flight at once, queue wait collapses, and thoughts land within their staleness
budgets instead of being rejected on arrival.

**Why this priority**: rejected-stale thoughts are pure waste — the model did the work
and the world discarded it. This is the single largest observed cause of "the village
feels lobotomized at speed" and blocks every downstream feature that needs live
cognition at high speed.

**Independent Test**: run the same scripted world at the same speed with concurrency 1
vs 4 and compare the share of thoughts rejected for staleness at landing.

**Acceptance Scenarios**:

1. **Given** an 8-agent world at 8× with local concurrency 4, **When** all agents'
   planners come due in the same window (post-restart herd), **Then** the share of
   planner results rejected as stale at landing is dramatically lower than with
   concurrency 1 under the identical scenario.
2. **Given** concurrency N, **When** more than N requests arrive at once, **Then** the
   overflow waits in the same priority order as today and no request is lost.

---

### User Story 2 - Best-effort thoughts stop losing every race (Priority: P2)

Musings (ambient interiority) are deliberately drop-when-busy. Today "busy" means "any
single call in flight," so on an active world musings lose essentially every admission
race and the villagers' inner life goes silent. With N slots, a musing is refused only
when ALL slots are occupied.

**Why this priority**: interiority is atmosphere, not correctness — but its total
silence was a live, user-visible symptom ("the world went quiet"). Fixing admission is
nearly free once slots exist.

**Independent Test**: under a constant planner load that keeps exactly one slot busy,
musing admission success goes from ~0% (concurrency 1) to routinely admitted
(concurrency ≥2).

**Acceptance Scenarios**:

1. **Given** concurrency 4 with one long call in flight, **When** a musing requests
   admission, **Then** it is admitted to a free slot rather than dropped as busy.
2. **Given** all N slots occupied, **When** a musing requests admission, **Then** it is
   dropped exactly as today (drop-when-busy contract unchanged).

---

### User Story 3 - Honest speed governance under concurrency (Priority: P3)

The cognition governor decides what the model may think about by predicting how stale
the answer will land. Its latency estimates MUST reflect how the tier actually operates.
With concurrency enabled, completed calls feed the estimator the true concurrent-rate
latency, so routing decisions and the recorded telemetry stay honest — closing the gap
where a sequential calibration undersold real concurrent load.

**Why this priority**: wrong estimates re-create today's problem in the opposite
direction (admitting thoughts that will land stale). Depends on US1 existing.

**Independent Test**: drive the tier at full concurrency and compare the estimator's
converged seconds-per-point against wall-clock measurements of the same calls.

**Acceptance Scenarios**:

1. **Given** sustained concurrent load, **When** the estimator converges, **Then**
   predicted landing ticks for new thoughts are within the same order of accuracy as
   under serial operation (no systematic optimism).
2. **Given** operator-visible spend reporting, **When** many calls complete
   concurrently, **Then** the monthly spend meter equals the sum of the individual
   calls' costs (no lost or double-counted updates).

---

### Edge Cases

- Concurrency configured to 0, negative, or absurd values → treated as 1 (or a sane
  cap) with a visible warning at daemon start; the world never fails to boot over it.
- The local server accepts fewer simultaneous requests than configured (server-side
  queueing) → calls still complete; the estimator simply observes the longer effective
  latency; nothing deadlocks.
- Model cold-load stampede: N concurrent first-calls while the model is still loading →
  all complete or fail per the existing health rules; no crash, no stuck slot.
- A call fails or times out while others are in flight → the failure affects tier
  health/breaker accounting exactly once, and surviving in-flight calls are unaffected.
- Daemon shutdown with calls in flight → shutdown completes without hanging on
  stragglers beyond the existing call timeout discipline.
- Configuration absent (existing worlds' llm.json) → behaves exactly as today
  (concurrency 1); no migration required.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Operators MUST be able to configure the local tier's concurrency level in
  the world's LLM configuration file; when unset, the level is 1 and behavior is
  indistinguishable from today's.
- **FR-002**: With concurrency N, the system MUST allow up to N local-tier calls to be
  in flight simultaneously; requests beyond N wait, in today's priority-then-FIFO
  order, and are never reordered or lost by the concurrency mechanism.
- **FR-003**: Best-effort (drop-when-busy) requests MUST be refused only when all N
  slots are occupied; the drop-when-busy contract itself is unchanged.
- **FR-004**: Latency estimates that drive cognition routing MUST be fed from calls as
  they actually complete under concurrency, so predictions reflect true concurrent
  operation.
- **FR-005**: Tier health and failure-breaker semantics MUST remain correct under
  concurrency: each call's outcome is counted exactly once, and one failing call MUST
  NOT corrupt the accounting of others in flight.
- **FR-006**: The monthly spend meter MUST remain exact under concurrent completions
  (final metered total equals the sum of individual call costs).
- **FR-007**: Invalid concurrency values MUST degrade safely to a working configuration
  with a visible warning; a world MUST never fail to start because of this setting.
- **FR-008**: The cloud tier's behavior MUST be unchanged by this feature.
- **FR-009**: Per-class or per-provider model routing MUST NOT be introduced by this
  feature (reserved for the multi-provider division-of-labor design, TASK-35).

### Key Entities

- **Tier slot**: one unit of concurrent capacity against the local model server; the
  set of N slots replaces today's single implicit slot.
- **Admission verdict**: the decision to run now (free slot), wait (queued, ordered),
  or drop (best-effort with no free slot).
- **Latency estimate**: the running seconds-per-point figure the cognition governor
  consumes; under this feature it reflects concurrent-rate reality.
- **Spend meter**: the monthly budget ledger; must stay exact under concurrent updates.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In the reference scenario (8-agent world, 8×, post-restart planner herd),
  thoughts rejected for staleness at landing drop by at least 80% at concurrency 4
  versus concurrency 1.
- **SC-002**: Best-effort musing admission success rises from near-zero to at least 50%
  in the constant-single-load test at concurrency ≥2.
- **SC-003**: With the setting absent or 1, the full existing test suite passes
  unchanged and observed behavior is identical to pre-feature builds.
- **SC-004**: Four short concurrent calls complete in at most twice the wall time of
  one equivalent warm call on the reference local server (measured baseline: 4-in-0.98s
  vs 3.8s cold single).
- **SC-005**: After a sustained concurrent-load run, the spend meter total matches the
  sum of per-call costs exactly (zero drift), and no health/breaker state is left
  inconsistent.

## Assumptions

- The local model server (Ollama-compatible endpoint) accepts and services concurrent
  requests natively; measured on the development machine (4 concurrent calls, one
  loaded model). Server-side slot limits are the server's concern; the feature only
  needs calls to complete.
- Default concurrency stays 1 to preserve existing worlds byte-for-byte; opting in is a
  per-world configuration decision.
- A practical upper cap on configured concurrency is acceptable (protects against
  pathological configs); the exact cap is an implementation-plan decision. **Resolved:
  the cap is 16** (delegated to plan.md; rationale in research.md R2 — `queueCap` is 32,
  and 16 slots exceed any measured local-server benefit while bounding pathological
  configs). Out-of-range values clamp (never error): `< 1 → 1`, `> 16 → 16`, each with a
  daemon boot-line warning; absent/0 stays 1.
- Sequential-vs-concurrent calibration discrepancy (recorded on TASK-40) is mitigated,
  not owned, here: this feature makes live estimates honest; calibration procedure
  changes remain TASK-40 scope.
- Cross-world coordination of a shared local server (multiple daemons, TASK-24) is out
  of scope; this feature governs concurrency within one daemon.
