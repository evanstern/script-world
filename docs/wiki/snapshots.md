---
name: snapshots
description: Hash-verified state snapshots bounding recovery replay — cadence (hourly/pause/shutdown), fallback chain, prune to 24
kind: component
sources:
  - internal/store/store.go
  - internal/sim/loop.go
  - internal/daemon/daemon.go
verified_against: 0cfc04adc5ea41bc9c35442f137e9e5d60763e17
---

# Snapshots

Snapshots are serialized reducer state at a known (tick, seq), used only to bound how
much of the [[event-log]] recovery must replay. They are an optimization, never
authority: the log wins on any conflict, and all snapshots can be discarded at the
cost of replay-from-genesis.

## How it works

Storage: `snapshots(id, tick, seq, state, state_hash, wall_time)` where `state` is the
canonical JSON of `sim.State` and `state_hash` its SHA-256 (`store.stateHash`).

Cadence, all driven from [[sim-loop]]:

- every `SnapshotEveryTicks = 3600` ticks (1 game hour) inside `runTick`;
- on every successful **pause** command (so stop-while-paused resumes instantly);
- on shutdown via `finalSnapshot` when the loop's context is canceled.

`Store.SaveSnapshot` writes; `Store.PruneSnapshots(24)` keeps the newest 24 (pruning
touches only `snapshots`, never `events`). `Store.LatestValidSnapshot` walks newest →
oldest and returns the first whose `state_hash` verifies — a corrupt newest snapshot
silently falls back to an older one, and if none survive, recovery starts from genesis
state (`sim.NewState(seed, map)`).

Recovery in `daemon.recoverState`: unmarshal the chosen snapshot into the state, then
replay events with `seq > snapshot.seq` through the same `Apply` reducer the live loop
uses, bumping `state.Tick` to the highest event tick seen.

## Connections

[[sim-loop]] produces snapshots; [[daemon-lifecycle]] consumes them at startup;
[[sim-state-reducer]]'s canonical marshal is what gets hashed; [[event-log]] remains
the truth they accelerate.

## Operational notes

Cadence bounds recovery to ≤1 game hour of events (≈3,600 quiet ticks or a few dozen
rows); measured recovery after kill -9 across 95k events was 18 ms
(`specs/001-world-daemon/quickstart-results.md`). Because quiet ticks log nothing, a
crash can lose up to one snapshot interval of *silent* clock progress — never any
recorded event; the resumed clock is `max(snapshot tick, last event tick)`.
