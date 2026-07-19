---
id: TASK-7
title: 'Agent mind v1: persona/soul, memory window, planner'
status: In Progress
assignee: []
created_date: '2026-07-19 01:13'
updated_date: '2026-07-19 04:22'
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
- [ ] #1 8 seeded agents plan via local model on cadence + triggers and act through the executor
- [ ] #2 persona.md is structurally read-only; soul.md accretes episodic memories with salience
- [ ] #3 Planner context uses the top-K reranked window, never the whole soul
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Branch task-7-agent-mind stacked on task-6-llm-orchestrator (PR base = task-6 branch until #5 merges)
2. Spec Kit spec (specs/002-agent-mind), spec-bridge link, plan, tasks
3. Implement: 8 seeded personas (persona.md immutable by construction + chmod 0444, soul.md sim-written view); episodic memories as events (agent.memory_added w/ salience) reduced into state; top-K working memory (salience x recency rerank + serendipity tail picks) as a pure sim function; mind driver in the daemon (event-driven replica, 30-game-min staggered cadence + wake/idle/night/encounter triggers) calling the local tier via the orchestrator; planner JSON goals injected into the loop as recorded inject_intent commands (replay never re-calls a model); reflex demoted to idle-grace fallback; scribe writes player-readable soul.md
4. Tests: prompt-window AC (top-K never whole soul), persona immutability, memory accretion + selection determinism, mind integration with mock local LLM (cadence + triggers -> planner-sourced intents -> executor acts), full -race suite
5. Wiki update, board close-out, PR
<!-- SECTION:PLAN:END -->
