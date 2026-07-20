---
name: sim-state-reducer
description: sim.State and Apply — the single event-driven mutation path used identically live and in replay; canonical JSON for hashing
kind: component
sources:
  - internal/sim/state.go
  - internal/sim/agents.go
verified_against: be54bb42adcbd14421c20269efc79da7b6beab9f
---

# Sim state & reducer

`sim.State` is the whole world in one struct: clock state (tick, paused, speed,
degraded, effective rate) plus the living world — agents with needs/intents/
inventories/memories (with `IdleSince` for the reflex grace and a `NearDeath`
latch), structures, cleared trees, harvested forage, den cooldowns, the social
fabric — relation edges, the debt ledger, the rumor registry with per-holder
variants ([[social-fabric]]) — the consolidated inner life: per-agent beliefs,
self-narrative, and the once-per-night consolidation ledger
([[nightly-consolidation]]) — and the [[gru]] (`Gru *Gru`, nil while not abroad;
`omitempty` keeps pre-TASK-10 snapshots valid) (executor types in `agents.go`;
memories belong to [[agent-mind]]). Its
`Apply(event)` method is the **only** event-driven mutation path — the live loop and
crash recovery run the exact same code, which is what makes replay provably equal to
live execution.

## How it works

`NewState(seed, m)` is genesis: tick 0 (day 1 06:00), `DefaultSpeed` (4x), eight
named agents on distinct passable tiles via [[deterministic-rng]], with deliberately
imperfect needs — day 1 must demand foraging, wood, and a fire before dark.

`Apply` switches on event type: `clock.*` maintain pause/speed/degradation;
`sim.night_started`/`sim.day_started` flip `Night` (waking is an explicit
`agent.woke`, never implicit); `sim.forage_regrown` clears a harvest overlay; the
`agent.*` family ([[event-types]]) drives intents, movement, work products
(inventory + overlays + structures), eating, sleep, talk, needs (absolute values),
and death; the `gru.*` family dispatches to `applyGru` in `gru.go` ([[gru]]).
Unknown types — including `daemon.*` and `world.created` — are recorded
history but state no-ops, so new event types never break old replay.

**Tick is deliberately not event-sourced**: quiet ticks (no events) advance the clock
without a log row. The live loop mutates `state.Tick` directly; recovery sets it to
`max(snapshot tick, last event tick)` and re-lives any quiet tail deterministically.

Canonical bytes: `Marshal()` uses `encoding/json` over structs only (fixed field
order — payload shapes like `AgentMovedPayload` are structs, never maps), so equal
states produce identical bytes. `Hash()` is SHA-256 of that, used by [[snapshots]]
verification and the determinism tests. Wall-clock time never appears in state.

## Connections

[[sim-loop]] generates events via the [[executor]] and applies them here;
[[daemon-lifecycle]] replays the [[event-log]] through `Apply` at startup;
[[event-types]] lists every payload struct defined in this file.

## Operational notes

`EffectiveRate`/`Degraded` are part of state (hence snapshots) but only change via
explicitly emitted transition events, so unloaded same-machine runs stay
byte-deterministic. Adding a state field means adding events that set it — direct
mutation outside `Apply` (except `Tick`) breaks the replay contract.
