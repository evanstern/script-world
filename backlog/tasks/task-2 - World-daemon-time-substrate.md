---
id: TASK-2
title: World daemon & time substrate
status: In Progress
assignee: []
created_date: '2026-07-19 01:13'
updated_date: '2026-07-19 01:27'
labels:
  - spec-candidate
  - engine
dependencies: []
ordinal: 2000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Go daemon skeleton for the always-on world: deterministic tick loop; game clock at default 1 game-min = 15 real-sec with speed range real-time up to as-fast-as-affordable; pause as a first-class verb; SQLite append-only event log + snapshots; per-world save directory (per-run flat files, cleanly separable runs); client attach/detach protocol. Grounding: docs/design/grounded-assumptions.md (Time & posture, Stack). Spec candidate #1 — write the Spec Kit spec before implementing.

Spec: specs/001-world-daemon
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Daemon runs detached 24/7; TUI client can attach/detach without stopping the world
- [ ] #2 Pause/speed controls work over the client protocol; default speed is 4x compression
- [ ] #3 Every sim event lands in the SQLite event log; world resumes from snapshot+log after restart
- [ ] #4 Spec phase: Setup (Shared Infrastructure)
- [ ] #5 Spec phase: Foundational (Blocking Prerequisites)
- [ ] #6 Spec phase: User Story 1 — The world runs without me (Priority: P1) 🎯 MVP
- [ ] #7 Spec phase: User Story 2 — Time is a dial (Priority: P2)
- [ ] #8 Spec phase: User Story 3 — Nothing is ever lost (Priority: P3)
- [ ] #9 Spec phase: Polish & Cross-Cutting Concerns
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Write Spec Kit spec (speckit-specify) for the world daemon & time substrate, grounded in docs/design/grounded-assumptions.md (Time & posture, Stack)
2. Link spec to this task via spec-bridge:link
3. speckit-plan + speckit-tasks to derive design artifacts and ordered tasks
4. Implement on the feature branch (one task, one PR): Go daemon, tick loop, clock/speeds/pause, SQLite event log + snapshots, per-world save dir, attach/detach protocol
5. Verify ACs end-to-end, spec-bridge:sync, open PR
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
spec-bridge sync: Setup (Shared Infrastructure): 0/3 · Foundational (Blocking Prerequisites): 0/9 · User Story 1 — The world runs without me (Priority: P1) 🎯 MVP: 0/7 · User Story 2 — Time is a dial (Priority: P2): 0/4 · User Story 3 — Nothing is ever lost (Priority: P3): 0/4 · Polish & Cross-Cutting Concerns: 0/3
<!-- SECTION:NOTES:END -->
