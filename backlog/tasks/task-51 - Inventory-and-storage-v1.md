---
id: TASK-51
title: Inventory and storage v1
status: In Progress
assignee: []
created_date: '2026-07-22 01:42'
updated_date: '2026-07-22 05:17'
labels: []
dependencies:
  - TASK-50
ordinal: 46000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Implement the storage layer specced in specs/013-inventory-storage (from TASK-26's design session): single bulk carry cap (24, everything costs 1), emergent ground piles + stockpile zones from a drop action (no player zoning), death drops, builder-owned finite chests (6 planks, 48 bulk), theft recorded-not-prevented via existing social machinery, food rot on the ground but not in chests. Reflex never touches storage; all storage goals are planner-only. Depends on TASK-50 (planks, fine-grained food).

Spec: specs/013-inventory-storage
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Spec phase: Setup
- [ ] #2 Spec phase: Foundational (Blocking Prerequisites)
- [ ] #3 Spec phase: User Story 1 — Villagers can only carry so much (P1) 🎯 MVP
- [ ] #4 Spec phase: User Story 2 — Ground piles and emergent stockpiles (P2)
- [ ] #5 Spec phase: User Story 3 — Chests: the village learns to keep things (P3)
- [ ] #6 Spec phase: User Story 4 — Theft is a story, not an error (P4)
- [ ] #7 Spec phase: User Story 5 — Rot: the ground is not a larder (P5)
- [ ] #8 Spec phase: Migration & Format Door (Cross-Cutting)
- [ ] #9 Spec phase: Polish & Cross-Cutting
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Full Spec Kit run complete (specs/013-inventory-storage: plan, research R1-R9, data-model, contracts/events, quickstart, tasks T001-T041). Constitution Check PASS pre-Phase-0 and post-Phase-1.

Shape: bulk cap 24 derived (never stored); Piles as event-sourced overlay state with per-batch rot deadlines; chest = Structure extension (Owner + Store, cap 48, 6 planks); theft = companion event batch through existing relation/memory/rumor machinery (social.chest_taken record + relation_changed reason "theft" + owner/witness memories); death spill reducer-internal on agent.died; rot sweep on the minute heartbeat; format bump 2->3 with people-preserving NO-land-reset migration (over-cap carry spills to a pile).

Execution: one branch task-51-inventory-storage in .worktrees/task-51, stories sequential US1->US5 then migration then polish, one PR.

Tier decision (constitution V, rubric in research R9): Opus 4.8 spec-implementer for Phases 2-6 and 8 — substrate/reducer/determinism surface, executor completion semantics, social-machinery coupling, migration (doctrine-adjacent; where live defects concentrated historically). Sonnet spec-implementer for marked TUI + planner-vocabulary tasks (T015, T021/T022, T026/T027, T034) — single-package view/prompt work.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
spec-bridge sync: Setup: 0/2 · Foundational (Blocking Prerequisites): 0/7 · User Story 1 — Villagers can only carry so much (P1) 🎯 MVP: 0/6 · User Story 2 — Ground piles and emergent stockpiles (P2): 0/7 · User Story 3 — Chests: the village learns to keep things (P3): 0/5 · User Story 4 — Theft is a story, not an error (P4): 0/4 · User Story 5 — Rot: the ground is not a larder (P5): 0/3 · Migration & Format Door (Cross-Cutting): 0/3 · Polish & Cross-Cutting: 0/4
<!-- SECTION:NOTES:END -->
