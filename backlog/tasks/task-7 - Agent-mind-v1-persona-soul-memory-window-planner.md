---
id: TASK-7
title: 'Agent mind v1: persona/soul, memory window, planner'
status: To Do
assignee: []
created_date: '2026-07-19 01:13'
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
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 8 seeded agents plan via local model on cadence + triggers and act through the executor
- [ ] #2 persona.md is structurally read-only; soul.md accretes episodic memories with salience
- [ ] #3 Planner context uses the top-K reranked window, never the whole soul
<!-- AC:END -->
