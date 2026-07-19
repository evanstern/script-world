---
id: TASK-5
title: 'Executor layer: needs, actions, day/night, death'
status: In Progress
assignee: []
created_date: '2026-07-19 01:13'
updated_date: '2026-07-19 03:47'
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

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Branch task-5-executor stacked on task-4-procgen-map (PR base = task-4 branch until #3 merges)
2. Replace placeholder wanderers with Agents: named bodies, integer needs (health/food/rest/warmth/morale, 0-1000 scale — integer math for cross-platform determinism), inventory (wood/food)
3. Executor in stepEvents: per-game-minute needs decay + death (starvation/exposure); intent execution — BFS next-hop pathing (1 tile/5 ticks), timed work (forage/chop/build/hunt), eat/sleep/wake/talk primitives; all mutations evented through the reducer
4. Reflex policy (deterministic, replaces LLM planner until TASK-7): eat when hungry, forage/hunt for food, chop wood, build campfire+shelter before night, sleep warm at night
5. Night mechanics: warmth drains outdoors at night, recovers near fire/in shelter; health drains at zero food or zero warmth -> agent.died
6. Dynamic terrain overlays in state: chopped trees, harvested forage (regrows), structures; TUI renders structures + needs in souls pane
7. Tests: determinism harness re-proven, multi-step intent chains in log (AC#1), starvation death (AC#2), night warmth differential (AC#3); full -race suite
8. Wiki-update, PR, board close-out
<!-- SECTION:PLAN:END -->
