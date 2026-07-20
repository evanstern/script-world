# Feature Specification: The Cognition Horizon

**Feature Branch**: `task-32-cognition-horizon`

**Created**: 2026-07-20

**Status**: Draft

**Input**: User description: "The cognition horizon: a deterministic substrate that scopes LLM authority by decision timescale vs turn latency in game time. Event-type registry with Fibonacci-point thought costs and game-time staleness budgets; a setup/calibration stage benchmarking the host+LLM to seconds-per-point, continuously re-estimated from telemetry with spike rejection; a deterministic router (no LLM in the loop) gating which decisions may go to the model at the current speed; landing-side enforcement (staleness stamp, guard re-validation with an adapt/reject+record/learn ladder); future-dated prompts and guarded conditional plans (timed guards subsume act-at-time-T); explicit pause semantics; staleness telemetry with causality ids first."

**Doctrine**: decision-4 (cognition horizon). A model turn takes real time; the world keeps moving while a mind thinks. Latency measured in game time scales with speed (a ~50s local planner turn is 50 game-seconds of drift at 1x but ~27 game-minutes at 32x), so the system must scope **what the model is allowed to decide** by **how stale its answer will be when it lands** — deterministically, with no model in the routing loop. Speed is never capped to protect cognition; cognition is scoped to survive speed.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Staleness is measured, never guessed (Priority: P1)

An operator sets up a world on their own hardware and model. A one-time calibration stage benchmarks that host+model combination against a uniform reference workload, producing a local "seconds-per-point" profile. From then on, every thought the simulation requests — planner turn, musing, conversation — leaves a durable telemetry trail: when the prompt snapshot was taken (game tick), what it was predicted to cost, when the result landed, what it actually cost, what triggered it, and what became of it. The operator can trace any action back through the thought that produced it to the stimulus that provoked the thought.

**Why this priority**: Every other behavior in this feature (routing, budgets, enforcement, tuning) acts on these measurements. Built first, it also quantifies the problem on the operator's real hardware before any gating behavior changes.

**Independent Test**: Run calibration on a fresh world, then run the world at any speed and inspect the event log: every model call has a complete telemetry record with causality references. No behavior change to the simulation itself.

**Acceptance Scenarios**:

1. **Given** a configured host and model, **When** the operator runs the calibration stage, **Then** a seconds-per-point profile is produced, stored with the world's configuration, and reported in human-readable form.
2. **Given** a running world, **When** any model call completes or fails, **Then** a telemetry record exists carrying: snapshot tick, landing tick, point cost, predicted and actual wall duration, the triggering event reference, and the outcome.
3. **Given** hours of telemetry in which observed latency has drifted systematically (e.g., host under new load), **When** the live estimate is compared to recent observations, **Then** it has followed the drift, while isolated spikes were excluded from the estimate but counted separately.
4. **Given** any executed intent, **When** the operator inspects the event log alone, **Then** the full chain — stimulus event → thought → intent → resulting action — is reconstructable from causality references.

---

### User Story 2 - Doomed thoughts are never attempted (Priority: P2)

Every decision type that can reach the model is registered ahead of time with two static values: a **thought cost** in Fibonacci points (1, 2, 3, 5, 8, 13 — ordinal, host-independent, a property of the prompt shape) and a **staleness budget** in game time (how long the decision stays valid — a property of the fiction). A deterministic router combines the point cost, the calibrated seconds-per-point, and the current speed into a predicted game-time drift, and only sends the decision to the model if the prediction fits inside the budget. Otherwise the decision class's registered degrade action runs instead (skip, fall to reflex, or route to a faster tier when one is configured). No model is ever consulted to make this determination.

**Why this priority**: This is the doctrine made mechanical. It prevents the system from burning wall-clock and tier capacity on thoughts that will arrive dead, and it is what lets high speeds coexist with a slow model.

**Independent Test**: With a calibrated profile, run the same trigger at 1x and at 32x and observe the router's recorded verdicts diverge deterministically.

**Acceptance Scenarios**:

1. **Given** speed 32x and a calibrated profile where a planner turn's predicted drift exceeds the planner class budget, **When** a planner trigger fires, **Then** no model call is made, the class's degrade action runs, and a routing telemetry record captures the suppressed decision and its arithmetic.
2. **Given** speed 1x and the same trigger, **When** the router evaluates, **Then** the decision routes to the model as today.
3. **Given** identical registry values, calibration profile, and speed, **When** the router evaluates the same decision twice, **Then** the verdict is identical (pure function; no model, no randomness, no wall clock).
4. **Given** a decision type not present in the registry, **When** the world starts, **Then** startup fails with an explicit error naming the unregistered type — intentional categorization is mandatory.

---

### User Story 3 - Stale intents never act (Priority: P3)

When a thought result lands, enforcement happens against the world as it is *now*, not as it was when the prompt was snapshotted. Staleness is measured exactly — game ticks elapsed between snapshot and landing — and checked against the class budget. Intents carry the assumptions they were formed under (guards), which are re-validated deterministically at landing. Failure follows a defined ladder: **adapt** (cheap deterministic repair when the intent's spirit survives, e.g., a target who moved is re-resolved), **reject + record** (fall to the reflex floor, emitting a rejection event — never a silent void), **learn** (rejections are classified as prediction-miss vs world-change and counted separately, so mistuned budgets surface for human retune and lag spikes never pollute heuristics).

**Why this priority**: This is the safety layer that makes wrong predictions harmless. Prediction (P2) is advisory; landing enforcement is authoritative.

**Independent Test**: Inject artificial latency into the model path and verify stale intents are rejected with recorded reasons while the reflex floor covers, and that no intent older than its budget ever executes.

**Acceptance Scenarios**:

1. **Given** an intent landing after more game ticks than its class budget, **When** it is validated, **Then** it does not execute; a rejection record with reason "stale" and the measured staleness is emitted; the reflex floor covers the agent.
2. **Given** an intent whose guard fails but whose deterministic repair is possible within budget (target moved to a reachable location), **When** it lands, **Then** the adapted intent executes and the adaptation is recorded.
3. **Given** a high-salience event (attack, fire, witnessed death) striking an agent while their thought is in flight, **When** the result lands, **Then** it is discarded as superseded, recorded as such, and a prompt re-plan is scheduled honoring the existing per-agent debounce floor.
4. **Given** a rejected planner intent, **When** the rejection is classified, **Then** prediction-miss (actual latency far over predicted) and world-change (guard failed due to events since snapshot) increment distinct counters, and a persistently elevated failure rate on one decision class is surfaced in telemetry for human retuning — budgets are never widened automatically.
5. **Given** an agent who died or fell asleep while their thought was in flight, **When** the result lands, **Then** it is rejected with that reason recorded (today's refusal, minus the silence).

---

### User Story 4 - Thoughts aim at the world they will land in (Priority: P4)

Prompts stop pretending thought is instant. A prompt snapshot tells the model when its decision will take effect ("it is 09:00; your decision lands around 09:30"), using the same prediction the router computed. The model may return a guarded conditional plan — "head to the square; if Rowan is gone by the time I arrive, check the well" — including timed guards ("when the meeting bell rings, attend"), which are evaluated deterministically by the executor. Timed guards are the mechanism for act-at-time-T; no separate scheduler exists.

**Why this priority**: Pure quality improvement layered on a safe substrate — P1–P3 make stale thought harmless; this makes thought less stale by construction.

**Independent Test**: Inspect prompt snapshots for the landing estimate; return a guarded plan from a scripted model stub and verify the executor holds, fires, and expires guards deterministically.

**Acceptance Scenarios**:

1. **Given** a planner prompt snapshot, **When** it is assembled, **Then** it states the current game time and the predicted effective game time of the decision.
2. **Given** a returned plan with a timed guard "at tick T do X", **When** ticks advance to T and the guard's validity window is open, **Then** X executes with no model involvement at firing time.
3. **Given** a guarded plan whose guard never fires within its validity window, **When** the window closes, **Then** the plan expires and the expiry is recorded.

---

### User Story 5 - Pause has defined cognition semantics (Priority: P5)

**Decision**: pause means *the world freezes and the minds catch up*. No new cognition starts while paused (scheduling is tick-driven and ticks stop), but in-flight thoughts and conversations complete on the wall clock and land at the frozen tick. Because staleness is measured in game ticks, everything landing during a pause lands at staleness zero — pause is the one state where thought fidelity is perfect. This blesses today's accidental behavior as doctrine, with enforcement (guards, supersede) still applying at landing.

**Why this priority**: Smallest slice; mostly codifying and testing behavior that already exists, so the semantics are chosen rather than accidental.

**Independent Test**: Pause with thoughts and a conversation in flight; verify completion, landing at the frozen tick, zero measured staleness, no new jobs until resume.

**Acceptance Scenarios**:

1. **Given** a paused world with a planner call in flight, **When** the call completes, **Then** the intent lands at the frozen tick with staleness 0 game ticks, guards still validated.
2. **Given** a paused world, **When** wall time passes, **Then** no new planner, musing, or conversation jobs start.
3. **Given** a conversation founded before the pause, **When** it completes during the pause, **Then** the full scene lands atomically at the frozen tick.
4. **Given** a resume after a long pause, **When** ticks flow again, **Then** cognition cadence self-heals with no burst compensating for the paused interval.

---

### Edge Cases

- **One-shot lag spike** (observed latency at or beyond the spike threshold, e.g. 3× predicted): the sample is excluded from the live estimate and counted; the landing ladder still measures true staleness, so the late result is rejected if over budget. A spike *rate* above threshold within a window is itself drift signal and raises a recalibration recommendation.
- **Speed changes or pause while a thought is in flight**: staleness is defined as landing tick minus snapshot tick — the game ticks that actually elapsed — so enforcement is exact under any speed trajectory. Only the router's *prediction* assumes the current speed persists; mispredictions are caught at landing.
- **No calibration profile present**: the system bootstraps with a documented pessimistic default until enough live samples accumulate; under bootstrap, routing at high speeds prefers the degrade action (fail toward reflex, never toward stale action).
- **Model returns unusable output after a long wait**: same drop contract as today, but the outcome is recorded (reason: unusable), never silent.
- **Registry gap**: any code path that would request a model call for an unregistered decision type is a startup-time failure, not a runtime surprise.
- **Uncapped speed (max)**: remains refused when a model is configured; the router does not make max safe, because prediction at unbounded speed is meaningless.

## Requirements *(mandatory)*

### Functional Requirements

**Registry — intentional categorization**

- **FR-001**: The system MUST maintain a registry in which every decision type that can reach the model is declared with: a thought cost in Fibonacci points (1, 2, 3, 5, 8, 13), a staleness budget expressed in game time, and a degrade action. The registry is static content, versioned with the world format.
- **FR-002**: World startup MUST fail, naming the offender, if any model-reaching decision type lacks a registry entry.
- **FR-003**: Point values MUST be ordinal and host-independent (a property of prompt shape: context size, output size, call count) — never expressed in wall-clock units.

**Calibration — the local bridge from points to seconds**

- **FR-004**: A setup/calibration stage MUST benchmark the configured host+model against a uniform reference workload and persist a seconds-per-point profile with the world's configuration.
- **FR-005**: The system MUST continuously re-estimate seconds-per-point from live telemetry using a spike-robust estimator: samples beyond a spike threshold are excluded from the estimate but counted; a sustained spike rate raises a recalibration signal. Systemic drift is followed; one-shot noise is not.
- **FR-006**: Absent any profile, the system MUST operate under a documented pessimistic bootstrap default and converge as live samples arrive.

**Router — deterministic gating**

- **FR-007**: A router MUST decide, before any model call, whether the decision may go to the model: route only if predicted wall latency (points × seconds-per-point) converted to game time at the current speed fits within the class staleness budget. The router MUST be a pure function of registry values, the calibration profile, and current speed — no model, no randomness, no wall clock reads.
- **FR-008**: When routing away from the model, the class's registered degrade action MUST run (skip, reflex floor, or faster tier when configured), and the suppressed decision MUST be recorded with its arithmetic.

**Landing — authoritative enforcement**

- **FR-009**: Every prompt snapshot MUST be stamped with its snapshot tick, its triggering event reference, and its prediction (points, predicted wall duration, predicted landing tick).
- **FR-010**: At landing, measured staleness (landing tick − snapshot tick) MUST be checked against the class budget; over-budget results MUST NOT execute.
- **FR-011**: Intents MUST carry deterministic guards (the assumptions they were formed under: target location/aliveness, reachability, absence of high-salience interrupt, timing) that are re-validated at landing.
- **FR-012**: Guard failure MUST follow the ladder: adapt (deterministic repair within budget) → reject + record → learn. A rejected planner intent MUST schedule a prompt re-plan honoring the per-agent debounce floor.
- **FR-013**: Rejections MUST be classified — prediction-miss vs world-change — with distinct counters; persistently elevated failure on one class MUST surface in telemetry for human retune. Budgets and points MUST NOT self-adjust.
- **FR-014**: A per-agent generation counter MUST be bumped by high-salience events; in-flight results from an older generation MUST be discarded as superseded at landing.
- **FR-015**: Every requested thought MUST terminate in exactly one recorded outcome: landed, adapted, rejected-stale, rejected-guard, superseded, expired, rejected-dead/asleep, unusable, or suppressed-by-router. Silent failure is eliminated.

**Prompts and plans — latency-aware thought**

- **FR-016**: Prompt snapshots MUST state the current game time and the predicted effective game time of the decision.
- **FR-017**: The plan vocabulary MUST support guarded conditional plans, including timed guards, each with a validity window; guards are evaluated deterministically at execution time, and expired plans are recorded as expired. Timed guards are the sole act-at-time-T mechanism.

**Pause**

- **FR-018**: Pause semantics are: no new cognition starts while paused; in-flight thoughts and conversations complete and land at the frozen tick; staleness measured in game ticks is therefore zero across a pause; landing enforcement (guards, supersede) still applies; resume MUST NOT trigger a compensating burst of cognition.

**Determinism and telemetry**

- **FR-019**: Byte-identical replay MUST be preserved: all model-derived content enters the deterministic space only as recorded events; router verdicts, guard evaluations, and staleness checks are reproducible from recorded data.
- **FR-020**: Telemetry records MUST carry causality references sufficient to reconstruct, from the event log alone, the chain stimulus event → thought → intent → resulting actions.

### Key Entities

- **Decision Class**: a registered category of model-reaching decision — its point cost, staleness budget (game time), degrade action, and guard expectations. The unit of intentional categorization.
- **Calibration Profile**: the local mapping from points to seconds for a host+model pair — baseline from the calibration stage, live estimate, sample statistics, spike counter, recalibration flag.
- **Thought Job**: one requested model interaction — snapshot tick, agent generation at snapshot, triggering event reference, prediction, and final outcome.
- **Guard**: a deterministic assumption attached to an intent or plan step — subject, expectation, validity window — evaluable against world state without model involvement.
- **Conditional Plan**: an ordered set of guarded steps returned by the model, executed deterministically; the carrier for act-at-time-T semantics.
- **Outcome Record**: the telemetry terminus of every thought job — one of the enumerated outcomes with measured staleness, predicted-vs-actual latency, and causality references.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Across a multi-game-day run at any speed, zero intents execute with measured staleness over their class budget.
- **SC-002**: 100% of thought requests terminate in exactly one recorded outcome; zero silent drops observed in a full-day audit of the event log.
- **SC-003**: Replay of a run recorded with the cognition horizon enabled is byte-identical to the original.
- **SC-004**: At 32x with a slow local model, wasted model work (calls whose results are discarded at landing) is at most 5% of calls, versus the unmeasured baseline where every landing is unchecked.
- **SC-005**: Calibration completes in under 5 minutes on the reference workload; the live estimate stays within 25% of the rolling median of observed latencies after 20 samples.
- **SC-006**: At 1x, at least 95% of decisions that route to the model today still route to the model (no low-speed regression).
- **SC-007**: For any executed action, an operator can reconstruct the full stimulus → thought → intent → action chain from the event log alone, with no auxiliary state.

## Assumptions

- The single global clock stays: throttling or per-agent time dilation is out of scope (TASK-33 owns adaptive throttling and depends on this feature's telemetry and debt definitions).
- Initial point costs and staleness budgets are hand-authored judgments recorded in the registry; telemetry informs human retuning, and no automatic adjustment of either is in scope.
- The existing deterministic reflex layer remains the permanent degraded floor for every gap; this feature narrows the gaps but never removes the floor.
- Faster-tier fallback as a degrade action is optional and only active where a faster tier is configured; the feature is complete without it.
- Conversations remain wall-clock scenes that land atomically; their registry entry treats the scene as one high-point decision with a scene-level budget.
- The existing refusal to run uncapped speed with a model configured is retained unchanged.
- Pause semantics are chosen here (world freezes, minds catch up) rather than left accidental; freezing cognition mid-flight (cancelling in-flight calls) was considered and rejected — it wastes completed work that is, by tick arithmetic, perfectly fresh.
