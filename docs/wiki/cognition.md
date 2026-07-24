---
name: cognition
description: The cognition horizon substrate — decision-class registry (Fibonacci points, game-tick staleness budgets), seconds-per-point estimation with spike rejection, deterministic LLM-vs-reflex routing, the calibration profile, and the adaptive-throttle debt/governor feedback controller
kind: component
sources:
  - internal/cognition/doc.go
  - internal/cognition/registry.go
  - internal/cognition/estimate.go
  - internal/cognition/route.go
  - internal/cognition/calibration.go
  - internal/cognition/governor.go
  - internal/sim/cognition.go
verified_against: be38288fa137064174eedbfb3b8a94cc5b1fb0b9
---

# Cognition horizon

`internal/cognition` (TASK-32, decision-4, specs/007-cognition-horizon) is the
deterministic substrate that scopes LLM authority by decision timescale versus
turn latency in game time: a model turn takes real seconds while the world
keeps ticking, and the drift scales with speed. Rather than capping speed to
protect cognition, the package decides — with no model in the loop — which
decisions may go to the model at the current speed, and what deterministic
floor covers when they may not. It is stdlib-only and imports nothing from
`internal/mind`, `internal/sim`, or `internal/llm`; all three depend on it,
never the reverse.

## How it works

**Registry** (`registry.go`): each model-reaching decision class is a
`DecisionClass{Class, Points, BudgetTicks, Degrade, FutureDated}`. `Points` is
the thought cost in Fibonacci points (the closed set 1/2/3/5/8/13 — ordinal,
host-independent, a property of the prompt shape); `BudgetTicks` is the
staleness budget in game ticks (a property of the fiction). Six classes are
registered (`planner` 3pt/1200t degrading to reflex, `conversation`
13pt/7200t, `meeting` 2pt/3600t degrading to a template, `consolidation`
5pt/28800t, `chronicle` 5pt/86400t, `metatron` 5pt/86400t); values are
doctrine — changing one is a reviewed code change, never runtime tuning.
The `musing` class retired with spec 017: musing is no longer a scheduled
call kind gated by its own router entry — it is a roster tool inside the
planner's tool-use loop, so it now shares the `planner` class's 3pt/1200t
horizon gate rather than carrying its own 1pt/3600t budget ([[agent-mind]],
[[tool-loop]]). Spec 029 adds no new class: the angel's fuzzy-order confirm
kind (`metatron_watch`, [[metatron-orders]]) maps to the EXISTING `metatron`
class (5pt/86400t) — same actor, event-triggered not cadence-scheduled, a
one-line `kindToClass` entry on the narrator/drama→`chronicle` precedent, so the
spec-007 registry doctrine contract is untouched. `kindToClass` maps every LLM
call kind (as a string, keeping the package leaf) to a class; `ValidateKinds` enforces FR-002 at daemon start
— an unmapped kind, a non-Fibonacci point value, or a non-positive budget is
a fatal startup error. `Degrade` names the suppression floor: `skip`
(recorded, not silent), `reflex`, or `template` (a `faster-tier` variant
existed but was never wired past skip and was removed as dead code, TASK-71).

**Routing** (`route.go`): `Route(dc, ticksPerSecond, secondsPerPoint)` is pure
arithmetic — predicted wall seconds = points × seconds-per-point; predicted
drift ticks = wall seconds × ticks-per-second; allow iff drift ≤ budget. No
model, no randomness, no wall-clock reads (FR-007), so identical inputs always
yield the identical `Verdict`. The verdict carries the arithmetic verbatim as
a string (e.g. `3pt x 17.0s/pt x 32x = 1632 ticks > budget 1200`) so every
suppression is auditable in telemetry. `ticksPerSecond <= 0` (uncapped max
speed) always suppresses — prediction at unbounded speed is meaningless.

**Estimation** (`estimate.go`): `Estimator` holds one provider's live
seconds-per-point as an EWMA (`EWMAAlpha` 0.2) over per-point-normalized call
durations. A sample beyond `SpikeFactor` (3.0) times the current estimate is
excluded from the EWMA but retained — with its value — in a `WindowSize`-20
ring of `{secPerPoint, spike}` slots; on the sample that first drives the
rolling spike rate over a full window past `BreachRate` (0.3), the estimator
ADOPTS (spec 031, breach-adoption): it re-seeds `estimate` to the window
median (all retained values, spike and non-spike alike), zeroes the ring, and
`Sample` returns a non-nil `Adoption{Prior, Adopted, SpikeRate}` — the
`cog.recalibration_recommended` episode, which now has an actor instead of
being an unread signal. Re-arm is structural: a fresh window must refill
before any further verdict, and post-adoption samples in the new regime are no
longer spikes against the adopted estimate. One-shot lag spikes (too few to
breach) are thus still rejected while systemic drift — including a step change
larger than `SpikeFactor`, which pre-031 froze the estimate forever — is
followed within one window. The estimator is process-lifetime only.

**Calibration** (`calibration.go`): `Profile` is `calibration.json` in the
world save directory (`World.CalibrationPath()`), written only by the
`promptworld calibrate` subcommand (full-file replace via `Save`) — the daemon
never writes it, so the recorded baseline moves only under a human's hand.
`LoadProfile` treats a missing file as legal (nil, nil) and a malformed one as
a warning the daemon downgrades to bootstrap defaults.
`SeedFor(p, name, zeroPriced)` returns a provider's recorded seconds-per-point
— the profile is keyed by PROVIDER NAME since spec 024; legacy worlds derive
providers named `local`/`cloud`, so pre-024 tier-keyed profiles keep matching
with no translation — or, on a miss, a bootstrap by pricing class: zero-priced
providers seed `BootstrapLocalSecPerPt` (20.0), priced ones
`BootstrapCloudSecPerPt` (10.0) — an
uncalibrated world fails toward reflex, never toward stale action.
`TierProfile.SecondsPerPoint`'s unit is doctrine, spelled out since spec 017:
for a single-shot kind it is one model call's wall time per point; for a loop
cognition (the villager planner) it is the WHOLE tool-use
loop's wall time per point — the same unit [[llm-orchestrator]]'s live
estimator observes via `Orchestrator.ObserveCognition` ([[tool-loop]]), so a
seeded baseline and a live observation stay directly comparable and the
router's suppression arithmetic stays truthful when a cognition spends N
model calls, not one.

**Adaptive-throttle governor** (`governor.go`, spec 028, doctrine research R6):
extends the horizon from the other side — instead of only scoping what a
model may decide at a given speed, the world governs its own effective speed
when the player wants both high speed and high thought fidelity. The
player's speed setting becomes a CEILING, not a promise: `Debt` and
`Governor` are pure, stdlib-only functions of a snapshot the daemon supplies
every sample; nothing here calls a model, reads a wall clock, or is
config-tunable at runtime.

- **Debt** (`Debt(pending []PendingDebtInput, ticksPerSecond) (debt float64,
  jobs int)`): the aggregate staleness signal (spec 033, revising spec 028
  FR-001/FR-002) — for each pending thought the seconds are piecewise:
  `PredictedSec − ElapsedSec` while within prediction (remaining work, drains
  as today), `ElapsedSec` once overrun (full accrued drift, grows) — then
  `× ticksPerSecond / BudgetTicks(class)`. An overdue thought's elapsed time
  IS its grounded debt; the pre-033 floor-to-zero inverted the signal under
  overload (worst drift → least debt → no shed, the world-01 defect). The
  boundary jump at `ElapsedSec == PredictedSec` is doctrine
  (specs/033-governor-accrued-debt/contracts/debt-formula.md). `debt` is the
  sum, `jobs` counts only positive-contributing entries (overdue thoughts now
  contribute, so they count). Unknown kinds are skipped (they cannot reach a
  model) and `ticksPerSecond ≤ 0` (uncapped max) yields zero — pure
  arithmetic, no randomness, identical inputs always yield identical debt.
- **`Governor`** (a hysteresis state machine, one instance owned by the
  daemon's sampler, [[daemon-lifecycle]]): `Sample(debt, jobs, paused,
  effective, requested) Decision` counts consecutive qualifying SAMPLES, not
  wall durations, at the daemon's fixed `GovernorCadence`. Breach accrues
  while `debt > ShedThreshold` and the effective speed sits above the 1x
  floor (`clock.LadderIndex`); a continuous `BreachWindow` sheds exactly one
  notch (`clock.CappedLadder()[idx-1]`). Recovery accrues only while governed
  with room to climb — the debt PROJECTED at the next notch up (current debt
  scaled by that notch's tick-rate ratio, FR-006) stays under `ShedThreshold ×
  RecoverHeadroom`; a continuous `RecoveryWindow` (deliberately longer than
  `BreachWindow` — asymmetric hysteresis, US3) recovers one notch, never
  above the requested ceiling. Any decision, pause, or a speed change between
  samples resets both windows; a paused sample is a no-op (FR-013 — elapsed
  pause time never counts). At the 1x floor with debt still over threshold,
  the governor saturates silently — no decision, visible only via status.
- **Doctrine constants** (versioned with the code, never runtime knobs,
  FR-007 — same posture as the registry's points/budgets): `GovernorCadence`
  1 s (the daemon's sampling interval), `ShedThreshold` 1.0 (budget-fractions
  above which breach accrues), `BreachWindow` 5 s, `RecoverHeadroom` 0.5
  (scales `ShedThreshold` to the recovery ceiling), `RecoveryWindow` 20 s.
- **Who owns/calls it**: the daemon builds a `governorSampler` only when an
  orchestrator exists (a no-LLM world constructs zero governor machinery,
  FR-003/SC-004) and runs it in its own goroutine, sampling
  `llm.Orchestrator.PendingCognition()` ([[llm-orchestrator]]) and the
  [[sim-loop]]'s non-blocking status door every cadence, storing the debt
  reading for status, and issuing any resulting shed/recover `Decision`
  through the loop's `Govern` door — which lands it as a recorded
  `clock.governor_shed`/`clock.governor_recovered` event or drops it silently
  if it no longer applies ([[event-types]], [[sim-state-reducer]]). The
  package itself owns no goroutine, no wall clock, and no loop reference —
  only the pure `Debt` function and the `Governor` decision struct.

**Tool-call telemetry** (`internal/sim/cognition.go`, spec 017 FR-007):
`CogToolCallPayload` (`Job`, `Ordinal`, `Tool`, `Args` capped to 2 KiB,
`Verdict` — the stringified `toolloop.Verdict` enum — `Reason`, `Tier`,
`SnapshotTick`) is one record per tool call a cognition's loop saw: landed,
rejected, read, or unlanded. `{Job, Ordinal}` is the correlation key
(1-based, dense per job, model-emission order). It rides the same reducer-no-op
`cog.*` doctrine as every other cognition event ([[event-types]]).
`NewCogToolCallPayload` assembles the payload sim-side — deliberately with
only plain/stdlib argument types (no `toolloop` or `mind` import) — so both
loop consumers ([[agent-mind]]'s mind, [[metatron]]) unpack their own
`toolloop.CallRecord` and call this one shared constructor rather than each
inventing its own payload shape.

## Connections

The [[agent-mind]] consults `Route` before every enqueue (`routeVerdict` in
`internal/mind/telemetry.go`) and records suppressions and thought outcomes as
`cog.thought` / `cog.outcome` events ([[event-types]]); the suppression floors
are the [[reflex-policy]] and pre-authored templates. The [[llm-orchestrator]]
owns one `Estimator` per provider (spec 024), feeds it each completed call's
duration normalized by the kind's point cost (successes only), and exposes the
live estimate back to the mind via `EstimateForKind` — the kind's currently
admissible chain-head provider's estimate, so a fast small model is never
averaged with a slow quality model. Since spec 017 the planner's per-round calls
inside [[tool-loop]] each opt out of that per-call feed (`Request.SkipObserve`)
and the loop itself reports exactly one whole-cognition observation via
`Orchestrator.ObserveCognition` when it finishes — and only on a completed
termination (landed / model_done / cap_exhausted); the failure family
(admission_refused / provider_error / ctx_done) feeds the estimator nothing,
mirroring the single-shot worker's own successes-only doctrine so a governor
observation is always a completed cognition's true cost, never a fragment of
one or a fast failure. The [[sim-loop]] enforces the budget at landing:
an intent whose measured staleness exceeds its class's `BudgetTicks` is
rejected (`OutcomeRejectedStale`) at the injection door in
`internal/sim/loop.go`. The daemon ([[daemon-lifecycle]]) runs `ValidateKinds`
before any model is reachable and seeds the estimators from the profile;
[[cli-promptworld]]'s `calibrate` subcommand benchmarks the host+model and
writes the profile. The daemon's governor sampler ([[daemon-lifecycle]])
drives `Debt`/`Governor` from [[llm-orchestrator]]'s `PendingCognition`
snapshot and the [[sim-loop]]'s status/`Govern` doors; the router
([[sim-loop]]'s landing ladder, and every `Route` call above) reads the
EFFECTIVE speed the governor may have shed, so shedding speed deterministically
widens what the model may own and recovery narrows it again (spec 028 FR-010,
extending decision-4 from the other side).

## Operational notes

No environment variables; the only file read is `calibration.json` in the
world directory, and only `promptworld calibrate` writes it. With no profile,
bootstrap defaults (local 20 s/pt, cloud 10 s/pt) apply and the daemon prints
a reminder to run calibrate. Telemetry: router verdicts land as `cog.outcome`
events with the arithmetic string as the reason; estimator drift surfaces as
`cog.recalibration_recommended` (fires once per breach episode, and since spec
031 the same episode adopts — the payload's additive `prior_s_per_pt` →
`adopted_s_per_pt` fields carry the re-seed arithmetic, `estimate_s_per_pt`
remaining "current estimate at emission", i.e. the adopted value);
`Estimator.Stats` exposes estimate, rolling spike rate, and lifetime
sample/spike counts. `OutcomeRetried` (`"retried"`, TASK-42) is the one
NON-terminal outcome value — a scene reply failed to parse and the scene
continued via one retry; consumers summing job completions must filter it, and
the payload's optional `Raw`/`Retried` fields (omitempty) carry the failed
reply's bounded verbatim text and the consumed-a-retry flag on terminals. Budgets are never widened automatically — persistent
suppression or rejection on one class is a human retune signal.
