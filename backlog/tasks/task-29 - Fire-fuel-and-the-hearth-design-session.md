---
id: TASK-29
title: 'Fire fuel and the hearth: design session'
status: To Do
assignee: []
created_date: '2026-07-20 19:54'
updated_date: '2026-07-21 14:12'
labels:
  - design
dependencies: []
ordinal: 25000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Roguelike survival design (user, 2026-07-20; see decision-3 strife doctrine). Fires today are permanent: build once (10 game-min, 2 wood) and the structure radiates warmth (radius 2) and gru-repelling light (radius 3) forever (docs/wiki/executor.md, gru.md). Socratic/spec session to make fire a consumable hearth: (1) Fuel — every fire carries a fuel level and burns roughly 1 wood per game-hour; at zero fuel it goes out (fire.extinguished event), leaving a cold firepit that is cheaper to relight than to rebuild. An 8-hour night therefore costs ~8 wood per fire — the single biggest wood sink in the economy and the reason a shared village hearth beats private fires. (2) Stoking — a new add-fuel action wired through the whole stack: planner goal enum (internal/mind/prompt.go, parse.go), reflex policy ladder (policy.go), intent duration, reducer event. Who gets up at 03:00 to feed the fire is a deliberate coordination problem — fire-duty rosters are exactly what spec 006 norms/votes should end up legislating. (3) Build cost — raising a new fire takes ~1 game-hour of work, up from 10 minutes. (4) Candidate extras to debate: cooking at a lit fire (cooked food worth more than raw, making the hearth the social center); storms/rain dousing outdoor fires (with TASK-28 weather); whether shelters shield fires. Cross-links: Fire is one of the five v1 items in TASK-25 — that spec should defer to this one for fire behavior; TASK-26 stockpiles interact (communal wood piles by the hearth, and their theft). Output: a spec under specs/ linked to the board via spec-bridge.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A grounding/design session produces a spec directory for fire fuel and burnout, linked on the board via spec-bridge
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Pre-session decisions (user, 2026-07-20): (1) Fire burns out without constant fuel — 1 wood per unit-of-time (working number: 1 wood/game-hour). (2) Building a fire costs about 1 hour of effort. (3) The fuel bill is the cooperation lever per decision-3: shared fires divide it, private fires multiply it.
<!-- SECTION:NOTES:END -->
