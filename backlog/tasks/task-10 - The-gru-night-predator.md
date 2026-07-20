---
id: TASK-10
title: 'The gru: night predator'
status: Done
assignee: []
created_date: '2026-07-19 01:14'
updated_date: '2026-07-19 22:00'
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
- [x] #1 The gru hunts only at night and only harms agents it can see
- [x] #2 Light/shelter reliably protect; wounded agents lose health, not instant death
- [x] #3 Gru encounters emit events usable as rumors and omens
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

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Live acceptance (world gru-proof, seed 42, max speed, 1257 game days / 42.5M events) + unit suite:

AC1 nocturnal & sight-triggered: 0 gru.* events outside the 22:00–06:00 window across the whole log (gru-night-violations-all=0); emergence/withdrawal exactly at boundaries. Sight-gating: TestGruLightAndShelterProtect — an agent in firelight or on a shelter is never attacked and the gru never steps into protected tiles.
AC2 protection & wound-not-execute: all 186 gru.attacked left victims at health 750 (1000−250 wound, absolute payload); agent.died count over 1257 days: ZERO — the gru never killed anyone. Floor (health ≥1, never a proximate death) proven by TestGruWoundsNotExecutes; cooldown proven same test.
AC3 rumor/omen fuel: night-2 chain in the log: gru.emerged→gru.sighted→memory "Saw the gru prowling in the dark" (sal 6, omen)→gru.attacked→"tore into me" (sal 9). 50 witnessed-attack memories (subject=victim, tone −60, sal 7) seeded rumors: 10,735 social.rumor_told carrying gru text, e.g. "Saw the gru attack Fern in the dark" propagating 3→6→7→0 with confidence 80→64→44→35. TellableFor path proven by TestGruStoryFuel.

Full suite green including determinism/replay/e2e. Discovered during proving (filed separately): IPC state reply exceeds maxLineBytes (1MB) once memories grow unbounded on long runs without TASK-9 consolidation — TUI/attach clients get bufio ErrTooLong.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
The gru shipped as an event-sourced entity (State.Gru): seeded nightly emergence from unlit border tiles at 22:00, withdrawal at 06:00; sight radius 8 with absolute protection from fire light (radius 3 > warmth 2) and shelter tiles, which the gru also never enters; greedy stalk + seeded prowl at 1 tile/4 ticks; adjacent wounds of 250 health on a 10-min cooldown, floored at 1 — wounds feed death-by-neglect, never execute. Victims wake and flee to warmth via the reflex (emergent curfew); witnessed attacks seed subject-tagged rumors served by TellableFor; sightings latch one omen memory per agent per night. Proven by 4 unit tests + full suite (determinism/replay/e2e) and a 1257-game-day live run: 0 daytime gru events, 186 non-lethal attacks, 0 deaths, village-wide gru rumor propagation. Wiki re-grounded (new gru note + 4 notes re-pinned). PR: https://github.com/evanstern/script-world/pull/10. Side discovery filed as TASK-19 (IPC 1MB state-line cap).
<!-- SECTION:FINAL_SUMMARY:END -->
