---
id: TASK-21
title: 'Idle musings: thought-only mind calls between planner turns'
status: Done
assignee: []
created_date: '2026-07-19 22:27'
updated_date: '2026-07-19 23:15'
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
- [x] #1 Agents emit agent.thought (source 'musing') between planner calls — multiple per agent per game day at watchable speeds
- [x] #2 Musing thoughts are recorded events visible in chronicle/souls surfaces and survive replay
- [x] #3 Musings never change goals/intents and yield to planner/conversation demand (best-effort admission), with a bounded fairness floor (~one musing per 2 wall-minutes) so a saturated local tier cannot silence them entirely
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. llm: KindMusing routed TierLocal; best-effort admission — musings are refused (ErrTierBusy) unless both local queues are empty; CLI kind list updated
2. sim/loop: add agent.thought to injectSocialWhitelist (musing door = the existing atomic social batch; thought events are reducer no-ops, chronicle material)
3. mind: per-agent museDue cadence (900 ticks = 15 game-min, staggered between planner slots), single-flight goroutine (never blocks absorb), prompt = musing system + existing userPrompt situation/memory window, parse = one plain line (reject JSON-ish), inject one agent.thought{source: musing}
4. Tests: llm routing + busy-drop; mind end-to-end musing injection + drop-on-error re-arm; suite
5. Wiki: llm-orchestrator, sim-loop, agent-mind notes + re-pin
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Live acceptance (muse-proof, seed 9, 4x, real gemma4:12b-mlx): musings landed at ticks 15149/15798/16554 — e.g. Rowan: 'I can already hear myself arguing with Sage over the best way to keep those fires burning bright.' Pace ~1 per 2 wall-min under full planner saturation = the fairness floor working as designed (~22/agent/game-day at 4x). Live finding folded in: back-to-back ~50s local planner calls admit zero pure best-effort work, hence Request.BestEffort + museStarveWindow. Unit: TestMusingsInjectThoughts (end-to-end through real loop), TestMusingDropsAreSilent, TestMusingBestEffort (drop + starved-bypass + quiet-serve), TestParseMusing. Race-clean; full suite green; wiki re-grounded (agent-mind, llm-orchestrator, sim-loop, event-types + repins).
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Musings shipped: llm KindMusing (local, best-effort Request.BestEffort admission + ErrTierBusy), mind museDue cadence (900 ticks staggered, single-flight, detached), agent.thought whitelisted through inject_social, fairness floor (museStarveWindow 2 wall-min) so tier saturation cannot silence interiority. Live-proven on real gemma at 4x — musings land at floor pace under full planner saturation. PR: https://github.com/evanstern/script-world/pull/12 (stacked on #11).
<!-- SECTION:FINAL_SUMMARY:END -->
