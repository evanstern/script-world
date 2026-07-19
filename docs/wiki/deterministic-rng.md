---
name: deterministic-rng
description: Stateless randomness — every random decision is a PCG seeded from (world seed, purpose, tick, index), so replay needs no RNG state
kind: pattern
sources:
  - internal/sim/rng.go
verified_against: 08d8c70e23c104a4c61df1749c00cb315f5c643d
---

# Deterministic RNG

script-world has no long-lived random stream. Every random decision constructs a fresh
`math/rand/v2` PCG seeded purely from its coordinates, making randomness a pure
function — the key trick that lets crash recovery and replay work without ever
persisting RNG state.

## How it works

`sim.rngAt(seed uint64, purpose string, tick int64, index int) *rand.Rand`:

- `purpose` (e.g. `"move"`, `"genesis"`) is FNV-64a hashed and XORed into the world
  seed, giving each decision family an independent stream;
- the second PCG seed word mixes `tick` (via the splitmix64 constant
  `0x9e3779b97f4a7c15`) with the entity `index`.

Consequences:

- **Replay-free**: recovery rebuilds state from events; when the loop then re-lives
  quiet ticks, each tick's random decisions regenerate identically because they depend
  only on (seed, purpose, tick, index) — nothing consumed earlier matters.
- **Order-independent**: entities draw from independent generators, so refactoring
  iteration order can't shift anyone's rolls.
- **Seed-sensitive**: different world seeds diverge immediately (tested by
  `TestDifferentSeedsDiverge` in `internal/sim/sim_test.go`).

## Connections

[[placeholder-sim]] draws movement through this; [[sim-state-reducer]]'s genesis
wanderer placement uses purpose `"genesis"`. The pattern is what makes
[[sim-loop]]-level determinism (SC-006) cheap: the [[event-log]] plus the seed is a
complete description of a run.

## Operational notes

Future systems (TASK-4 procgen, TASK-5 executor) should draw randomness the same way —
new purpose tags, never a shared stateful generator — or the replay contract breaks.
Research note R3 in `specs/001-world-daemon/research.md` records the deviation from a
single seeded stream and why.
