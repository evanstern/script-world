---
name: sim-loop
description: The single-goroutine fixed-timestep loop — tick execution, command intents at tick boundaries, pacing, auto-slow degradation
kind: component
sources:
  - internal/sim/loop.go
verified_against: fdd311a7f7e8b0f5d2c759318a486cc8edd4a06f
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
exclusive with `Goal`), plus `Kind`/`Qty` (spec 013 R4) arguing a storage goal
(`drop`/`pick_up`/`deposit`/`withdraw`) when `Goal` is one of them — additive and
ignored otherwise, so pre-013 callers leave them zero. An empty `Class` means an
unmetered caller (tests, tooling): the ladder below is skipped and no telemetry
is emitted — the pre-TASK-32 contract.

At the boundary, a metered intent climbs the **landing ladder** against the
world as it is now (`staleness = state.Tick − SnapshotTick`, floored at 0):

1. dead/asleep agent → `rejected-unavailable`;
2. `Generation` mismatch with `Agent.Generation` → `superseded`;
3. staleness over the class's `BudgetTicks` (looked up via
   `cognition.ClassFor`) → `rejected-stale`;
4. any `Guard.Eval` failure → `rejected-guard` — EXCEPT the hail rung
   (TASK-47): a failed `target_present` on a `talk_to` landing whose living
   target is `hailable` (or is the actor's own hailer — mutual convergence)
   proceeds as **adapted** instead of rejecting; a `target_present` guard that
   holds but whose target moved likewise marks the landing **adapted** (the
   repair is `resolveGoal`'s re-resolution);
5. success: the goal must first be a World tool on the [[tool-registry]]'s
   villager roster (spec 014 US3 — an out-of-roster or unknown name rejects
   with the same `unknown goal` reason as before; real planner traffic is
   unaffected), then `resolveGoal` resolves coordinates deterministically,
   recorded as `agent.intent_set (source: planner, job: InjectArgs.JobID)` +
   `agent.thought` (since spec 017 the tool-use loop's job id threads onto the
   landed event's `Job` field at this single emission site), or — for a
   `Plan` — validated against `PlanStepCap` and `tool.PlanStepGoals()`,
   the registry-derived plan-step set (spec 014 FR-006; deriving it cured the
   TASK-55 drift where the old hand-maintained `planGoals` map silently
   rejected the nine spec-012 verbs — FR-012, the migration's sole behavioral
   delta; missing `Until` defaults to `state.Tick + PlanDefaultWindowTicks`)
   and recorded as `agent.plan_set`; a `resolveGoal` failure is itself
   `rejected-guard`. Since spec 019 (R2), a non-empty `InjectArgs.Reason` also
   rides onto the landed `agent.intent_set` event's `Reason` field (reflex-
   and executor-authored intent_set events carry none), so the planner's
   free-text reason survives to completion as recorded input rather than a
   second event. A
   successful `talk_to` landing with a `hailable` target additionally emits
   `social.hailed` (in- or out-of-radius — the courtesy pause is uniform;
   [[executor]] enforces it and resolves met/expiry).

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
narrator entries per [[chronicle]], nudges and miracles per [[metatron]] /
[[metatron-miracles]], proposal rephrasing
per [[governance]] — `agent.thought` is
whitelisted as a reducer no-op, `chronicle.entry` appends the story ring,
`metatron.nudged` spends a charge with a validating reducer the dry-run enforces,
the four `metatron.time_snapped`/`metatron.item_granted`/`metatron.entity_moved`/
`metatron.entity_removed` miracle types (spec 016) are whitelisted the same way —
their reducer arms enforce presence/destination/charge before anything lands,
the whitelist is only the isolation boundary — `meeting.proposal_rephrased` swaps
an enacted norm's text and nothing else,
the `cog.*` telemetry — `cog.thought`, `cog.outcome`,
`cog.recalibration_recommended`, and (since spec 017) `cog.tool_call` (the
tool-use loop's per-call trace, [[tool-loop]]) — is whitelisted as reducer
no-ops so the [[cognition]] layer's observability is recorded, never silent,
and (since spec 019, US3) `journal.entry_written`/`journal.entry_deleted` —
the two mind-injectable journal mutations, whose reducer dry-run enforces the
rune budget (written) and entry existence (deleted) before either lands):
an atomic, whitelisted batch of conversation, consolidation, musing, chronicle,
nudge, miracle, phrasing, or telemetry effects, dry-run on a state copy before
applying — the dry-run probe is reconstructed from bytes and so carries no
unexported/unserialized state, so `handleCommand` re-attaches the loop's static
map (`probe.SetMap(l.m)`) before applying, letting miracle arms validate the
terrain vocabulary in the dry-run exactly as the real apply and replay will.
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
[[metatron-miracles]]'s four event types ride `InjectSocial`'s whitelist.
[[tool-loop]] is the caller behind both doors' villager/metatron traffic since
spec 017 — its handlers wrap `InjectIntent` (world verbs, `set_plan`) and
`InjectSocial` (`muse`, and Metatron's nudges/`work_miracle`), and its buffered
`CallRecord`s land as the `cog.tool_call` batch through the same social door.

## Operational notes

Measured throughput at max speed on the target machine: ~1.65M ticks/sec, measured on
the TASK-2-era placeholder sim before the village systems landed (the full village does
more work per tick). Store errors inside the loop are fatal (the daemon exits) — an
unwritable log must never silently diverge from state.
