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
verified_against: 8be4440aae8d108884080cb6476782d2f11ad165
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
staleness budget in game ticks (a property of the fiction). Seven classes are
registered (`planner` 3pt/1200t degrading to reflex, `musing` 1pt/3600t,
`conversation` 13pt/7200t, `meeting` 2pt/3600t degrading to a template,
`consolidation` 5pt/28800t, `chronicle` 5pt/86400t, `metatron` 5pt/86400t);
values are doctrine — changing one is a reviewed code change, never runtime
tuning. `kindToClass` maps every LLM call kind (as a string, keeping the
package leaf) to a class; `ValidateKinds` enforces FR-002 at daemon start —
an unmapped kind, a non-Fibonacci point value, or a non-positive budget is a
fatal startup error. `Degrade` names the suppression floor: `skip` (recorded,
not silent), `reflex`, `template`, or `faster-tier` (registry-expressible but
treated as skip in v1).

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

## Connections

The [[agent-mind]] consults `Route` before every enqueue (`routeVerdict` in
`internal/mind/telemetry.go`) and records suppressions and thought outcomes as
`cog.thought` / `cog.outcome` events ([[event-types]]); the suppression floors
are the [[reflex-policy]] and pre-authored templates. The [[llm-orchestrator]]
owns one `Estimator` per tier, feeds it each completed call's duration
normalized by the kind's point cost (successes only), and exposes the live
estimate back to the mind. The [[sim-loop]] enforces the budget at landing:
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
