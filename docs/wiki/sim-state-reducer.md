---
name: sim-state-reducer
description: sim.State and Apply — the single event-driven mutation path used identically live and in replay; canonical JSON for hashing
kind: component
sources:
  - internal/sim/state.go
verified_against: 08d8c70e23c104a4c61df1749c00cb315f5c643d
---

# Sim state & reducer

`sim.State` is the whole world in one struct: clock state (tick, paused, speed,
degraded, effective rate) plus placeholder sim state (night flag, wanderers). Its
`Apply(event)` method is the **only** event-driven mutation path — the live loop and
crash recovery run the exact same code, which is what makes replay provably equal to
live execution.

## How it works

`NewState(seed)` is genesis: tick 0 (day 1 06:00), `DefaultSpeed` (4x), wanderer
positions derived from the seed via [[deterministic-rng]] — no stored RNG state.

`Apply` switches on event type: `clock.paused`/`clock.resumed` flip `Paused`;
`clock.speed_set` sets `Speed`; `clock.degraded`/`clock.recovered` maintain `Degraded`
and `EffectiveRate`; `sim.night_started`/`sim.day_started` flip `Night` and wake
wanderers; `agent.moved`/`agent.slept` update wanderer structs. Unknown types —
including `daemon.*` and `world.created` — are recorded history but state no-ops, so
new event types never break old replay.

**Tick is deliberately not event-sourced**: quiet ticks (no events) advance the clock
without a log row. The live loop mutates `state.Tick` directly; recovery sets it to
`max(snapshot tick, last event tick)` and re-lives any quiet tail deterministically.

Canonical bytes: `Marshal()` uses `encoding/json` over structs only (fixed field
order — payload shapes like `AgentMovedPayload` are structs, never maps), so equal
states produce identical bytes. `Hash()` is SHA-256 of that, used by [[snapshots]]
verification and the determinism tests. Wall-clock time never appears in state.

## Connections

[[sim-loop]] generates events via [[placeholder-sim]] and applies them here;
[[daemon-lifecycle]] replays the [[event-log]] through `Apply` at startup;
[[event-types]] lists every payload struct defined in this file.

## Operational notes

`EffectiveRate`/`Degraded` are part of state (hence snapshots) but only change via
explicitly emitted transition events, so unloaded same-machine runs stay
byte-deterministic. Adding a state field means adding events that set it — direct
mutation outside `Apply` (except `Tick`) breaks the replay contract.
