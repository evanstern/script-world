---
id: TASK-28
title: 'Seasons and ambient temperature: design session'
status: To Do
assignee: []
created_date: '2026-07-20 19:54'
updated_date: '2026-07-20 19:55'
labels:
  - design
dependencies: []
ordinal: 23000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Roguelike survival design (user, 2026-07-20; see decision-3 strife doctrine). Today the only environmental cycle is the binary day/night flag: night runs 22:00-06:00 and warmth simply decays -4/min outdoors at night vs +2 by day (docs/wiki/executor.md, game-clock.md). No seasons, no year, no weather. Socratic/spec session covering: (1) Calendar — simple alternating hot/cold seasons layered on the existing clock (internal/clock); season length open, ~10 game-days is attractive because the 30-day proving run (TASK-14) would cross 3 transitions. (2) Ambient temperature — replace the binary night-cold with a deterministic temperature curve, a pure function of (seed, tick): seasonal baseline plus a diurnal swing that troughs pre-dawn (~04:00-05:00) and peaks at 13:00-14:00; warmth decay becomes proportional to the gap between ambient temperature and comfort, with fire/shelter/sleep as modifiers. (3) Seasonal scarcity — cold season slows or stops forage regrowth and thins den yields, so surviving winter requires warm-season stockpiles (the ant-and-grasshopper pressure that feeds TASK-26 storage and spec 006 norms). (4) Candidate extras to debate: seeded cold snaps and storms as shock events (a storm could douse outdoor fires, with TASK-29); longer nights in the cold season. Hard invariant: temperature must be derivable from (seed, tick) with no new persisted state beyond events — replay determinism is test-enforced. Output: a spec under specs/ linked to the board via spec-bridge.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A grounding/design session produces a spec directory for seasons and ambient temperature, linked on the board via spec-bridge
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Pre-session decisions (user, 2026-07-20): (1) Two seasons only — hot and cold, no four-season calendar. (2) Day/night temperature difference is in: temperature drops at night and spikes at 13:00-14:00. (3) Purpose is decision-3: seasons exist to turn the labor-budget screw, not for flavor.
<!-- SECTION:NOTES:END -->
