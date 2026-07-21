---
name: executor
description: The deterministic agent-body layer — integer needs with death, multi-step intents (forage/chop/hunt/build/eat/sleep), per-minute heartbeat, dynamic terrain overlays
kind: component
sources:
  - internal/sim/executor.go
  - internal/sim/agents.go
  - internal/sim/plan.go
  - internal/sim/terrain.go
verified_against: a49d615ec26d41ff14784f5a8f03f89d0e6c96f9
---

# Executor

The executor (TASK-5) replaced the placeholder wanderers: agents are now
deterministic bodies with needs, inventories, and multi-step intents, run unattended
by `stepEvents` between planner calls. The LLM planner (TASK-7) will *choose* goals;
the executor is what makes goals physically happen — and it must keep bodies alive
with no planner at all (the degraded-mode contract from the grounding session).

## How it works

**Agents** (`agents.go`): eight named bodies (`sim.AgentNames`) with authored
personas ([[agent-mind]]). `Needs{Health, Food, Rest, Warmth, Morale}` are integers 0..1000 —
integer math keeps decay byte-deterministic across platforms. `Inventory` carries
wood and food. All tuning constants (decay rates, action durations, yields, costs,
thresholds) sit at the top of `agents.go`.

**Heartbeat**: every game-minute (`tick%60 == 0`) each living agent's needs decay via
`decayNeeds`: food always falls; rest falls awake / recovers asleep; warmth falls at
night outdoors, recovers near fire or in shelter, drifts up by day. Zero food or zero
warmth drains health; health at 0 emits `agent.died` with cause `starvation` /
`exposure` / `collapse`. The new values land as one absolute `agent.needs_changed`
event per agent per minute (absolute values = replay-safe).

**Intents**: `Intent{Goal, Target, Res, WorkStart}` executes as a state machine —
walk (one tile per 5 ticks, staggered per agent, next hop from [[reflex-policy]]'s
BFS), then on arrival: instant goals (`sleep`, `wander`, `goto_warmth`) complete
immediately; work goals re-validate the resource (someone may have taken it), emit
`agent.work_started`, and after the goal's duration emit the completion event
(`agent.foraged/chopped/hunted/built`), which the reducer turns into inventory,
overlays, and a cleared intent.

**Guarded plans** (TASK-32, `plan.go`): a planner reply may land as a short
conditional plan — up to `PlanStepCap` (3) `PlanStep`s, each with a goal, an
optional `When` guard, and an `Until` validity deadline (default window
`PlanDefaultWindowTicks`, 2 game-hours). The steps live on `Agent.Plan` in
deterministic state (`agent.plan_set`); each idle tick the executor evaluates
the head step via `planStepEvents` *before* falling through to the reflex:
holding (guard false, window open) emits nothing, expiry or a failed goal
resolution clears the whole plan with `agent.plan_expired` (a broken sequence
is not resumed), and firing emits `agent.plan_step_started` plus the intent
with source `plan`. No model runs at firing time — timed guards are the sole
act-at-time-T mechanism. `Agent.Generation` (also TASK-32) counts
high-salience interrupts: the reducer bumps it on memories at or above
`GenerationBumpSalience` (9), and in-flight thoughts snapshotted under an
older generation are superseded when they land ([[cognition]]).

**Terrain overlays** (`terrain.go`): chopped trees and harvested forage are
event-sourced state over the static map — `effectiveKind`/`passable` merge
[[worldmap-generation]] with `State.Cleared`/`Harvested`; forage regrows after 12
game-hours (`sim.forage_regrown`), dens cool down 6 game-hours after a hunt.
Structures (`fire`, `shelter`) exist only in state; `warmAt` is fire within Manhattan
radius 2 or standing on a shelter.

The executor also emits `agent.memory_added` events from the salience table in
`memory.go` ([[agent-mind]]) alongside memorable happenings, and regenerates
Metatron's nudge charges (`metatron.charge_regenerated` at absolute 6-game-hour
tick boundaries while below the cap — [[metatron]]); its reflex fires only
on agents idle past `reflexGraceTicks` (120). `stepEvents` also runs the
[[gru]]'s whole turn (`gruStep`) each tick, and the heartbeat's near-death memory
names "the gru" as the cause when the last wound was recent. The per-minute social beat
(`socialEvents`, [[social-fabric]]) runs the adjacency ladder — repay an open
debt, give to a starving neighbor, or talk (chat-while-working, cooldown-bounded)
with a verbatim rumor fallback — and the hourly due-check breaks overdue debts
(also emitting a `norm.violated` when a repay-debts norm is in force — [[governance]]).
`stepEvents` further runs the whole governance layer (TASK-13, `governanceEvents` in
`governance.go`): the daily meeting lifecycle (11:30 convene with attendee intent
pinning to `attend_meeting`, noon open, speaking-turn beats, timebox+grace close)
and the per-minute curfew/exile violation detectors. `attend_meeting` is the one
intent goal the executor sets itself (never planner-choosable): arrival idles at
the meeting place until close, and stale pins clear when the meeting ends.
`stepEvents` stays a pure function of (pre-tick state, map, next tick);
every effect is an event through [[sim-state-reducer]] — the determinism and replay guarantees of
the substrate hold unchanged over the whole layer.

## Connections

[[reflex-policy]] decides what idle agents do; [[sim-loop]] drives the tick;
[[event-types]] catalogs the event families; the [[gru]] preys on the bodies at
night; [[tui-client]] renders bodies, needs gauges, and structures. TASK-7
replaces goal *selection*, never execution.

## Operational notes

A fresh village (seed 42) builds fires within the first game-hour and survives
multiple days unattended. Known day-1 quirk: agents can't see construction in
progress, so several may each build a fire in the same window — wasteful, harmless.
Event volume: ~4 needs events/game-minute plus movement bursts; a two-day run is
~100k events.
