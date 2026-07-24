# Feature Specification: Calibration UX — uncalibrated worlds warn instead of silently over-suppressing

**Feature Branch**: `035-calibration-ux`

**Created**: 2026-07-24

**Status**: Draft

**Input**: User description: "Calibration UX: uncalibrated worlds silently over-suppress at speed — warn, auto-suggest calibrate, disclose concurrency bias (TASK-40)."

**Doctrine**: decision-4 (cognition horizon, specs/007-cognition-horizon). An LLM world with no
calibration profile runs on deliberately pessimistic bootstrap latency defaults — it fails toward
reflex, never toward stale action. On fast hardware the bootstrap can overstate real latency by
~20x, so an uncalibrated world silently suppresses whole cognition classes (conversations,
planners) at high speed with no signal beyond one boot line. Since spec 031 (estimator
breach-adoption, TASK-86) the live estimator follows real load in both directions, which makes the
calibration seed only a starting point — **but suppression starves the estimator of the very
samples that would correct it** (a suppressed thought never runs, so it never produces a latency
observation). A too-pessimistic seed at high speed therefore cannot self-correct, which is why this
feature's warnings are load-bearing, not cosmetic. This feature is **pure visibility**: no change
to routing, estimation, suppression, or the speed ladder.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Raising speed on an uncalibrated world warns loudly (Priority: P1)

An operator runs an LLM world that has never been calibrated and raises the world speed. If the
new speed pushes any cognition class into suppression under the bootstrap latency assumptions, the
speed-change reply carries a warning that names the suppressed classes and tells the operator the
exact calibrate command to run. The speed change itself still applies — speed is never capped to
protect cognition (doctrine).

**Why this priority**: this is the exact silent-failure moment from the field report (TASK-39/40):
the operator asked for 32x, got a world where villagers stopped conversing and planning, and had
no way to know why. Catching the transition into suppression is the single highest-value signal.

**Independent Test**: on a world with an LLM configured and no calibration profile, issue a
speed change to a high notch and observe the warning naming suppressed classes in the reply;
issue the same change on a calibrated world and observe no warning; confirm the speed changed
in both cases.

**Acceptance Scenarios**:

1. **Given** an uncalibrated LLM world at 1x, **When** the operator sets speed to 32x, **Then**
   the reply carries a warning naming each cognition class the bootstrap estimates suppress at
   32x (e.g. planner, conversation) and suggesting `promptworld calibrate <world>`, **and** the
   world is now running at 32x.
2. **Given** an uncalibrated LLM world at 1x, **When** the operator sets speed to a notch where
   bootstrap estimates suppress nothing (e.g. 4x), **Then** no calibration warning appears.
3. **Given** a calibrated LLM world (profile present and loaded), **When** the operator sets any
   speed, **Then** no calibration warning appears — output is unchanged from before this feature.
4. **Given** a world with no LLM configured, **When** the operator sets any speed, **Then** no
   calibration warning appears.

---

### User Story 2 - Boot warning states the concrete consequence (Priority: P2)

An operator starts the daemon for an LLM world with no calibration profile. Instead of today's
one generic line ("no calibration profile — bootstrap defaults … run `promptworld calibrate`"),
the boot output states the concrete consequence of running uncalibrated: which cognition classes
are suppressed at which speed notches under the bootstrap assumptions, and the exact calibrate
command for this world.

**Why this priority**: boot is the one moment every operator sees; naming the consequence ("at
32x, planners and conversations are suppressed until you calibrate") converts a shrug-line into
an actionable warning. Lower than US1 because boot output scrolls away while the speed change is
the moment of harm.

**Independent Test**: start a daemon on a world with an LLM config and no calibration file;
observe the boot warning with the per-class suppression summary and the calibrate suggestion.
Start it again after calibrating; observe the normal seeded line and no warning.

**Acceptance Scenarios**:

1. **Given** an LLM world with no calibration profile, **When** the daemon starts, **Then** boot
   output includes a warning that (a) says the world is uncalibrated and running on pessimistic
   defaults, (b) summarizes the cognition horizon at bootstrap values per class across the speed
   ladder, and (c) names the exact `promptworld calibrate <world>` command.
2. **Given** an LLM world with a loadable calibration profile, **When** the daemon starts,
   **Then** the boot output is the existing seeded line, with no new warning.
3. **Given** an LLM world with a malformed/unreadable calibration file (which already falls back
   to bootstrap defaults), **When** the daemon starts, **Then** the same uncalibrated warning
   appears alongside the existing unreadable-profile note.

---

### User Story 3 - Calibration state is visible in status, not just at boot (Priority: P3)

An operator (or the TUI) inspects a running world's status and can see, per provider, whether its
latency estimate was seeded from a calibration profile (and when that profile was recorded) or
from bootstrap defaults. Uncalibrated state is thereby visible for the whole life of the daemon,
not only in a boot line that scrolled away.

**Why this priority**: persistent visibility closes the loop for operators who attach to a
long-running world they didn't boot; it is also the hook the TUI needs to render an
"uncalibrated" badge. Lower priority because US1/US2 already cover the moments of action.

**Independent Test**: query status on an uncalibrated running world and see each provider marked
as bootstrap-seeded; calibrate, restart, query again and see the calibration timestamp.

**Acceptance Scenarios**:

1. **Given** a running uncalibrated LLM world, **When** the operator queries status, **Then**
   each provider's entry shows it is running on bootstrap (uncalibrated) estimates.
2. **Given** a running calibrated LLM world, **When** the operator queries status, **Then** each
   calibrated provider's entry shows the profile's calibration timestamp.
3. **Given** a profile that covers some providers but not others, **When** the operator queries
   status, **Then** covered providers show the timestamp and uncovered providers show bootstrap.
4. **Given** a world with no LLM configured, **When** the operator queries status, **Then** the
   status shape is unchanged (no LLM section, as today).

---

### User Story 4 - Calibrate discloses its sequential-measurement bias (Priority: P3)

An operator runs `promptworld calibrate` and the output discloses that the measurement is taken
sequentially (one reference call at a time) while a live world drives the same endpoint
concurrently (many agents queue on it), so the measured seconds-per-point is a **floor**: under
concurrent load the effective rate can be several times higher (field measurement: 8.1 s/pt
sequential ran ~24 s/pt effective under 8-agent load). The horizon summary carries the same
caveat so the operator doesn't read "conversation OK at 32x" as a guarantee.

**Why this priority**: closes the second finding on TASK-40 (calibration optimism under
concurrency) at the disclosure level. The runtime side is already handled by spec 031's live
adoption; measuring under representative concurrency was TASK-32 idea E, deferred by spec 007 and
still out of scope here.

**Independent Test**: run calibrate against any world and observe the disclosure line in the
output near the per-provider results and horizon summary.

**Acceptance Scenarios**:

1. **Given** any world with an LLM config, **When** the operator runs calibrate, **Then** the
   output includes a disclosure that the measured seconds-per-point is sequential and acts as a
   floor under concurrent load, and that the live estimator adapts upward at runtime.
2. **Given** a calibrate run whose horizon summary reports classes "OK" at high speed, **When**
   the operator reads the summary, **Then** the caveat is adjacent to (or part of) that summary,
   not buried elsewhere.

---

### Edge Cases

- **Partial profiles**: a calibration file that covers only some providers — per-provider
  status must be truthful (US3 scenario 3); the boot and speed warnings fire when any provider
  that cognition routes through is still on bootstrap estimates.
- **Malformed profile**: an unreadable calibration file already degrades to bootstrap defaults;
  it must now also count as "uncalibrated" for every warning surface (US2 scenario 3).
- **Repeated speed changes**: the warning is stateless and fires on every speed-change reply
  that lands in suppressing territory — an operator bouncing between 16x and 32x sees it each
  time they re-enter 32x. No warning-fatigue suppression in this feature (simplicity; revisit
  only on operator complaint).
- **Speed lowered out of suppression**: setting a speed where nothing is suppressed produces no
  warning, even on an uncalibrated world.
- **Warning must not block or fail the command**: the speed change reply is a warning-augmented
  success, never an error; clients that ignore the new field behave exactly as today.
- **Live estimator drift vs the warning's gate**: the warning *gate* is seed state (only
  bootstrap-seeded providers can trigger it) while the suppression *arithmetic* uses the
  router's actual current estimates — so a calibrated world whose live estimator drifted
  upward under load gets no warning from this feature (that signal is spec 031's adoption
  event and the governor), and an uncalibrated world whose live estimator has already
  converged out of suppression gets no false warning either. At boot the two coincide: the
  live estimate IS the bootstrap seed.
- **`max` speed on a pure-sim world**: no LLM, no warning — the existing max-gate behavior is
  untouched.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: When a daemon starts an LLM world without a usable calibration profile (absent or
  unreadable), the boot output MUST include a warning that states the world is running on
  pessimistic bootstrap estimates, summarizes which cognition classes are suppressed at which
  speed-ladder notches under those estimates, and names the exact `promptworld calibrate <world>`
  command.
- **FR-002**: When a speed change on an uncalibrated LLM world lands on a speed at which the
  bootstrap estimates suppress one or more cognition classes, the speed-change reply MUST carry a
  warning naming the suppressed classes and suggesting the calibrate command. The speed change
  MUST still apply — the warning never blocks, alters, or fails the command.
- **FR-003**: Speed-change replies on calibrated worlds, on uncalibrated worlds at
  non-suppressing speeds, and on worlds without an LLM MUST be unchanged — zero new warnings.
- **FR-004**: The status surface MUST expose, per provider, whether its latency estimate was
  seeded from a calibration profile or from bootstrap defaults, and the profile's calibration
  timestamp when seeded. Worlds without an LLM keep today's status shape exactly.
- **FR-005**: The calibrate command's output MUST disclose that its measurement is sequential
  and therefore a floor under concurrent live load, adjacent to the per-provider results /
  horizon summary, and note that the live estimator adapts at runtime.
- **FR-006**: The suppression arithmetic behind FR-001/FR-002 MUST be the same deterministic
  routing arithmetic the live system uses (same class registry, same budgets, same speed
  ladder, same estimate inputs) — the speed-change warning may never disagree with what the
  router would actually do at the moment of the warning; the boot warning evaluates at the
  bootstrap seeds, the only estimates that exist at boot.
- **FR-007**: This feature MUST NOT change routing decisions, estimator behavior, suppression
  behavior, bootstrap default values, or the speed ladder. All additions are observational.
- **FR-008**: New status/reply fields MUST be additive and omitted when empty: a no-LLM
  world's outputs are byte-identical everywhere, and a calibrated world's boot lines and
  speed-change replies are byte-identical — its status gains only the additive per-provider
  calibration fields of FR-004.

### Key Entities

- **Calibration seed state**: per provider — `calibrated` (with the profile's timestamp) or
  `bootstrap`. Determined once at daemon start from the profile-load outcome; never mutated by
  live estimator adaptation (spec 031 adoption changes the live estimate, not the seed state).
- **Suppression summary**: the per-class, per-speed-notch verdict of the routing arithmetic —
  the shared content of the boot warning (FR-001, evaluated at the bootstrap seeds) and the
  speed-change warning (FR-002, evaluated at the router's current estimates), already computed
  today by the calibrate command's horizon summary.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator who starts an uncalibrated LLM world and raises it to a suppressing
  speed encounters the warning at least twice (boot and speed change) without consulting any
  documentation, and each warning names both the consequence (which classes are suppressed) and
  the remedy (the exact calibrate command).
- **SC-002**: A calibrated world produces zero new warnings across boot and speed changes
  (byte-identical where previously recorded); its status gains only the additive per-provider
  calibration timestamp. A no-LLM world's outputs are byte-identical everywhere.
- **SC-003**: Routing behavior is unchanged: for any fixed estimate and speed, the set of
  admitted/suppressed thoughts before and after this feature is identical.
- **SC-004**: An operator attaching to a long-running world can determine from status alone,
  in one query, whether any provider is running on bootstrap estimates.
- **SC-005**: Every calibrate run's output includes the sequential-floor disclosure; an operator
  reading the horizon summary sees the caveat without scrolling elsewhere.

## Doctrine Review — bootstrap default stands (closes TASK-40's open question)

TASK-40 asked whether `BootstrapLocalSecPerPt = 20.0` should be lowered, since it overstates a
fast rig's real latency ~20x and drives the silent over-suppression. Reviewed against
specs/007-cognition-horizon (decision-4):

- The pessimistic default is doctrine, not accident: under bootstrap, the system fails toward
  reflex (suppressed cognition, world keeps moving) and never toward stale action (FR-006 and
  the "no calibration profile" edge case in spec 007). Lowering the default trades silent
  over-suppression for silent stale action — a strictly worse failure mode, because stale
  actions corrupt the world while suppression only impoverishes it.
- Spec 031 (estimator breach-adoption) made the live estimator follow real load in both
  directions, so a calibrated-or-sampled world converges away from any seed. The residual harm
  of the pessimistic seed is exactly the visibility gap this feature closes.

**Decision**: the bootstrap defaults are unchanged. The remedy for over-suppression is
calibration plus the warnings specified here, not a more optimistic default. This closes the
"revisit bootstrap default" question on TASK-40 by artifact.

## Assumptions

- "Uncalibrated" is a per-provider fact (profile entry present and loaded vs bootstrap seed),
  and the world-level warnings (FR-001/FR-002) fire when at least one provider is
  bootstrap-seeded; the status surface (FR-004) carries the per-provider truth.
- The warning *gate* is seed state (bootstrap-seeded providers only); the suppression
  *arithmetic* is the router's own, with its current estimate inputs. Calibration UX reports
  the consequence of *not calibrating*, not of runtime drift, which has its own signals
  (spec 031 adoption event, spec 028/033 governor).
- The speed-change warning rides the existing speed-change reply as an additive field; how each
  client (CLI, TUI) renders it is that client's concern, but the daemon-side field plus CLI
  rendering are in scope here. A TUI badge for uncalibrated state is enabled by FR-004 but its
  visual design is out of scope (TASK-34's TUI follow-up).
- Measuring calibration under representative concurrency (TASK-32 idea E) remains deferred, as
  spec 007 decided; this feature discloses the bias instead (FR-005).
- Warnings are stateless per command — no rate limiting, no per-session memory of "already
  warned" (simplicity first; revisit on operator feedback).
