---
name: cognition
description: The cognition horizon substrate — decision-class registry (Fibonacci points, game-tick staleness budgets), seconds-per-point estimation with spike rejection, deterministic LLM-vs-reflex routing, and the calibration profile
kind: component
sources:
  - internal/cognition/doc.go
  - internal/cognition/registry.go
  - internal/cognition/estimate.go
  - internal/cognition/route.go
  - internal/cognition/calibration.go
  - internal/sim/cognition.go
verified_against: cabe1fb4fdc5fd575a58b33f4b22a184280d467d
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
[[tool-loop]]). `kindToClass` maps every LLM call kind (as a string, keeping
the package leaf) to a class; `ValidateKinds` enforces FR-002 at daemon start
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

**Estimation** (`estimate.go`): `Estimator` holds one tier's live
seconds-per-point as an EWMA (`EWMAAlpha` 0.2) over per-point-normalized call
durations. A sample beyond `SpikeFactor` (3.0) times the current estimate is
excluded from the EWMA but counted in a `WindowSize`-20 ring; when the rolling
spike rate over a full window first exceeds `BreachRate` (0.3), `Sample`
returns true exactly once (re-armed after the rate falls back) — the
`cog.recalibration_recommended` signal. One-shot lag spikes are thus rejected
while systemic drift is followed. The estimator is process-lifetime only.

**Calibration** (`calibration.go`): `Profile` is `calibration.json` in the
world save directory (`World.CalibrationPath()`), written only by the
`promptworld calibrate` subcommand (full-file replace via `Save`) — the daemon
never writes it, so the recorded baseline moves only under a human's hand.
`LoadProfile` treats a missing file as legal (nil, nil) and a malformed one as
a warning the daemon downgrades to bootstrap defaults. `SeedFor` returns a
tier's recorded seconds-per-point, or the deliberately pessimistic bootstrap
constants (`BootstrapLocalSecPerPt` 20.0, `BootstrapCloudSecPerPt` 10.0) — an
uncalibrated world fails toward reflex, never toward stale action.
`TierProfile.SecondsPerPoint`'s unit is doctrine, spelled out since spec 017:
for a single-shot kind it is one model call's wall time per point; for a loop
cognition (the villager planner on the local tier) it is the WHOLE tool-use
loop's wall time per point — the same unit [[llm-orchestrator]]'s live
estimator observes via `Orchestrator.ObserveCognition` ([[tool-loop]]), so a
seeded baseline and a live observation stay directly comparable and the
router's suppression arithmetic stays truthful when a cognition spends N
model calls, not one.

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
owns one `Estimator` per tier, feeds it each completed call's duration
normalized by the kind's point cost (successes only), and exposes the live
estimate back to the mind. Since spec 017 the planner's per-round calls
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
writes the profile.

## Operational notes

No environment variables; the only file read is `calibration.json` in the
world directory, and only `promptworld calibrate` writes it. With no profile,
bootstrap defaults (local 20 s/pt, cloud 10 s/pt) apply and the daemon prints
a reminder to run calibrate. Telemetry: router verdicts land as `cog.outcome`
events with the arithmetic string as the reason; estimator drift surfaces as
`cog.recalibration_recommended` (fires once per breach, re-armed on recovery);
`Estimator.Stats` exposes estimate, rolling spike rate, and lifetime
sample/spike counts. `OutcomeRetried` (`"retried"`, TASK-42) is the one
NON-terminal outcome value — a scene reply failed to parse and the scene
continued via one retry; consumers summing job completions must filter it, and
the payload's optional `Raw`/`Retried` fields (omitempty) carry the failed
reply's bounded verbatim text and the consumed-a-retry flag on terminals. Budgets are never widened automatically — persistent
suppression or rejection on one class is a human retune signal.
