---
name: sim-loop
description: The single-goroutine fixed-timestep loop — tick execution, command intents at tick boundaries, pacing, auto-slow degradation
kind: component
sources:
  - internal/sim/loop.go
verified_against: 8f24c13a5b2eb1c1f37244978055e3f6eb5d42d2
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
([[agent-mind]]): validated, resolved to coordinates deterministically at the
boundary via `resolveGoal`, recorded as `agent.intent_set (source: planner)` +
`agent.thought`. `Loop.InjectSocial` is the second door — the mind's injection
door ([[social-fabric]], [[nightly-consolidation]], musings per [[agent-mind]],
narrator entries per [[chronicle]], nudges per [[metatron]], proposal rephrasing
per [[governance]] — `agent.thought` is
whitelisted as a reducer no-op, `chronicle.entry` appends the story ring,
`metatron.nudged` spends a charge with a validating reducer the dry-run enforces,
`meeting.proposal_rephrased` swaps an enacted norm's text and nothing else):
an atomic, whitelisted batch of conversation, consolidation, musing, chronicle,
nudge, or phrasing effects, dry-run on a state copy before applying. Model output enters
the sim only through these two doors, as recorded input. The protocol `Status`
carries `MetatronCharges` so clients render the ⚡ bank without a state fetch.

## Connections

[[game-clock]] supplies intervals; the [[executor]] supplies tick events;
[[sim-state-reducer]] is the mutation path; [[event-log]] and [[snapshots]] persist;
[[ipc-server]] feeds commands in and broadcasts events out; [[daemon-lifecycle]] owns
the ctx whose cancellation triggers the final snapshot.

## Operational notes

Measured throughput at max speed on the target machine: ~1.65M ticks/sec with the
placeholder sim. Store errors inside the loop are fatal (the daemon exits) — an
unwritable log must never silently diverge from state.
