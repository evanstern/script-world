# Phase 0 Research: Adaptive Time Throttling

All unknowns from Technical Context resolved below. Grounding: docs/wiki (cognition, sim-loop, game-clock,
llm-orchestrator — all pinned), spec 007 artifacts, and direct source verification at `059b95c`.

## R1 — Where the governor lives

**Decision**: an external controller owned by the daemon, issuing decisions into the sim loop through the same
command door as every other input (a new `govern` command → recorded `clock.governor_*` events applied at a tick
boundary). The controller itself is a pure state machine in `internal/cognition` (`governor.go`); the daemon owns
the goroutine that samples debt on a wall-clock cadence and calls `Loop.Do`-style entry.

**Rationale**: `internal/sim` must stay free of `internal/llm` imports (the debt inputs live in the orchestrator).
The player, Metatron, and the minds already enter exclusively through the command channel as recorded input; the
governor is just another outside will. The pure controller is unit-testable with zero goroutines.

**Alternatives considered**: (a) in-loop evaluation via an injected `debtFn` callback — rejected: pushes wall-side
LLM-coupled sampling into the loop's select and complicates pause handling for no gain; (b) governor inside the
orchestrator — rejected: the orchestrator has no business commanding the clock; composition stays file/door-shaped.

## R2 — State representation: requested vs effective speed

**Decision**: `State.Speed` KEEPS its meaning as "the speed the loop paces at" and becomes the *effective* speed.
A new additive field `State.RequestedSpeed clock.Speed` with `json:"requested_speed,omitempty"` carries the
player's ceiling **only while governed**; empty means ungoverned (requested == effective).

**Rationale**: verified consumers of `State.Speed` — the mind's router (`internal/mind/telemetry.go:50,66`,
`replica.Speed.TicksPerSecond()`) and the auto-slow observer (`internal/sim/loop.go` `observeWindow`, compares
measured vs `l.state.Speed.TicksPerSecond()`) — both automatically satisfy FR-010 and FR-011 with zero changes if
`Speed` is the effective speed. `omitempty` keeps every pre-028 snapshot and canonical-bytes hash byte-identical.

**Alternatives considered**: `Speed` = requested + new `GovernedSpeed` — rejected: every existing consumer would
need auditing and editing; the two verified hot consumers want effective, not requested.

## R3 — Event vocabulary and reducer semantics

**Decision**: two new reducer-applied event types:

- `clock.governor_shed` — payload `{requested, from, to, debt, jobs}`
- `clock.governor_recovered` — same payload shape

Reducer: `Speed = to`; `RequestedSpeed = requested` when `to != requested`, cleared (empty) when `to == requested`;
`EffectiveRate` maintained exactly as `clock.speed_set` does (`= to.TicksPerSecond()` unless `Degraded`).
`clock.speed_set` (player) additionally clears `RequestedSpeed` — a player command always collapses governed state
(FR-009). `clock.paused`/`clock.resumed`/`clock.degraded`/`clock.recovered` are untouched.

**Rationale**: mirrors the auto-slow precedent (`clock.degraded`/`clock.recovered` — recorded, reducer-applied,
payload carries the measurement). Payload carries the full arithmetic (FR-008) so the event log alone reconstructs
every governed interval (SC-005).

**Format door**: **no format bump.** The state field is additive-`omitempty` (old snapshots unaffected, old worlds
never contain the new event types, replay of every existing shape is unchanged), which is exactly the boundary
spec 013 drew for when a bump IS required. New events appear only in logs written by this feature.

## R4 — The pending-thought inventory (debt inputs)

**Decision**: the orchestrator grows a per-job registry — an entry per accepted `Submit` from acceptance to reply,
tracking kind, serving/queued provider, enqueue time, and dispatch time — exposed as
`Orchestrator.PendingCognition() []PendingThought{Kind, Provider, PredictedSec, ElapsedSec}` where `PredictedSec` =
class points × the provider's **current** live seconds-per-point estimate, computed at read time.

**Rationale**: queued channel entries cannot be introspected, so acceptance-scoped bookkeeping is required anyway;
computing predictions at read time (rather than freezing them at submit) means debt always reflects the freshest
estimator state, including spike-rejected drift (edge case: estimator lag spike). `internal/llm` already imports
`internal/cognition` (estimators), so points lookup is free.

**Details**: queued jobs (not yet dispatched) count their full prediction; in-flight jobs are **piecewise** (revised by
spec 033 — see specs/033-governor-accrued-debt/contracts/debt-formula.md): while within prediction they count the draining
remaining work `PredictedSec − ElapsedSec`, and once overrun (`ElapsedSec ≥ PredictedSec`) they count their full accrued
`ElapsedSec` drift rather than zero. The original floored `max(0, PredictedSec − ElapsedSec)` inverted the signal under
overload (overdue thoughts vanished from the sum exactly when the system was drowning); the accrued arm grounds an overrun in
the measured staleness its reply will land with. Every model-bound kind participates (planner, conversation, meeting,
consolidation, chronicle, metatron): long-budget classes contribute proportionally tiny fractions by construction, so no
kind-filtering is needed.

## R5 — Debt arithmetic

**Decision**: per pending thought, `fraction = (secondsSec × ticksPerSecond) / BudgetTicks`; debt = Σ fractions
(dimensionless budget-fractions, per the spec's FR-001), where `secondsSec` is the piecewise staleness time — draining
remaining work within prediction, full accrued drift once overrun (revised by spec 033,
specs/033-governor-accrued-debt/contracts/debt-formula.md). Helper lives in `internal/cognition` beside `Route` — the same
pure-arithmetic doctrine (FR-002): no wall-clock reads inside the helper; the sampler passes elapsed values in.

## R6 — Controller constants and hysteresis math

**Decision** (doctrine constants in `internal/cognition/governor.go`, human-tuned from this feature's own
telemetry, never runtime knobs — FR-007):

| Constant | Initial value | Meaning |
|---|---|---|
| `GovernorCadence` | 1 s | wall-clock sampling interval |
| `ShedThreshold` | 1.0 | debt (budget-fractions) above which a breach window accrues |
| `BreachWindow` | 5 s | continuous breach required to shed one notch |
| `RecoverHeadroom` | 0.5 | projected debt at the candidate notch must be < `ShedThreshold × RecoverHeadroom` |
| `RecoveryWindow` | 20 s | continuous headroom required to recover one notch |

Recovery projection (FR-006): `projected = debt × (candidateTPS / currentTPS)` — current debt rescaled to the
candidate notch's tick rate, which is exact because every fraction is linear in ticks-per-second. Windows reset on:
any shed/recover decision, any player speed change, any pause (and both stay reset until resume — FR-013), and on
governor start. `BreachWindow` (5 s) matches the auto-slow `degradeWindow` precedent; `RecoveryWindow` is 4× longer
(asymmetric hysteresis, FR-006/US3-AC4).

**Ladder**: the capped ladder `1x, 4x, 8x, 16x, 32x` from `internal/clock` (the six values minus `max`). Floor 1x;
the governor never touches `max` (FR-004, FR-012 — the `ipc-server` refusal stands unchanged).

## R7 — Pause and no-LLM inertness

**Decision**: the daemon's sampler reads the loop status each cadence; while `Paused` it neither accumulates
windows nor issues decisions, and clears both windows so resume starts fresh (FR-013 — in-flight thoughts drain
debt on their own during the pause). When no LLM is configured the daemon simply never constructs the governor
(same pattern as the orchestrator itself): zero machinery, zero events, `RequestedSpeed` never set (FR-003,
SC-004).

## R8 — Status and TUI surface

**Decision**: protocol `Status` gains `RequestedSpeed string` (empty when ungoverned, from state) plus
`GovernorDebt float64` and `GovernorJobs int` (from the daemon's governor snapshot, folded in by `ipc-server`
exactly as the LLM `StatusSnapshot` is). The TUI header line (`internal/tui/views.go:115`) shows the effective
speed as today and appends, when governed: `asked 32x — 3 minds in flight, debt 140%`. The digest view
(`internal/tui/digest.go`) renders the two new event types like `clock.degraded`.

## R9 — Testing strategy

Unit: debt helper + controller state machine (pure, table-driven: shed, multi-notch, floor saturation, projection
parking at a marginal notch, window resets, asymmetry). `internal/llm`: registry add/remove under `-race`,
PendingCognition snapshot correctness with queued + in-flight + completed jobs. `internal/sim`: reducer arms for
both events (Speed/RequestedSpeed/EffectiveRate transitions), `govern` command emission + idempotence, player
`set_speed` clearing governed state. Replay: a log containing sheds, recoveries, player overrides, and a
mid-governed pause replays byte-identical (SC-001). Status/TUI: snapshot-render tests for the governed header.
SC-002 (halved stale-discard rate in a scripted crisis) validates via a deterministic scripted-latency harness in
the quickstart, mirroring the 012/T045 live-observation precedent if impractical in CI.
