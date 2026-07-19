---
name: executor
description: The deterministic agent-body layer — integer needs with death, multi-step intents (forage/chop/hunt/build/eat/sleep), per-minute heartbeat, dynamic terrain overlays
kind: component
sources:
  - internal/sim/executor.go
  - internal/sim/agents.go
  - internal/sim/terrain.go
verified_against: cdb24b60395f9f75d86df545df7dcc027f384bcb
---

# Executor

The executor (TASK-5) replaced the placeholder wanderers: agents are now
deterministic bodies with needs, inventories, and multi-step intents, run unattended
by `stepEvents` between planner calls. The LLM planner (TASK-7) will *choose* goals;
the executor is what makes goals physically happen — and it must keep bodies alive
with no planner at all (the degraded-mode contract from the grounding session).

## How it works

**Agents** (`agents.go`): four named bodies (`Ash`, `Birch`, `Cedar`, `Rowan` until
TASK-7 personas). `Needs{Health, Food, Rest, Warmth, Morale}` are integers 0..1000 —
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

**Terrain overlays** (`terrain.go`): chopped trees and harvested forage are
event-sourced state over the static map — `effectiveKind`/`passable` merge
[[worldmap-generation]] with `State.Cleared`/`Harvested`; forage regrows after 12
game-hours (`sim.forage_regrown`), dens cool down 6 game-hours after a hunt.
Structures (`fire`, `shelter`) exist only in state; `warmAt` is fire within Manhattan
radius 2 or standing on a shelter.

`stepEvents` stays a pure function of (pre-tick state, map, next tick); every effect
is an event through [[sim-state-reducer]] — the determinism and replay guarantees of
the substrate hold unchanged over the whole layer.

## Connections

[[reflex-policy]] decides what idle agents do; [[sim-loop]] drives the tick;
[[event-types]] catalogs the event families; [[tui-client]] renders bodies, needs
gauges, and structures. TASK-7 replaces goal *selection*, never execution.

## Operational notes

A fresh village (seed 42) builds fires within the first game-hour and survives
multiple days unattended. Known day-1 quirk: agents can't see construction in
progress, so several may each build a fire in the same window — wasteful, harmless.
Event volume: ~4 needs events/game-minute plus movement bursts; a two-day run is
~100k events.
