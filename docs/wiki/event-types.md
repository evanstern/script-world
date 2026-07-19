---
name: event-types
description: The event taxonomy — namespaced types, their payload structs, who emits them, and their reducer effects
kind: concept
sources:
  - internal/sim/state.go
  - internal/sim/agents.go
  - internal/sim/executor.go
  - internal/sim/loop.go
  - internal/daemon/daemon.go
verified_against: aff0448e78ebec0f7724fc4c8ab02d4961e37236
---

# Event types

Every event has a namespaced `type` and a canonical-JSON payload defined as a Go
struct in `internal/sim/state.go` (structs, never maps, so bytes are deterministic).
This catalog is the contract downstream consumers (chronicle, Metatron digests, the
TUI) will read.

## How it works

| Type | Payload struct | Emitted by | Reducer effect |
|---|---|---|---|
| `world.created` | `WorldCreatedPayload{name, seed}` | CLI `new`, tick 0 | none (genesis marker) |
| `clock.paused` / `clock.resumed` | `{}` | loop command | pause flag (+ snapshot on pause) |
| `clock.speed_set` | `SpeedSetPayload{speed}` | loop command | `Speed` updated |
| `clock.degraded` / `clock.recovered` | `DegradedPayload{effective_rate}` / `{}` | loop auto-slow | degradation flags |
| `sim.day_started` / `sim.night_started` | `DayPayload{day}` | executor, 06:00/22:00 | `Night` flag only — waking is explicit |
| `sim.forage_regrown` | `RegrownPayload{x, y}` | executor, regrow tick | harvest overlay removed |
| `agent.intent_set` | `IntentSetPayload{agent, goal, target, res, source}` | reflex (grace-gated) or planner injection | intent installed; `source` says which mind chose it |
| `agent.work_started` | `WorkStartedPayload{agent, tick}` | executor at target | `WorkStart` stamped |
| `agent.intent_done` | `AgentPayload{agent}` | executor (done/invalid/unreachable) | intent cleared |
| `agent.moved` | `AgentMovedPayload{agent, x, y}` | executor pathing | position updated |
| `agent.foraged` / `agent.chopped` / `agent.hunted` | `HarvestPayload{agent, x, y}` | work completion | +food/+wood, overlay (harvest/cleared/den cooldown), intent cleared |
| `agent.built` | `BuiltPayload{agent, kind, x, y}` | work completion | structure added, wood spent, intent cleared |
| `agent.ate` | `AgentPayload{agent}` | reflex (instant) | −1 carried food, +350 food need |
| `agent.slept` / `agent.woke` | `AgentPayload{agent}` | executor | sleep flag (slept clears intent) |
| `agent.needs_changed` | `NeedsPayload{agent, …}` | per-game-minute heartbeat | needs set to absolute values |
| `agent.died` | `DiedPayload{agent, cause}` | heartbeat at 0 health | `Dead`, intent cleared |
| `agent.talked` | `TalkedPayload{a, b}` | executor, adjacent idle pair | +morale both, talk cooldown; both remember |
| `agent.memory_added` | `MemoryAddedPayload{agent, text, salience}` | executor salience heuristics | append to `Memories` ([[agent-mind]]) |
| `agent.thought` | `ThoughtPayload{agent, text, source}` | `inject_intent` command | none (chronicle material) |
| `daemon.started` / `daemon.stopped` | `DaemonStartedPayload` / `DaemonStoppedPayload` | daemon lifecycle | none |

Conventions: `clock.*` are applied player/scheduler commands; `sim.*` and `agent.*`
are world happenings (pure functions of state + seed + tick); `daemon.*` are process
bookkeeping, wall-time dependent, and excluded from determinism comparisons (as are
`clock.*` in the binary-level test, since their ticks depend on command timing).
Payloads record **outcomes** (positions reached, absolute need values), never dice
rolls, so replay needs no RNG. Unknown types are no-ops in the reducer, so adding
types is backward-compatible with old replay code.

## Connections

[[sim-state-reducer]] applies these; the [[executor]], [[reflex-policy]], and
[[sim-loop]] emit the sim/agent/clock families; [[daemon-lifecycle]] emits `daemon.*`; [[event-log]] stores them;
[[ipc-protocol]] pushes them to subscribers verbatim.

## Operational notes

Later features add families (TASK-8 rumors/relationships, TASK-10 the gru, TASK-11
chronicle annotations). The outcome-payload convention ([[deterministic-rng]]) is
load-bearing — keep it.
