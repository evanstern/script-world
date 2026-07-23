# Feature Specification: Extract the Intent-Landing Ladder into Named Rungs

**Feature Branch**: `022-landing-ladder-rungs`

**Created**: 2026-07-23

**Status**: Draft

**Input**: User description: "Extract the intent-landing ladder in internal/sim/loop.go handleCommand (inject_intent case) into a landIntent method with named rungs (TASK-70, from the 2026-07-22 team review improvement 1). Each rung becomes a named function matching the doctrine — fresh / adapted / hail-relaxed / superseded / guard-failed / stale — and the adapted/failed/hailTarget flag interplay is replaced by explicit rung outcomes. Pure refactor: behavior must be bit-identical; the determinism harness must prove same-seed byte-identical state-hash replay before and after. Add unit tests exercising each rung in isolation, including the hail special-cases (mutual-hailer, in-radius, moved-target). No event-schema or doctrine changes."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A maintainer can read the landing ladder as named doctrine rungs (Priority: P1)

Today the intent-landing decision — whether a cognition result lands fresh, lands adapted,
lands with a hail, or is rejected as superseded / stale / guard-failed — is one ~195-line
inline block inside the command handler, with the hail relaxation spliced inside the
guard-evaluation loop and three mutually interacting flags (`adapted`, `failed`,
`hailTarget`) carrying the outcome. A maintainer changing any rung must re-derive the
whole flag interplay. After this change, the landing decision reads as a sequence of
named steps that match the doctrine vocabulary, each producing an explicit outcome, and
the command handler merely invokes the ladder and translates its outcome into events.

**Why this priority**: this is the stated purpose of the task — the 2026-07-22 team
review judged this block the worst complexity hotspot in the core and the most likely
place for the next bug. Everything else in this spec is a safety net for this change.

**Independent Test**: read the extracted ladder top-to-bottom; verify each doctrine rung
(fresh / adapted / hail-relaxed / superseded / guard-failed / stale) is a named unit whose
outcome is an explicit value, and that no boolean flag set in one rung is consumed by a
different rung.

**Acceptance Scenarios**:

1. **Given** the refactored code, **When** a maintainer locates the landing logic,
   **Then** it lives in a dedicated method/type outside the command-handler switch, and
   each doctrine rung is a named function or named case with a doctrine-matching name.
2. **Given** the refactored code, **When** the rungs are inspected, **Then** the
   `adapted` / `failed` / `hailTarget` cross-rung flag interplay no longer exists —
   each rung returns an explicit outcome value consumed in one place.
3. **Given** any landing attempt, **When** the ladder runs, **Then** the events emitted
   (kind, payload fields, ordering) are exactly those the current inline block emits.

---

### User Story 2 - The refactor is provably behavior-identical (Priority: P1)

The simulation is deterministic and replay-audited: the same seed must produce a
byte-identical event timeline and state hash. A reviewer (or gate) must be able to prove
the refactor changed nothing observable — not "tests still pass" but bit-identical
replay on the existing determinism harness, plus the full race-checked test suite.

**Why this priority**: co-equal P1 — a refactor of the core landing path without this
proof is a regression risk, not an improvement. The task's acceptance criteria make the
determinism harness the explicit safety net.

**Independent Test**: run the existing determinism/replay byte-identity test suite on
the refactored code; every existing seed and scenario replays to the same timeline and
state bytes as before the refactor.

**Acceptance Scenarios**:

1. **Given** the existing determinism harness (same-seed timeline and replay
   byte-identity tests, including the hail replay determinism test), **When** it runs
   against the refactored code, **Then** every test passes unchanged — no test file
   edits, no golden-data updates.
2. **Given** the full test suite with the race detector, **When** it runs, **Then** it
   passes.
3. **Given** the refactor diff, **When** reviewed, **Then** it contains no event-schema
   change, no doctrine/constant change, and no change to any emitted payload.

---

### User Story 3 - Each rung is unit-testable in isolation (Priority: P2)

The review's core complaint is that the current shape makes rung-level testing
impossible — you cannot exercise the hail relaxation without staging a full command
round-trip. After extraction, each rung has direct unit tests, including the three hail
special-cases: the mutual-hailer (target is the actor's own hailer — land adapted, no
new hail), the in-radius hailable target (guard holds but the target is still hailed),
and the moved-target adapt case (guard holds at repaired coordinates — land adapted).

**Why this priority**: P2 only because it depends on the P1 extraction existing; it is
still a hard requirement of the task (board AC #3).

**Independent Test**: run the new rung unit tests; each doctrine rung and each hail
special-case has at least one test that exercises it directly through the extracted
ladder without a full command round-trip.

**Acceptance Scenarios**:

1. **Given** the extracted ladder, **When** the new unit tests run, **Then** each rung
   — fresh, adapted, hail-relaxed, superseded, guard-failed, stale — is exercised in
   isolation with its outcome asserted.
2. **Given** a landing whose target-present guard fails but whose target is the actor's
   own hailer, **When** the ladder runs, **Then** the outcome is adapted with no new
   hail — covered by a direct unit test.
3. **Given** a talk_to landing whose target is present in-radius and hailable, **When**
   the ladder runs, **Then** the landing is fresh (not adapted) and the target is
   hailed — covered by a direct unit test.
4. **Given** a landing whose target-present guard holds at coordinates that differ from
   the target's original position, **When** the ladder runs, **Then** the outcome is
   adapted — covered by a direct unit test.

---

### Edge Cases

- Dead or asleep actor: rejected as unavailable before any rung runs — ordering must be
  preserved exactly (dead check before asleep check, both before generation/staleness).
- Uncognized landings (no cognition class): today they skip the generation, staleness,
  and guard rungs entirely but still resolve/emit; the ladder must preserve this split.
- Guard failure on a talk_to whose target is dead or out of hail range: falls through to
  the plain guard rejection (no relaxation) — exactly as today.
- A landing that is both adapted (moved target) and hail-relaxed on different guards in
  the same guard list: outcome precedence and the single-hail emission must match today.
- Rejection events: a metered rejection emits its rejection record AND returns an error
  — the only command path that pairs the two; the extraction must not decouple them.
- Plan landings (multi-step plans) share the ladder's front rungs (unavailable /
  superseded / stale / guards) but diverge after: the extraction must keep the shared
  prefix identical.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The intent-landing decision MUST be extracted out of the command-handler
  switch into a dedicated, directly-invocable unit (method or small type) in the sim
  package.
- **FR-002**: Each doctrine rung — fresh, adapted, hail-relaxed, superseded,
  guard-failed, stale (plus the unavailable pre-checks) — MUST be a named function or
  named case whose name matches the doctrine vocabulary.
- **FR-003**: Rung results MUST be communicated as explicit outcome values; the
  `adapted` / `failed` / `hailTarget` cross-loop flag interplay MUST NOT survive the
  refactor.
- **FR-004**: The refactor MUST be behavior-identical: same inputs produce the same
  events (kind, payload, ordering), same errors (message text included, since errors
  surface to callers), and same state mutations as the current code.
- **FR-005**: The existing determinism harness (same-seed timeline test and all replay
  byte-identity tests) MUST pass unchanged — no edits to existing tests or fixtures.
- **FR-006**: New unit tests MUST exercise every rung in isolation, including the three
  hail special-cases (mutual-hailer, in-radius hail, moved-target adapt).
- **FR-007**: The full test suite MUST pass under the race detector.
- **FR-008**: No event schema, event payload, doctrine constant, or guard vocabulary
  change is permitted.
- **FR-009**: The grounding wiki note for the sim loop MUST be re-verified and re-pinned
  after the change lands (Principle IV).

### Key Entities

- **Landing ladder**: the ordered decision sequence a cognition result passes through
  when it lands on the world: unavailable pre-checks → superseded → stale → guard
  evaluation (with hail relaxation and adapt detection) → plan-or-goal resolution →
  outcome emission.
- **Rung outcome**: the explicit result of running the ladder — which doctrine outcome
  applied (landed fresh, landed adapted, rejected: unavailable / superseded / stale /
  guard-failed), plus any hail target to pause.
- **Hail special-cases**: mutual-hailer (target already hailing the actor), in-radius
  hailable target, moved-target adapt — the three relaxations spliced into guard
  evaluation today.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: The landing decision is readable as a linear sequence of named rungs; zero
  cross-rung mutable flags remain (verified by review of the extracted unit).
- **SC-002**: 100% of existing determinism and replay byte-identity tests pass with zero
  modifications to their files or fixtures.
- **SC-003**: Every doctrine rung and every hail special-case has at least one direct
  unit test that does not require a full command round-trip (≥ 6 rungs + 3 special-cases
  covered).
- **SC-004**: The full suite passes race-checked; the sim-loop wiki note is re-pinned to
  the merge commit.

## Assumptions

- "Bit-identical" is operationalized as: the existing determinism/replay test suite
  passes unchanged, since those tests already compare full event timelines and
  marshalled state bytes across seeds and replay. No new golden-file mechanism is
  required.
- Error message text is part of observable behavior (errors return to IPC callers), so
  rejection reason strings must be preserved verbatim.
- The plan-landing path (step-capped, roster-derived accept set) and the goal-resolution
  path stay where they are unless moving them is required to extract the ladder cleanly;
  either way their behavior is untouched.
- The rung names in code follow the doctrine words used by the review (fresh / adapted /
  hail-relaxed / superseded / guard-failed / stale); minor naming adjustments to match
  package conventions are acceptable as long as the doctrine mapping is obvious.
- This is TASK-70 on the board; the spec will be linked via spec-bridge before
  implementation.
