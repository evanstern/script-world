---
name: sim-loop
description: The single-goroutine fixed-timestep loop — tick execution, command intents at tick boundaries, pacing, auto-slow degradation
kind: component
sources:
  - internal/sim/loop.go
verified_against: a49d615ec26d41ff14784f5a8f03f89d0e6c96f9
---

# Sim loop

`sim.Loop` is the one goroutine that owns `State` and the write path to the store,
holding the static terrain (`worldmap.Map`, via `NewLoop(state, m, store, notify)`)
as read-only context for tick generation.
Everything external — pause, resume, speed changes, status reads — enters through a
command channel and is applied at a tick boundary, with every applied command recorded
as an event. That makes the [[event-log]] the complete input record of a run.

## How it works

`Loop.Run(ctx)` is a state machine over three modes:

- **Paused**: no timer; block on commands or ctx. Resume restarts pacing fresh.
- **Timed** (interval > 0): a timer fires per `Speed.Interval()`; each firing runs one
  tick and advances the schedule by exactly one interval. If the loop falls more than
  one interval behind, the schedule resets to now — **no catch-up bursts**; the world
  slows honestly instead of skipping (FR-012).
- **Max speed** (interval 0): spin ticks back-to-back with a non-blocking command
  check and a `runtime.Gosched()` every 1024 ticks.

`runTick`: compute `stepEvents(state, map, nextTick)` (pure), advance `state.Tick`, apply
each event through the reducer, `AppendEvents` in one transaction, then `notify`
(the [[ipc-server]] broadcast — must never block). Every `SnapshotEveryTicks = 3600`
ticks it snapshots and prunes.

`handleCommand` implements idempotent semantics: pausing a paused world emits nothing;
`set_speed` to the current speed emits nothing; otherwise the `clock.*` event is
applied, appended, and broadcast, and a pause also triggers an immediate snapshot.
Replies carry a coherent `Status` snapshot (tick, game time, flags, last seq).
Emitted events now land regardless of the command's error — a rejected
`inject_intent` is the only command that pairs an error with events (its rejection
telemetry, so no failure is silent); every other error path emits nothing.

Auto-slow (`observeWindow`): every `degradeWindow = 5s` the loop compares achieved
ticks/sec against the requested rate; sustained shortfall below 90% emits
`clock.degraded` (with the measured rate), recovery to ≥95% emits `clock.recovered`.
At max speed whatever is achieved is the contract — no degradation events.

`Loop.Do(name, speed)` is the thread-safe entry used by IPC sessions; it fails cleanly
via the loop's `done` channel if the loop has stopped. `Loop.DoState()` answers the
protocol's `state` command with the canonical `State` JSON plus a status captured in
the same loop iteration — the returned `last_seq` is exactly the log position the
state reflects, which is what makes client-side replicas gapless.
`Loop.InjectIntent` (the `inject_intent` command) is the door for planner output
([[agent-mind]]). `InjectArgs` now carries cognition-horizon landing metadata
([[cognition]]): `Class`, `JobID`, `SnapshotTick`, `Generation`,
`PredictedWallMs`/`ActualWallMs`, `Guards`, and an optional `Plan` (mutually
exclusive with `Goal`). An empty `Class` means an unmetered caller (tests,
tooling): the ladder below is skipped and no telemetry is emitted — the
pre-TASK-32 contract.

At the boundary, a metered intent climbs the **landing ladder** against the
world as it is now (`staleness = state.Tick − SnapshotTick`, floored at 0):

1. dead/asleep agent → `rejected-unavailable`;
2. `Generation` mismatch with `Agent.Generation` → `superseded`;
3. staleness over the class's `BudgetTicks` (looked up via
   `cognition.ClassFor`) → `rejected-stale`;
4. any `Guard.Eval` failure → `rejected-guard`; a `target_present` guard that
   holds but whose target moved marks the landing **adapted** (the repair is
   `resolveGoal`'s re-resolution);
5. success: `resolveGoal` resolves coordinates deterministically, recorded as
   `agent.intent_set (source: planner)` + `agent.thought`, or — for a `Plan` —
   validated against `PlanStepCap` and the `planGoals` vocabulary (missing
   `Until` defaults to `state.Tick + PlanDefaultWindowTicks`) and recorded as
   `agent.plan_set`; a `resolveGoal` failure is itself `rejected-guard`.

Every metered verdict lands atomically as `cog.outcome` (rejections also emit
`agent.intent_rejected`), classified `prediction-miss` when
`ActualWallMs > PredictionMissFactor × PredictedWallMs` and `world-change`
otherwise — see [[event-types]] for payload shapes.

Both injection doors are deliberately pause-open (FR-018): pause means "the
world freezes and the minds catch up" — an in-flight thought completes on the
wall clock and lands at the frozen tick, where its game-tick staleness is zero
by construction.

`Loop.InjectSocial` is the second door — the mind's injection
door ([[social-fabric]], [[nightly-consolidation]], musings per [[agent-mind]],
narrator entries per [[chronicle]], nudges per [[metatron]], proposal rephrasing
per [[governance]] — `agent.thought` is
whitelisted as a reducer no-op, `chronicle.entry` appends the story ring,
`metatron.nudged` spends a charge with a validating reducer the dry-run enforces,
`meeting.proposal_rephrased` swaps an enacted norm's text and nothing else,
and the `cog.*` telemetry triple — `cog.thought`, `cog.outcome`,
`cog.recalibration_recommended` — is whitelisted as reducer no-ops so the
[[cognition]] layer's observability is recorded, never silent):
an atomic, whitelisted batch of conversation, consolidation, musing, chronicle,
nudge, phrasing, or telemetry effects, dry-run on a state copy before applying.
Model output enters
the sim only through these two doors, as recorded input. The protocol `Status`
carries `MetatronCharges` so clients render the ⚡ bank without a state fetch.

## Connections

[[game-clock]] supplies intervals; the [[executor]] supplies tick events;
[[sim-state-reducer]] is the mutation path; [[event-log]] and [[snapshots]] persist;
[[ipc-server]] feeds commands in and broadcasts events out; [[daemon-lifecycle]] owns
the ctx whose cancellation triggers the final snapshot. The landing ladder's
budgets and classes come from [[cognition]] (`cognition.ClassFor`), whose router
and estimators produce the snapshot/landing metadata the ladder judges.

## Operational notes

Measured throughput at max speed on the target machine: ~1.65M ticks/sec with the
placeholder sim. Store errors inside the loop are fatal (the daemon exits) — an
unwritable log must never silently diverge from state.
