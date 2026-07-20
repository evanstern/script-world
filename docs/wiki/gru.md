---
name: gru
description: The nocturnal sight-triggered predator — an event-sourced entity that wounds but never kills; fire light and shelter are absolute safety; encounters seed rumors and omens
kind: component
sources:
  - internal/sim/gru.go
verified_against: be54bb42adcbd14421c20269efc79da7b6beab9f
---

# The gru

The gru (TASK-10) makes night dangerous the Zork way. It is an **entity, not a
phenomenon** — a positioned body in event-sourced state (`State.Gru`, nil while it
is not abroad) — because sight-triggering needs geometry, the TUI needs something
to render, and rumors need something to have been seen. Its design contract from
the grounding session: it *wounds*; death stays with neglect
(starvation/exposure/collapse), which the wound merely feeds.

## How it works

**Lifecycle**: at 22:00 a seeded per-night roll (`rngAt("gru-emerge")`,
`gruEmergePerMille = 600`) decides whether it comes out; if so it slips in from a
seeded passable, unlit border tile (`gru.emerged{night, x, y}`). At 06:00 it is
gone (`gru.withdrew{day}`, state nil). Every decision is a pure function of
(seed, night/tick) — [[deterministic-rng]] — so the whole predator replays.

**Sight**: it sees live agents within Manhattan `gruSightRadius` (8) — **unless
they are protected**. Protection is fire light (`gruLightRadius = 3`, strictly
wider than `warmAt`'s fire radius 2, so a warm agent is always a safe agent) or
standing on a shelter tile. The gru also never steps into protected tiles, so it
visibly circles the firelight. Protection is absolute, not probabilistic.

**Movement** (`gru.moved{x, y}`, one tile per `gruMoveEveryTicks = 4`, slightly
faster than agents' 5): greedy chase toward the nearest visible agent (ties to
the lowest index), seeded prowl when nobody is visible. Deliberately greedy
rather than BFS — a monster that can be baffled by water and firelight is the
right monster.

**Attack** (`gru.attacked{agent, health}`): adjacent + visible + a
10-game-minute cooldown (`gruAttackCooldown = 600`). The payload carries the
**absolute post-wound health** (outcome convention), a `gruWound = 250` drop
floored at `gruWoundFloor = 1` — the gru is never the proximate cause of an
`agent.died`. The reducer arm (`applyGru`) wakes the victim and clears their
intent, handing them to [[reflex-policy]], which at night flees to warmth — the
night curfew, emergent rather than scripted. The heartbeat's near-death memory
names "the gru" as the cause when the last wound was recent (`LastVictim` /
`LastAttack` on the `Gru` struct).

**Story fuel**: the victim keeps a salience-9 memory; awake witnesses within
`witnessRadius` keep a subject-tagged, tone-negative memory about the victim
(salience 7 ≥ `rumorMinSalience`), which [[social-fabric]]'s `TellableFor` serves
as gossip — a witnessed attack becomes a village-wide rumor with mutating
confidence. Any awake agent within sight range — safe ones by the fire included —
gets one `gru.sighted{agent, x, y}` plus an omen memory per night (a `Seen`
bitmask on the `Gru` struct latches it). [[event-types]] catalogs the family.

## Connections

[[executor]] calls `gruStep` from `stepEvents` (same purity contract);
[[sim-state-reducer]] dispatches `gru.*` to `applyGru`; [[reflex-policy]] supplies
the flee-to-warmth response; [[social-fabric]] turns witness memories into rumors;
[[tui-client]] renders it as a red G; [[worldmap-generation]] bounds its spawn
border.

## Operational notes

Live proving (seed 42, 1257 game days): the gru emerged ~60% of nights, attacked
186 times, never killed (zero deaths in the whole run — every wound left its
victim at 750), emitted zero events outside 22:00–06:00, and a witnessed attack
on Fern propagated as a rumor through the entire village with confidence decaying
80 → 35. Sightings are personal (subject −1) and thus omen material, not gossip;
only *witnessed attacks* seed rumors, which makes gru rumors appropriately rare.
