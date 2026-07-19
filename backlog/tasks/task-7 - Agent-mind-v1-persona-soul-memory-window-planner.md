---
id: TASK-7
title: 'Agent mind v1: persona/soul, memory window, planner'
status: Done
assignee: []
created_date: '2026-07-19 01:13'
updated_date: '2026-07-19 04:42'
labels:
  - spec-candidate
  - agents
dependencies:
  - TASK-5
  - TASK-6
ordinal: 7000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The thinking layer: persona.md (immutable, never in any write path) + soul.md (sim-written, player-readable) per agent per run; top-K working memory (reverse-chron, cheap rerank, serendipity mix from the tail); planner calls on 30-game-min cadence + scene-change triggers producing structured intents for the executor. Grounding: grounded-assumptions.md (Agent mind). Spec candidate #2.

Spec: specs/002-agent-mind
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 8 seeded agents plan via local model on cadence + triggers and act through the executor
- [x] #2 persona.md is structurally read-only; soul.md accretes episodic memories with salience
- [x] #3 Planner context uses the top-K reranked window, never the whole soul
- [x] #4 Spec phase: Setup
- [x] #5 Spec phase: Foundational (blocking)
- [x] #6 Spec phase: US2 — personas & souls (files first: US1's prompts need them)
- [x] #7 Spec phase: US1 + US3 — the mind driver
- [x] #8 Spec phase: Polish
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Branch task-7-agent-mind stacked on task-6-llm-orchestrator (PR base = task-6 branch until #5 merges)
2. Spec Kit spec (specs/002-agent-mind), spec-bridge link, plan, tasks
3. Implement: 8 seeded personas (persona.md immutable by construction + chmod 0444, soul.md sim-written view); episodic memories as events (agent.memory_added w/ salience) reduced into state; top-K working memory (salience x recency rerank + serendipity tail picks) as a pure sim function; mind driver in the daemon (event-driven replica, 30-game-min staggered cadence + wake/idle/night/encounter triggers) calling the local tier via the orchestrator; planner JSON goals injected into the loop as recorded inject_intent commands (replay never re-calls a model); reflex demoted to idle-grace fallback; scribe writes player-readable soul.md
4. Tests: prompt-window AC (top-K never whole soul), persona immutability, memory accretion + selection determinism, mind integration with mock local LLM (cadence + triggers -> planner-sourced intents -> executor acts), full -race suite
5. Wiki update, board close-out, PR
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
spec-bridge sync: Setup: 0/1 · Foundational (blocking): 0/6 · US2 — personas & souls (files first: US1's prompts need them): 0/4 · US1 + US3 — the mind driver: 0/6 · Polish: 0/3

AC evidence: #1 TestPlannerDrivesAgents + live Ollama run (8 seeded agents, planner-sourced intents on cadence+triggers, executor acts; goal spread forage 9 / chop 2 / build_fire 2 / goto_warmth 1 in the first game-hour; Hazel's persona visibly steering her stated reasons). #2 persona mode 0444 + no post-genesis write path + shasum identical across a run AND a daemon restart; souls accrete dated salience-starred memories (Sage: 'day 1 06:19 (5★) Built a fire.') and survive restart (event-sourced). #3 TestWindowBound + TestPromptWindowBound (150-memory soul → ≤10 prompt lines) + deterministic selection incl. bucketed serendipity. Replay is model-free: TestInjectedPlannerIntent state-hash equality. -race green, 10 packages. Known gap noted: mind replica can drop batches at max speed (resync future work).

spec-bridge sync: Setup: 1/1 · Foundational (blocking): 6/6 · US2 — personas & souls (files first: US1's prompts need them): 4/4 · US1 + US3 — the mind driver: 6/6 · Polish: 3/3 — status In Progress → Done
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
All spec tasks complete (Setup: 1/1 · Foundational (blocking): 6/6 · US2 — personas & souls (files first: US1's prompts need them): 4/4 · US1 + US3 — the mind driver: 6/6 · Polish: 3/3). Derived Done by spec-bridge sync.
<!-- SECTION:FINAL_SUMMARY:END -->
