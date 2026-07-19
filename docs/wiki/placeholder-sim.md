---
name: placeholder-sim
description: Two wanderers on a 16×16 grid with a 22:00/06:00 day-night cycle — deliberately minimal scaffolding that exercises the substrate
kind: component
sources:
  - internal/sim/placeholder.go
verified_against: f4786fdb378059d04d20f2b8c8bced549d7a9922
---

# Placeholder simulation

The placeholder sim exists only to push real, deterministic events through the
substrate — log, snapshots, protocol — until the actual village systems (TASK-4 map,
TASK-5 executor) replace it. The plan capped it at ~150 lines to keep it from becoming
accidental gameplay.

## How it works

Constants: `wandererCount = 2`, `GridSize = 16` (exported for map rendering in the
[[tui-client]]), `nightStartSecond = 22*3600`, `dayStartSecond = 6*3600`.

`stepEvents(s *State, nextTick)` is a **pure function** of (state, next tick) — it
must not mutate state; the loop applies its returned events through the reducer:

- At `SecondOfDay(nextTick) == nightStartSecond`: emit `sim.night_started` plus one
  `agent.slept` per wanderer.
- At `dayStartSecond`: emit `sim.day_started` (whose reducer effect wakes everyone).
- On each game-minute boundary (`nextTick%60 == 0`, skipping the night-start tick):
  each awake wanderer takes a random step — `rngAt(seed, "move", nextTick, i)` drawing
  a −1/0/+1 delta per axis, clamped to the grid — emitting `agent.moved` only when the
  position actually changes.

Because movement stops while asleep, nights (22:00→06:00, 8 game hours) are almost
event-silent; at default 4x that is two real hours of quiet, which is why
[[snapshots]] cadence — not event frequency — bounds crash-recovery clock loss.

## Connections

[[sim-loop]] calls `stepEvents` each tick; [[deterministic-rng]] supplies the
randomness; [[game-clock]] provides the boundary detection; the emitted types are
cataloged in [[event-types]] and applied by [[sim-state-reducer]].

## Operational notes

Event rate at default speed: 2 moves/game-minute daytime ≈ 8 events/real-minute at 4x.
When TASK-5 lands, this file is expected to be deleted or replaced wholesale; tests
that count on wanderer behavior (`TestDayNightCycle`, e2e determinism) will need
re-targeting at the same time.
