# Feature Specification: llm.json robustness knobs — in-loop cognition retry + configurable max_tokens

**Feature Branch**: `task-72-llm-robustness-knobs`

**Created**: 2026-07-23

**Status**: Draft

**Input**: User description: "llm.json robustness knobs: in-loop cognition retry + configurable max_tokens (TASK-72). Two related robustness knobs, one PR. (a) In-loop transport retry: a single provider_error currently terminates the whole cognition; only conversations get a one-shot retry. A flaky local call wastes an entire planner/metatron thought and waits out the 120-tick rearm. Add ONE in-loop retry on TermProviderError before terminating, without disturbing estimator/breaker doctrine, and observable in the recorded trail. (b) Configurable max_tokens: per-call-site hardcodes today; add per-kind llm.json overrides following the established warn-not-error clamp convention."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A flaky provider call no longer wastes a whole thought (Priority: P1)

An operator runs a village against a local model that occasionally drops or garbles a
single request (transient transport failure). Today, one such failure during a villager's
planner cognition or a metatron console turn kills the entire thought: the agent loses
its turn to act and waits out the full re-arm interval before thinking again. With this
feature, the cognition quietly retries the failed model call once; if the retry succeeds,
the thought completes as if nothing happened, and the recovery is visible in the decision
trail. If the retry also fails, the cognition terminates exactly as it does today.

**Why this priority**: this is the pain that motivated the task — on flaky local tiers,
single-call blips cost entire planner/metatron cognitions plus the 120-tick re-arm wait,
which compounds into visibly duller agents. It delivers value with zero configuration.

**Independent Test**: can be fully tested by driving a tool-loop cognition against a fake
provider that fails exactly once mid-loop — the cognition must complete successfully with
the retry visible in its recorded trail — and against one that fails twice consecutively —
the cognition must terminate with the same outcome and error as today.

**Acceptance Scenarios**:

1. **Given** a planner or metatron tool-loop cognition in progress, **When** one model
   call fails with a transport-level provider error, **Then** the loop retries that call
   once, and on success the cognition proceeds and completes normally.
2. **Given** a cognition whose model call failed and whose retry also fails, **When** the
   second failure returns, **Then** the cognition terminates with the same termination
   class and error surface it has today (no third attempt, no new failure mode).
3. **Given** a cognition that recovered via retry, **When** an operator inspects the
   cognition's recorded trail, **Then** the retried round is visibly marked — the
   recovery is never silent.
4. **Given** a model call refused at admission (budget exhausted, queue full, tier busy
   or down) or cancelled by context, **When** the failure returns, **Then** no retry
   occurs and the cognition terminates exactly as today.
5. **Given** a cognition that recovered via retry, **When** latency estimation and
   circuit-breaker state are inspected afterward, **Then** they match what a single
   successful cognition produces today: no extra latency observations, no change in
   breaker strikes (busy-is-not-down preserved).

---

### User Story 2 - Operator tunes cognition token budgets in llm.json (Priority: P2)

An operator whose local model needs more (or less) room per response tunes the token
budget for the planner loop, the metatron console turn, and nightly consolidation
directly in the world's `llm.json` — no rebuild. Absent knobs mean today's exact
behavior. A nonsensical value never prevents the world from booting: it is clamped to a
sane bound and the operator sees a warning at boot, following the same convention as the
existing `loop_max_rounds` knob.

**Why this priority**: valuable tuning surface, but it has a safe default (today's
hardcodes) and no urgency — worlds run fine without touching it.

**Independent Test**: can be fully tested by booting worlds with absent, valid,
zero, negative, and oversized knob values and observing the effective budgets and
boot warnings; no code change between runs.

**Acceptance Scenarios**:

1. **Given** an `llm.json` with no token-budget knobs (any existing world), **When** the
   world boots, **Then** effective budgets equal today's built-in values and no warning
   is emitted.
2. **Given** an `llm.json` setting a valid in-range budget for one of the three kinds,
   **When** the world boots, **Then** that kind's cognitions use the configured budget
   and the other kinds keep their defaults.
3. **Given** an out-of-range value (negative, or above the upper bound), **When** the
   world boots, **Then** the world boots successfully, the value is clamped, and an
   operator-facing warning names the field, the offending value, and the effective value.
4. **Given** a knob explicitly set to 0, **When** the world boots, **Then** the kind uses
   its default budget with no warning (0 means "unset", matching `loop_max_rounds`).

---

### Edge Cases

- Retry succeeds but the model returns no actionable tool call → the loop proceeds to its
  normal "model done" outcome; the retry is still recorded.
- Context is cancelled between the failure and the retry (or during the retry) → the
  cognition terminates as context-done, exactly as today; the attempt is not counted as a
  provider failure.
- A tool handler's infrastructure failure (a dispatch that returns an error after the
  model call succeeded) is NOT a transport failure → no retry; terminates as today.
- The failed attempt must not consume a round from the loop's round cap — rounds count
  model responses, and a failed call produced none.
- Provider failure on the very first round (empty transcript beyond the seed) → retry
  works the same as mid-loop.
- The conversation subsystem's existing one-shot retry (utterance/outcome sites) is a
  separate mechanism and must be left untouched — no double-retry stacking for
  conversation cognitions, which do not run in the tool loop.
- Token-budget knobs and `loop_max_rounds` coexist in the same file; each normalizes
  independently and warnings accumulate.
- A world whose `llm.json` predates this feature (no new fields) must behave
  byte-for-byte as today.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The tool-loop cognition driver MUST retry a failed model call exactly once
  per cognition run when the failure classifies as a transport-level provider error.
  This covers the cognition families that run in the tool loop today: villager planner
  thoughts and metatron console turns.
- **FR-002**: Retry MUST trigger only on transport-level provider errors. Admission
  refusals (budget exhausted, queue full, tier busy, tier down), context cancellation or
  deadline, and tool-handler infrastructure failures MUST NOT trigger a retry and MUST
  terminate the cognition exactly as they do today.
- **FR-003**: When the retry also fails, the cognition MUST terminate with the same
  termination classification and error propagation as a single failure does today; there
  is never a second retry.
- **FR-004**: A retried round MUST be observable in the cognition's recorded trail (the
  per-cognition record/event stream), carrying at least the fact that a retry occurred
  and the first failure's reason. A silent retry is a defect.
- **FR-005**: Retry MUST NOT change how the latency estimator is fed: the loop's model
  calls remain excluded from per-call latency observation (successes-only, whole-run
  wall-time sampling discipline unchanged), and a recovered cognition produces no more
  observations than a normal successful cognition does today.
- **FR-006**: Retry MUST NOT change circuit-breaker semantics: each individual provider
  failure strikes (or does not strike) the breaker exactly as it does today —
  busy-is-not-down is preserved, and the retry adds no new strike classification.
- **FR-007**: The world's `llm.json` MUST accept optional per-kind token-budget overrides
  for exactly three budgets: the villager planner tool-loop round budget (default 512),
  the metatron console-turn round budget (default 1024), and the nightly consolidation
  budget (default 1024). Absent or 0 means the default.
- **FR-008**: Token-budget knobs MUST follow the established warn-not-error clamp
  convention: a valid in-range value (1 to the upper bound, 4096) passes through;
  negative values fall back to the default with an operator warning; values above the
  upper bound clamp to it with an operator warning; a world MUST never fail to boot over
  these knobs. Warnings surface through the same boot-time channel as existing knob
  warnings.
- **FR-009**: All other model-call token budgets (conversation utterance and outcome,
  meeting, narrator, metatron digest) MUST remain at their current built-in values,
  unaffected by the new knobs.
- **FR-010**: A configuration with none of the new fields MUST produce behavior
  indistinguishable from today's (defaults byte-for-byte compatible).

### Key Entities

- **Token-budget knobs**: three new optional operator-facing fields in the world's
  `llm.json`, one per cognition kind (planner loop, metatron turn, consolidation), each
  normalizing independently to (effective value, optional warning).
- **Cognition trail record**: the per-cognition sequence of recorded calls/events that
  operators inspect (decision-trace view); gains visibility of the retried round.
- **Termination taxonomy**: the existing classification of how a cognition ends
  (landed, model-done, cap-exhausted, admission-refused, provider-error, context-done);
  unchanged in shape — retry only affects how many attempts precede a provider-error
  termination.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A cognition experiencing exactly one transient provider failure completes
  successfully 100% of the time (against a deterministic fail-once fake), where today it
  fails 100% of the time.
- **SC-002**: A cognition experiencing two consecutive provider failures terminates
  identically to today's single-failure behavior (same classification, same error),
  verified by tests exercising both paths.
- **SC-003**: Every retry is discoverable from the recorded trail alone — an operator
  (or test) can count retries without reading logs.
- **SC-004**: Latency-estimator sample counts and breaker state transitions in a
  retry-recovery run are identical to those of an equivalent no-failure run.
- **SC-005**: An operator can change any of the three token budgets by editing one field
  in `llm.json` and rebooting — no rebuild; the new budget is observable on the wire.
- **SC-006**: 100% of worlds boot regardless of the knob values supplied (absent, zero,
  valid, negative, oversized); invalid values always produce a clamped effective value
  plus a warning, never a failure.
- **SC-007**: The full test suite (including race detection) passes; existing worlds
  (config files without the new fields) show no behavior change.

## Assumptions

- **Scope of the knobs is the three budgets named on TASK-72** (planner loop 512,
  metatron console turn 1024, consolidation 1024). The remaining hardcoded budgets are
  deliberately excluded: conversation utterance (128) was explicitly probed and chosen in
  TASK-42, conversation outcome (224), meeting (72), narrator (800), and metatron digest
  (400) are tuned values with no demonstrated need for operator control.
- **The metatron knob governs the console-turn budget only.** The metatron digest call is
  a different budget (400) and stays hardcoded, even though it shares the metatron call
  kind.
- **Upper clamp bound is 4096** — 4–8× the current defaults, enough headroom for verbose
  models while bounding pathological configs; mirrors the bounded-knob doctrine of the
  existing worker/round knobs (which use 16).
- **The retry is immediate** (no backoff): the failure mode being addressed is a
  transient blip, and the cognition already runs under its caller's existing deadline;
  a failed attempt plus retry stays inside today's timeout envelopes.
- **A failed attempt consumes no loop round**: the round cap counts model responses;
  a transport failure produced none.
- **The conversation retry (TASK-42) is out of scope and unchanged** — conversations do
  not run in the tool loop, and their utterance/outcome retry discipline stays as is.
- **Retry counting is per cognition run**, not per round: one recovery per thought,
  matching the "ONE retry" language on the task and the conversation precedent (one
  retry per scene).
