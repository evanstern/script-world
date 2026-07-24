# Feature Specification: Adaptive Time Throttling

**Feature Branch**: `task-33-adaptive-throttle`

**Created**: 2026-07-23

**Status**: Draft

**Input**: User description: "Adaptive time throttling: speed as a ceiling, staleness debt as the governor. The player's speed
setting becomes a CEILING, not a promise: the sim continuously computes aggregate staleness debt — the global sum over all
in-flight and queued planner/conversation jobs of predicted game-tick drift — and when debt exceeds a budget, the loop sheds one
notch on the existing six-value speed ladder (32x→16x→8x→4x→1x, floor 1x), recovering notch-by-notch as debt drains. Asymmetric
hysteresis so speed never oscillates. Sheds and recoveries are recorded clock.* events; the player's requested speed is preserved
and the effective (governed) speed is what the loop paces against. Determinism boundary: governing is wall-side pacing exactly
like pause — tick CONTENT never changes, replay stays byte-identical. SpeedMax remains refused when an LLM is configured. The
router evaluates against the EFFECTIVE speed, so shedding speed widens what the LLM may own. TUI/status must communicate governed
state. Debt scope: global sum (salience weighting deferred). Depends on spec 007 (TASK-32, shipped)."

**Doctrine**: extends decision-4 (cognition horizon). Spec 007 scopes what the model may decide at a given speed; this feature
closes the loop from the other side — when the player wants both high speed and high thought fidelity (a crisis unfolding at
32x), the simulation governs itself instead of forcing a manual speed drop. The speed setting becomes a **ceiling, not a
promise**: a feedback controller over a measurable signal (aggregate in-flight staleness debt) sheds speed one notch at a time
and climbs back as the debt drains — RimWorld-style adaptive time, driven by cognition load instead of frame rate. Governing is
wall-side pacing, exactly like pause: it changes when ticks happen, never what they contain.

**Session decisions** (recorded on TASK-33, 2026-07-23): shed policy is notch-down on the existing six-value speed ladder
(proportional pacing and automatic micro-pause rejected); debt scope is a global sum over all pending model-bound thoughts
(salience weighting deferred); the SpeedMax-refused-with-LLM rule is retained unchanged.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Debt is measured and visible (Priority: P1)

Before the world governs anything, the operator can see the signal it would govern on. The running world continuously derives
one number — aggregate staleness debt — from the thoughts currently in flight or queued: each pending model-bound thought
contributes the game-time drift it is predicted to still accumulate before landing, expressed as a fraction of its decision
class's staleness budget, and the world sums those fractions. Status output and the TUI show the debt, the number of
contributing thoughts, and the requested speed, at all times.

**Why this priority**: telemetry-first is standing doctrine (spec 007 US1): every later behavior (shed, recover, tuning of
thresholds) acts on this measurement, and shipping it alone already answers "how close is my world to needing a governor?" on
the operator's real hardware, with zero behavior change.

**Independent Test**: run a world with a slow model at high speed, trigger several planner thoughts and a conversation, and
watch the debt figure in status rise while jobs are in flight and drain as they land — with the simulation's behavior otherwise
unchanged.

**Acceptance Scenarios**:

1. **Given** a running world with an LLM configured, **When** thoughts are in flight or queued, **Then** status reports a debt
   value equal to the sum over those thoughts of predicted remaining drift as a fraction of each thought's class budget, plus
   the count of contributing thoughts.
2. **Given** a world with no model-bound work pending, **When** status is read, **Then** debt is exactly zero.
3. **Given** the same set of pending thoughts, the same latency estimates, and the same speed, **When** debt is derived twice,
   **Then** the value is identical — the derivation is pure arithmetic with no model consulted and no randomness.
4. **Given** a world with no LLM configured, **When** the world runs at any speed, **Then** the debt signal is absent/inert and
   no governor machinery observes anything.

---

### User Story 2 - The world sheds speed under debt (Priority: P2)

A crisis breaks out at 32x: three villagers' minds go into flight at once. Instead of letting their thoughts land half a game
hour stale — or forcing the player to notice and drop speed manually — the world sheds one notch on the speed ladder
(32x→16x) when aggregate debt stays over the shed threshold through a short breach window, and sheds again if debt stays over.
The player's requested speed is untouched; the loop simply paces at the governed notch. Every shed is a recorded event carrying
the arithmetic that justified it. Because the cognition router (spec 007) evaluates at the effective speed, shedding widens
what the model may own — thoughts that would have been suppressed or landed dead at 32x now land sane at 16x.

**Why this priority**: this is the feature's reason to exist — speed and thought fidelity stop being a manual trade-off. It
depends only on US1's signal.

**Independent Test**: with a calibrated slow model, script a burst of concurrent thoughts at 32x and verify the world drops to
16x (and further under sustained load), the shed events record the debt arithmetic, and the router's verdicts at the governed
speed admit classes it refused at 32x.

**Acceptance Scenarios**:

1. **Given** debt above the shed threshold sustained through the breach window, **When** the governor evaluates, **Then**
   effective speed drops exactly one notch on the ladder, and a recorded event captures: requested speed, prior and new
   effective speed, measured debt, and the number of contributing thoughts.
2. **Given** debt still above threshold after a further breach window at the shed notch, **When** the governor evaluates,
   **Then** it sheds one further notch, down to the floor of 1x at most — never below, and never to uncapped.
3. **Given** a governed world at 16x with requested 32x, **When** the cognition router evaluates a decision class, **Then** the
   prediction uses the effective speed (16x), so classes admissible at 16x route to the model even though 32x was requested.
4. **Given** effective speed at the 1x floor with debt still over threshold, **When** the governor evaluates, **Then** no
   further shed occurs and the saturated state is visible in status; per-class routing (spec 007) remains the only remaining
   defense, unchanged.
5. **Given** a world with no LLM configured, **When** it runs at any requested speed for any duration, **Then** zero governor
   events are emitted and effective speed always equals requested speed.

---

### User Story 3 - Speed recovers without oscillating (Priority: P3)

The crisis passes; the in-flight thoughts land and debt drains. The world climbs back toward the requested speed one notch at a
time — but only when the debt it would carry *at the restored speed* would still sit safely under the shed threshold, and only
after the drained state has held through a recovery window longer than the breach window. Climbing a notch multiplies every
pending thought's predicted drift, so recovery that looked at current debt alone would immediately re-shed; the governor
projects before it climbs. The result: no flapping between notches, and requested speed is restored promptly once the load is
genuinely gone.

**Why this priority**: without recovery the governor is a one-way ratchet and the ceiling is meaningless; without hysteresis the
speed oscillates and is worse than a manual drop. Depends on US2.

**Independent Test**: drive debt over threshold, let the scripted load end, and verify the world returns to requested speed
notch-by-notch with no shed→recover→shed flapping; hold a steady marginal load and verify effective speed parks at one stable
notch.

**Acceptance Scenarios**:

1. **Given** a governed world whose debt, projected at the next notch up, would remain below the shed threshold by the required
   margin, **When** that condition holds through the full recovery window, **Then** effective speed rises exactly one notch and
   a recorded event captures the recovery and its arithmetic.
2. **Given** a marginal steady load where debt at the current notch is under threshold but projected debt one notch up breaches
   it, **When** wall time passes, **Then** effective speed stays parked at the current notch indefinitely — no oscillation.
3. **Given** the governor recovered to the requested speed, **When** debt stays low, **Then** the governor is quiescent:
   effective equals requested and no further events are emitted.
4. **Given** any alternation of load bursts and quiet, **When** the run is audited, **Then** the recovery window is observably
   longer than the breach window (asymmetric hysteresis), and no shed event follows a recovery event within one recovery
   window unless debt actually re-breached.

---

### User Story 4 - The player sees it and stays in charge (Priority: P4)

The player asked for 32x and the world is running 16x. The TUI says so, plainly: the speed indicator shows the effective speed
and, whenever it differs from the requested speed, why — "asked 32x, running 16x — 3 minds in flight, debt 140% of budget". A
player speed change always takes effect immediately: setting a lower speed than the governed notch simply runs there (and
clears the governed state); setting a higher one raises the ceiling, and the governor re-evaluates on its normal cadence. Pause
is untouched: pausing stops the clock and the governor with it, in-flight minds catch up at the frozen tick per spec 007's
pause doctrine, and a resume starts the governor's windows fresh.

**Why this priority**: a governor that silently overrides the player reads as a broken speed control; the communication layer is
what makes the ceiling semantics legible. Depends on US2/US3 existing to have something to show.

**Independent Test**: govern a world down under scripted load and verify the TUI shows requested vs effective with the reason;
issue player speed changes below, at, and above the governed notch and verify each takes effect as specified; pause and resume
mid-governed and verify the governor suspends and restarts windows fresh.

**Acceptance Scenarios**:

1. **Given** effective speed differs from requested, **When** the player looks at the TUI or reads status, **Then** both speeds
   are visible along with the count of in-flight thoughts and the debt level driving the difference.
2. **Given** a world governed to 8x with requested 32x, **When** the player sets speed 4x, **Then** the world runs at 4x
   immediately, requested becomes 4x, and the governed state clears (effective equals requested).
3. **Given** a governed world, **When** the player sets a higher requested speed, **Then** the new ceiling is recorded
   immediately and the governor re-evaluates on its normal cadence — the world never runs above the new request nor above what
   debt allows.
4. **Given** a governed world, **When** the player pauses, **Then** no governor evaluation or events occur while paused;
   in-flight thoughts land at the frozen tick (spec 007 FR-018); **and When** the player resumes, **Then** breach/recovery
   windows restart fresh — a pause never converts into an instant shed or recovery.
5. **Given** an LLM is configured, **When** the player requests uncapped speed (max), **Then** it is refused exactly as today —
   the governor governs only the capped ladder and does not make max speed meaningful.

---

### Edge Cases

- **No calibration profile**: debt uses the same live estimates and pessimistic bootstrap defaults as spec 007 routing; an
  uncalibrated world overestimates drift and sheds conservatively — it fails toward fidelity, never toward stale action.
- **A thought outlives its prediction** (elapsed wall time exceeds predicted): it counts its full accrued elapsed drift — the
  measured minimum staleness its reply will land with — and that contribution grows the longer it languishes (revised by
  spec 033; the original definition floored the overrun to zero, which inverted the signal — the worse the drowning, the
  sooner every in-flight thought went overdue and vanished from the sum, so the governor never shed exactly when it should).
  An overrun is a measurement, not an invention: an overdue thought's elapsed time IS its grounded debt. See
  specs/033-governor-accrued-debt/contracts/debt-formula.md for the piecewise arm and the deliberate boundary jump at
  elapsed == predicted.
- **Speed changes while thoughts are in flight**: debt is re-derived each evaluation at the current effective speed, so a
  player drop instantly shrinks debt and a climb instantly grows it; landing enforcement stays exact regardless (staleness is
  measured in actually-elapsed ticks, spec 007).
- **Debt breaches at the 1x floor**: the governor saturates visibly (status shows floor + over-budget debt); it never pauses
  the world on its own — automatic micro-pause was considered and rejected in the design session.
- **Estimator lag spike mid-window**: a one-shot spike inflates one thought's prediction only as far as the spike-robust
  estimator lets it (spec 007 FR-005); the breach window means a single evaluation blip cannot shed speed by itself.
- **Governor events racing a player speed change**: player commands and governor decisions serialize through the same single
  ordered command path as every other input; whichever lands first is simply recorded first — there is no merge ambiguity.
- **World replayed on different hardware**: replay applies the recorded shed/recover events; it never re-derives debt. A
  slower or faster replay host produces byte-identical state because governing was always wall-side pacing plus recorded
  inputs.

## Requirements *(mandatory)*

### Functional Requirements

**Debt — the measurable signal**

- **FR-001**: The system MUST continuously derive an aggregate staleness debt: for every model-bound thought currently in
  flight or queued, its staleness wall time — **piecewise**: the predicted remaining wall time (prediction minus elapsed)
  while the thought is within its prediction, and its full accrued elapsed drift once it has overrun (elapsed ≥ predicted) —
  converted to game ticks at the current effective speed, divided by its decision class's staleness budget; debt is the global
  sum of these dimensionless budget-fractions. An overdue thought counts its accrued, growing drift, not zero (revised by
  spec 033 — see specs/033-governor-accrued-debt/contracts/debt-formula.md; the original floored-to-zero definition inverted
  the signal under overload). Salience weighting is explicitly out of scope (recorded session decision).
- **FR-002**: Debt derivation MUST be pure arithmetic over the decision-class registry, the live latency estimates, the
  pending-thought set, and the effective speed — no model consulted, no randomness. Identical inputs MUST yield identical
  debt.
- **FR-003**: Debt, the count of contributing thoughts, the requested speed, and the effective speed MUST be readable in
  status output at all times. With no LLM configured the governor MUST be fully inert: zero debt machinery observed, zero
  events, effective always equal to requested.

**Governor — ceiling and ladder**

- **FR-004**: The player's speed setting MUST become a ceiling: the world MUST never pace faster than the requested speed, and
  the governor MUST only move the effective speed along the existing capped speed ladder, with 1x as the hard floor. The
  governor MUST never pause the world and MUST never touch uncapped speed.
- **FR-005**: When debt exceeds the shed threshold continuously through the breach window, the effective speed MUST drop
  exactly one notch; sustained breach MUST shed further notches one window at a time.
- **FR-006**: The governor MUST recover one notch only when debt *projected at the candidate notch* (current debt scaled by
  the notch's tick-rate ratio) would remain below the shed threshold by a defined margin, sustained through the recovery
  window. The recovery window MUST be longer than the breach window (asymmetric hysteresis).
- **FR-007**: Shed threshold, recovery margin, breach window, and recovery window MUST be doctrine constants versioned with
  the code — tuned by humans from telemetry, never runtime knobs and never self-adjusting (same doctrine as registry points
  and budgets, spec 007).
- **FR-008**: Every shed and every recovery MUST land as a recorded clock event carrying: requested speed, prior and new
  effective speed, measured debt, and the contributing-thought count. No governor speed change may ever be silent (auto-slow
  degraded/recovered precedent).
- **FR-009**: A player speed command MUST always set the requested speed and take effect immediately: a request at or below
  the current effective speed runs there at once and clears governed state; a request above it raises the ceiling and leaves
  the governor to re-evaluate on its normal cadence.

**Composition with the existing substrate**

- **FR-010**: The cognition router (spec 007 FR-007) MUST evaluate predictions against the effective speed, so shedding speed
  deterministically widens what the model may own and recovery narrows it again.
- **FR-011**: The existing honest-slowdown observer (achieved-versus-requested tick rate) MUST measure against the effective
  speed, so a governed world is not falsely reported as degraded.
- **FR-012**: The refusal of uncapped speed while an LLM is configured MUST be retained unchanged (spec 007 assumption; recorded
  session decision).
- **FR-013**: While paused, the governor MUST NOT evaluate or emit events; in-flight thoughts complete under pause doctrine
  (spec 007 FR-018) and naturally drain debt. On resume, breach and recovery windows MUST restart fresh — elapsed pause time
  MUST never count toward either window.

**Determinism**

- **FR-014**: Governing MUST be wall-side: tick content MUST never depend on debt, thresholds, or wall-clock observations.
  Governor decisions enter the deterministic space only as recorded events applied at tick boundaries, and replay of a
  governed run MUST be byte-identical — replay re-applies recorded governor events and never re-derives debt.

**Player communication**

- **FR-015**: The TUI MUST show the effective speed as the world's speed; whenever effective differs from requested it MUST
  additionally communicate the requested speed and the cause in plain language (in-flight thought count and debt relative to
  budget) — e.g. "asked 32x, running 16x — 3 minds in flight, debt 140%".

### Key Entities

- **Staleness Debt**: the world-level feedback signal — a dimensionless sum, over pending model-bound thoughts, of predicted
  remaining game-tick drift as a fraction of each thought's class staleness budget. Derived, never stored; zero when nothing
  is pending.
- **Governor State**: requested speed (the player's ceiling), effective speed (what the loop paces at), and the running
  breach/recovery window accumulators. Requested and effective speeds are world state; window accumulators are wall-side
  observer state, never persisted.
- **Governor Event**: the recorded shed/recover decision — requested speed, prior and new effective speed, measured debt,
  contributing-thought count. The sole channel by which governing enters the deterministic space.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Replay of any governed run is byte-identical to the original, including runs containing sheds, recoveries,
  player overrides, and pauses mid-governed.
- **SC-002**: In a scripted crisis scenario (concurrent thought burst at requested 32x with a slow model), the fraction of
  model results discarded at landing as stale is reduced by at least half versus the same scenario with the governor disabled,
  with zero manual speed input.
- **SC-003**: Under a steady synthetic load held near the threshold for many windows, the effective speed changes at most once
  per recovery window — no shed↔recover flapping.
- **SC-004**: A world with no LLM configured shows zero governor events and zero observable overhead across a full game day at
  any speed.
- **SC-005**: For any governed interval, an operator can reconstruct from the event log alone: when speed shed, when it
  recovered, what the debt was, and how many thoughts drove it.
- **SC-006**: The effective speed never exceeds the requested speed at any point in any run, and a player speed command is
  reflected in pacing within one evaluation cadence.

## Assumptions

- Spec 007 (cognition horizon) is shipped and is the substrate: decision-class registry with points and game-tick budgets,
  per-provider seconds-per-point estimation with spike rejection, deterministic routing, landing-side staleness enforcement,
  and pause doctrine. This feature adds a feedback controller over those existing signals and adds no new measurement
  primitives.
- Debt is a global sum (session decision); per-agent or salience-weighted debt is deferred until telemetry from this feature
  shows the need.
- Shedding moves along the existing six-value speed ladder (session decision); proportional/continuous pacing and automatic
  micro-pause were considered and rejected. Micro-pause may return as a future opt-in feature.
- The single global clock stands: per-agent time dilation remains out of scope (spec 007 assumption, unchanged).
- Governor constants (threshold, margin, windows) are initial hand-authored judgments recorded in code; this feature's own
  telemetry (recorded governor events) is the input for human retuning.
- The honest-slowdown observer (auto-slow `clock.degraded`/`clock.recovered`) remains: it reports the host failing to keep the
  effective pace, while the governor chooses the effective pace — two different facts, both recorded.
- Conversations remain wall-clock scenes (spec 007 assumption); their in-flight predictions contribute to debt like any other
  pending thought, scaled by their scene-level class budget.
