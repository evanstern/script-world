---
id: TASK-5
title: 'Executor layer: needs, actions, day/night, death'
status: To Do
assignee: []
created_date: '2026-07-19 01:13'
labels:
  - engine
  - sim
dependencies:
  - TASK-4
ordinal: 5000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Deterministic agent bodies: pathfinding; action primitives (forage, chop, gather, build, hunt, eat, sleep, talk-intent); needs decay (health, food, rest, warmth, morale); day/night cycle; death from collapsed health or starvation. Grounding: grounded-assumptions.md (Agent mind, The world).
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Agents execute multi-step intents unattended between planner calls
- [ ] #2 Needs decay and are satisfiable via world resources; an agent starved of food or health dies
- [ ] #3 Night is mechanically distinct (light/shelter matter)
<!-- AC:END -->
