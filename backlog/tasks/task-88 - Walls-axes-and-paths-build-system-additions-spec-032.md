---
id: TASK-88
title: 'Walls, axes, and paths: build-system additions (spec 032)'
status: In Progress
assignee: []
created_date: '2026-07-24 04:07'
updated_date: '2026-07-24 04:07'
labels: []
dependencies: []
ordinal: 75000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Villagers gain movement-blocking, repairable walls (plank/stone, HP + demolish verb), an axe tool gating chop/quarry yields (1 bare / 3 with axe), and stone paths giving exactly 2x walking speed — all deterministic, planner-reachable, event-sourced.

Spec: specs/032-walls-axes-paths
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Spec phase: Setup
- [ ] #2 Spec phase: Foundational (Blocking Prerequisites)
- [ ] #3 Spec phase: User Story 1 - Walls shape the world (Priority: P1) 🎯 MVP
- [ ] #4 Spec phase: User Story 2 - Axes make harvesting worthwhile (Priority: P2)
- [ ] #5 Spec phase: User Story 3 - Paths speed travel (Priority: P3)
- [ ] #6 Spec phase: Polish & Cross-Cutting
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Full Spec Kit run complete: spec.md (+clarifications 2026-07-23: demolish verb; speed-only paths), plan.md, research.md R1-R8, data-model.md, contracts/{recipes,events}.md, quickstart.md, tasks.md (23 tasks / 6 phases). Implement in .worktrees/task-88 on branch task-88-walls-axes-paths, one PR.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Model tier: Opus 4.8 (spec-implementer via Agent model param). Rubric justification (constitution V): cross-package slice (internal/sim + internal/tool + internal/tui); changes core movement/pathability semantics (passable(), movement cadence dual-phase gate); introduces new executor scheduling shape (WorkStart-reset multi-cycle work for demolish/repair). Not a routine single-package slice.
<!-- SECTION:NOTES:END -->
