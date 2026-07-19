---
name: reflex-policy
description: Deterministic survival decisions for idle agents (eat → food → night warmth → rest → prep → wander) plus BFS pathfinding with fixed neighbor order
kind: component
sources:
  - internal/sim/policy.go
  - internal/sim/path.go
verified_against: aff0448e78ebec0f7724fc4c8ab02d4961e37236
---

# Reflex policy & pathfinding

`decideIntent` is the deterministic pure function that gives idle, awake agents
something to do — since TASK-7, only agents idle past the 120-tick grace (the
planner's injection window). It is the permanent degraded mode: when no planner
thoughts arrive, this keeps bodies alive. `resolveGoal` (same file) is the shared
target resolver: planner goals from [[agent-mind]] resolve to coordinates through
the exact same nearest-X helpers the reflex uses.

## How it works

Priority ladder (first match wins):

1. **Eat** — hungry (`Food < 350`) with food in inventory → instant `agent.ate`.
2. **Get food** — hungry, nothing carried → nearest reachable effective-forage tile,
   else nearest ready den (`hunt`).
3. **Night, cold** — reach existing warmth (`goto_warmth`), else build a fire if
   carrying 2 wood, else chop the nearest tree (yes, chopping in the cold dark —
   that's the day-1 drama working as designed).
4. **Night, warm** — sleep where you stand.
5. **Exhausted by day** (`Rest < 250`) — nap, preferring a warm tile.
6. **Prep** — no fire in the village → build/chop toward one; then a shelter (5
   wood); then stock the larder to 3 carried meals.
7. **Wander** — a seeded short stroll (`rngAt(seed, "wander", tick, idx)`).

Waking (`wakeReason` in executor.go) mirrors this: day + decent rest, or a hunger
emergency the agent can act on (food in hand). Fully-rested agents sleep through the
night — the live-run sleep/wake churn bug is documented in the TASK-5 notes.

Pathfinding (`path.go`): breadth-first search with **fixed neighbor order (N, E, S,
W)** and FIFO frontier, so shortest paths and nearest-match searches are identical
on every run. `nextStep` re-derives one hop per move from the shortest path (paths
are never stored in state — movement outcomes are evented, so replay needs no path
data). `nearest` finds the closest reachable tile matching a predicate in BFS order;
`nearestAdjacentTo` finds a standing tile beside a resource (chop from beside the
tree). The escape clause lets an agent standing on impassable terrain (pre-terrain
saves) step out.

## Connections

[[executor]] invokes decisions on a staggered cadence and executes the resulting
intents; passability comes from [[executor]]'s terrain overlays over
[[worldmap-generation]]; randomness only via [[deterministic-rng]] purpose tags
(`wander`, plus `genesis` placement in [[sim-state-reducer]]).

## Operational notes

BFS over a 64×64 map per decision/move is the current throughput ceiling — the
executor still clears >200k ticks/sec in the test harness, and auto-slow
([[sim-loop]]) degrades honestly under load. TASK-7 replaces this ladder with
planner-chosen goals; the ladder itself must remain reachable as the fallback.
