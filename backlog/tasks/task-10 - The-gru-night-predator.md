---
id: TASK-10
title: 'The gru: night predator'
status: In Progress
assignee: []
created_date: '2026-07-19 01:14'
updated_date: '2026-07-19 21:11'
labels:
  - sim
dependencies:
  - TASK-5
ordinal: 10000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Nocturnal, sight-triggered predator that wounds (health damage feeding death-by-neglect; it does not execute). Light and shelter are safety; spawn/movement rules and entity-vs-phenomenon to be designed in-task. Feeds shelter economy, night curfew pressure, rumor/omen material. Grounding: grounded-assumptions.md (The world).
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 The gru hunts only at night and only harms agents it can see
- [ ] #2 Light/shelter reliably protect; wounded agents lose health, not instant death
- [ ] #3 Gru encounters emit events usable as rumors and omens
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Design (decided in-task per description):
ENTITY, not phenomenon — the gru is a positioned body in State (State.Gru, event-sourced, omitempty so old snapshots stay valid). Sight-triggered implies geometry; a position gives the TUI something to render and rumors something to point at.

Mechanics (all deterministic, outcome-payload events; stepEvents stays pure):
1. Emergence: per-night seeded roll (rngAt "gru-emerge", NightIndex) at 22:00; spawns on a seeded passable, unlit border tile → gru.emerged{night,x,y}. Withdraws at 06:00 → gru.withdrew{day}.
2. Sight: sees live agents within Manhattan 8 UNLESS protected. Protection = fire light (radius 3 > warm radius 2, so warm agents are always safe) or standing on a shelter. Light/shelter reliably protect (AC2).
3. Movement: 1 tile per 4 ticks (slightly faster than agents' 5) via BFS toward nearest visible agent (tie: lowest index); seeded prowl when none visible → gru.moved{x,y}.
4. Attack: adjacent + visible + 10-game-min cooldown → gru.attacked{agent,health} with ABSOLUTE post-wound health (wound 250, floored at 1 — the gru wounds, neglect kills: AC1/AC2). Reducer wakes victim, clears intent (reflex then flees to warmth — emergent curfew), sets health.
5. Story fuel (AC3): victim memory sal 9; witnesses within 8 get subject=victim tone-negative memory (sal ≥4 ⇒ TellableFor rumor seed); awake agents within 8 get once-per-night gru.sighted{agent,x,y} + personal omen memory (seen-bitmask latch in Gru state).

Steps: branch task-10-gru off 001-world-daemon → gru.go (state/behavior) + reducer cases + TUI glyph → unit tests (nocturnality, protection, wound-not-execute, rumor-seed) → live proving run at max speed (grep gru.* events, tick ACs from evidence) → wiki-update (executor, event-types, sim-loop notes) → PR.
<!-- SECTION:PLAN:END -->
