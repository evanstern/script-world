---
id: TASK-5
title: 'Executor layer: needs, actions, day/night, death'
status: Done
assignee: []
created_date: '2026-07-19 01:13'
updated_date: '2026-07-19 03:59'
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
- [x] #1 Agents execute multi-step intents unattended between planner calls
- [x] #2 Needs decay and are satisfiable via world resources; an agent starved of food or health dies
- [x] #3 Night is mechanically distinct (light/shelter matter)
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

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented on branch task-5-executor (PR will target main — TASK-4 merged as a true merge commit, verified ancestry). internal/sim executor layer: 4 named agents, integer needs (0-1000, cross-platform deterministic), multi-step intents with BFS pathing and timed work, reflex survival policy (permanent degraded-mode fallback), night warmth mechanics, death by starvation/exposure/collapse, event-sourced terrain overlays (cleared trees, forage regrowth, den cooldowns, fire/shelter structures). AC#1 proven by TestMultiStepIntentExecution (full intent chains, zero input); AC#2 by TestNeedsDecayAndSatisfaction + TestStarvationDeath (cause recorded, dead stay dead); AC#3 by TestNightWarmthMechanics (cold drains, fire restores, exposure kills). Determinism + replay re-proven over the executor (30-40k tick harnesses); TestVillageSurvivesTwoDays green on seeds 42+7. Live-run found and fixed a sleep/wake churn bug (fully-rested agents at night); known quirk noted: several agents may build fires in the same construction window. -race suite green incl fresh e2e. Wiki: placeholder-sim removed, executor + reflex-policy notes added, gate green (20 notes).
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Executor layer shipped: deterministic agent bodies that survive unattended — needs decay with lethal floors, multi-step intents (forage/chop/hunt/build/eat/sleep) over BFS pathfinding, a reflex survival policy that builds fires before the first night, mechanically distinct nights (warmth/fire/shelter), and death with recorded causes. All three ACs test-proven; village survives two full days on multiple seeds; substrate determinism guarantees re-proven over the whole layer.
<!-- SECTION:FINAL_SUMMARY:END -->
