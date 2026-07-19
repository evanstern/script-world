---
id: TASK-21
title: 'Idle musings: thought-only mind calls between planner turns'
status: In Progress
assignee: []
created_date: '2026-07-19 22:27'
updated_date: '2026-07-19 22:34'
labels:
  - sim
  - llm
dependencies: []
ordinal: 17000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
More idle thoughts across the game day (user request 2026-07-19). A dedicated light local-tier call (new llm kind 'musing', routed local) for agents who are idle or mid-work: emits agent.thought with source 'musing' — pure flavor/interiority, never a goal change. Strictly lowest priority: fires opportunistically on its own per-agent cadence, dropped (not queued) when the local tier is busy, never starves planner or conversation calls. Thoughts land through a thought-only injection door so they are recorded, replayable chronicle material. Depends on TASK-20 pacing decisions only loosely; independent branch.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Agents emit agent.thought (source 'musing') between planner calls — multiple per agent per game day at watchable speeds
- [ ] #2 Musings never set or change intents and never displace planner/conversation calls (dropped when the tier is busy)
- [ ] #3 Musing thoughts are recorded events visible in chronicle/souls surfaces and survive replay
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. llm: KindMusing routed TierLocal; best-effort admission — musings are refused (ErrTierBusy) unless both local queues are empty; CLI kind list updated
2. sim/loop: add agent.thought to injectSocialWhitelist (musing door = the existing atomic social batch; thought events are reducer no-ops, chronicle material)
3. mind: per-agent museDue cadence (900 ticks = 15 game-min, staggered between planner slots), single-flight goroutine (never blocks absorb), prompt = musing system + existing userPrompt situation/memory window, parse = one plain line (reject JSON-ish), inject one agent.thought{source: musing}
4. Tests: llm routing + busy-drop; mind end-to-end musing injection + drop-on-error re-arm; suite
5. Wiki: llm-orchestrator, sim-loop, agent-mind notes + re-pin
<!-- SECTION:PLAN:END -->
