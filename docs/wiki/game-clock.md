---
name: game-clock
description: Game time math — 1 tick = 1 game second, epoch day 1 06:00, Speed type mapping game time to real time (1x/4x/8x/16x/max)
kind: component
sources:
  - internal/clock/clock.go
verified_against: 08d8c70e23c104a4c61df1749c00cb315f5c643d
---

# Game clock

`internal/clock` owns all conversion between ticks, game time, and real time. It is
pure math with no state: the current tick lives in the sim state, not here.

## How it works

Constants: `TickGameSeconds = 1` (fixed for format_version 1, recorded in the world
manifest so it can never silently change under an existing run); `EpochSecondOfDay =
6*3600` — tick 0 is **day 1, 06:00**. Days are 1-based.

`Speed` is a string type with exactly five values: `Speed1x`, `Speed4x` (the
`DefaultSpeed`: 1 game minute per 15 real seconds), `Speed8x`, `Speed16x`, `SpeedMax`.
The numeric meaning is game-seconds per real-second; `SpeedMax` maps to 0 as an
"uncapped" sentinel. `ParseSpeed` rejects anything else — validation happens both in
the CLI and in the loop's `set_speed` handling.

Key functions:

- `Speed.TicksPerSecond() float64` — requested tick rate; 0 means uncapped.
- `Speed.Interval() time.Duration` — real-time budget per tick; 0 for max. This is
  what the scheduler in [[sim-loop]] paces against.
- `GameTime(tick) (day, hour, min, sec)` — calendar coordinates via integer division;
  no floats, so no drift over weeks-long runs.
- `SecondOfDay(tick)` — used by [[placeholder-sim]] to detect the 22:00 night start
  and 06:00 day start boundaries by exact equality (valid because ticks are whole
  game seconds).
- `Format(tick)` — the display form used everywhere: `"day N HH:MM"`.

## Connections

[[sim-loop]] converts `Speed` to a scheduling interval; [[sim-state-reducer]] stores
the current `Speed` and pause flag; [[placeholder-sim]] and [[event-types]] use
day/night boundary detection; [[cli-scriptworld]] prints `Format` output.

## Operational notes

A game day is 86,400 ticks; at default 4x that is 6 real hours per game day. Night
(22:00–06:00) is 8 game hours. The clock has no notion of pause or degradation — those
are loop/state concerns; this package would be unchanged by any speed-policy rework
that keeps the five speed values.
