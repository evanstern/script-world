---
name: game-clock
description: Game time math — 1 tick = 1 game second, epoch day 1 06:00, Speed type mapping game time to real time (1x/4x/8x/16x/32x/max)
kind: component
sources:
  - internal/clock/clock.go
verified_against: 6eb8b60ceb65d760408051eadf50a789603efa18
---

# Game clock

`internal/clock` owns all conversion between ticks, game time, and real time. It is
pure math with no state: the current tick lives in the sim state, not here.

## How it works

Constants: `TickGameSeconds = 1` (fixed for format_version 1, recorded in the world
manifest so it can never silently change under an existing run); `EpochSecondOfDay =
6*3600` — tick 0 is **day 1, 06:00**. Days are 1-based.

`Speed` is a string type with exactly six values: `Speed1x`, `Speed4x` (the
`DefaultSpeed`: 1 game minute per 15 real seconds), `Speed8x`, `Speed16x`,
`Speed32x` (the top of the watchable ladder, TASK-20), `SpeedMax`.
The numeric meaning is game-seconds per real-second; `SpeedMax` maps to 0 as an
"uncapped" sentinel, reserved for pure-sim worlds — [[ipc-server]] refuses it
when an LLM is configured. `ParseSpeed` rejects anything else — validation happens both in
the CLI and in the loop's `set_speed` handling.

Key functions:

- `Speed.TicksPerSecond() float64` — requested tick rate; 0 means uncapped.
- `Speed.Interval() time.Duration` — real-time budget per tick; 0 for max. This is
  what the scheduler in [[sim-loop]] paces against.
- `GameTime(tick) (day, hour, min, sec)` — calendar coordinates via integer division;
  no floats, so no drift over weeks-long runs.
- `SecondOfDay(tick)` — used by the [[executor]] to detect the 22:00 night start
  and 06:00 day start boundaries by exact equality (valid because ticks are whole
  game seconds).
- `Format(tick)` — the display form used everywhere: `"day N HH:MM"`.
- `FormatTOD(sec)` — a second-of-day as `"HH:MM"` (TASK-36; meeting-convention
  hours in prompts, narration, and the charter).
- `TickAt(day, hour, min, sec) int64` — the inverse of `GameTime`: the tick
  ordinal for 1-based-day calendar coordinates (spec 016). Used by the
  [[metatron-miracles]] time-snap doors to translate an operator/angel
  "day N HH:MM" target into the tick the reducer snaps to; it does not judge
  direction — the clock is monotonic and forward-only, and the reducer rejects
  a non-forward target.
- `ParseTimeOfDay(s string) (hour, min int, err error)` — parses a `"HH:MM"`
  label, validating the 24-hour range; pairs with a day number and `TickAt` at
  the miracle snap doors.
- `CappedLadder() []Speed` — the six-value ladder EXCLUDING `SpeedMax`, in
  ascending order (1x…32x), as a fresh copy callers cannot mutate — the
  adaptive-throttle governor's (spec 028) shed/recover notches never touch
  uncapped speed.
- `LadderIndex(s Speed) int` — `s`'s position on the capped ladder (0 = the
  1x floor, 4 = the 32x ceiling), or `-1` when `s` is off the ladder
  (`SpeedMax` or an unparsed value) — the governor's own floor/ceiling checks
  read this rather than re-deriving ladder position.

## Connections

[[sim-loop]] converts `Speed` to a scheduling interval; [[sim-state-reducer]] stores
the current `Speed` and pause flag; the [[executor]] and [[event-types]] use
day/night boundary detection; [[cli-promptworld]] prints `Format` output;
[[metatron-miracles]]'s time-snap doors use `TickAt`/`ParseTimeOfDay` to resolve
a target tick; [[cognition]]'s adaptive-throttle governor walks `CappedLadder`/
`LadderIndex` to shed and recover notches ([[daemon-lifecycle]] samples it).

## Operational notes

A game day is 86,400 ticks; at default 4x that is 6 real hours per game day. Night
(22:00–06:00) is 8 game hours. The clock has no notion of pause or degradation — those
are loop/state concerns. Spec 028 (adaptive throttle) added `CappedLadder`/`LadderIndex`
as read-only ladder helpers over the existing six speed values — the clock still holds
no governor state and computes nothing about debt; the package's earlier "unchanged by
any speed-policy rework" claim held only up to this addition.
