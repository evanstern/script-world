---
name: event-types
description: The event taxonomy — namespaced types, their payload structs, who emits them, and their reducer effects
kind: concept
sources:
  - internal/sim/state.go
  - internal/sim/placeholder.go
  - internal/sim/loop.go
  - internal/daemon/daemon.go
verified_against: f4786fdb378059d04d20f2b8c8bced549d7a9922
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
| `clock.paused` | `{}` | loop command | `Paused = true` (+ snapshot) |
| `clock.resumed` | `{}` | loop command | `Paused = false` |
| `clock.speed_set` | `SpeedSetPayload{speed}` | loop command | `Speed` updated |
| `clock.degraded` | `DegradedPayload{effective_rate}` | loop auto-slow | `Degraded = true`, rate recorded |
| `clock.recovered` | `{}` | loop auto-slow | `Degraded = false`, rate restored |
| `sim.day_started` | `DayPayload{day}` | placeholder, 06:00 | `Night = false`, wake all |
| `sim.night_started` | `DayPayload{day}` | placeholder, 22:00 | `Night = true` |
| `agent.moved` | `AgentMovedPayload{agent, x, y}` | placeholder, minute boundary | position updated |
| `agent.slept` | `AgentPayload{agent}` | placeholder, night start | `Asleep = true` |
| `daemon.started` | `DaemonStartedPayload{tick, recovery_ms}` | daemon startup | none |
| `daemon.stopped` | `DaemonStoppedPayload{tick}` | daemon shutdown | none |

Conventions: `clock.*` are applied player/scheduler commands; `sim.*` and `agent.*`
are world happenings (pure functions of seed + tick); `daemon.*` are process
bookkeeping, wall-time dependent, and excluded from determinism comparisons (as are
`clock.*` in the binary-level test, since their ticks depend on command timing).
Unknown types are no-ops in the reducer, so adding types is backward-compatible with
old replay code.

## Connections

[[sim-state-reducer]] applies these; [[placeholder-sim]] and [[sim-loop]] emit the
sim/clock families; [[daemon-lifecycle]] emits `daemon.*`; [[event-log]] stores them;
[[ipc-protocol]] pushes them to subscribers verbatim.

## Operational notes

Later features add families (TASK-5 needs/death, TASK-8 rumors, TASK-10 the gru).
The convention to preserve: outcome-carrying payloads (record what happened, not what
was rolled) so replay never needs to re-randomize, per [[deterministic-rng]].
