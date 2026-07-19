---
id: TASK-2
title: World daemon & time substrate
status: In Progress
assignee: []
created_date: '2026-07-19 01:13'
updated_date: '2026-07-19 01:22'
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
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Write Spec Kit spec (speckit-specify) for the world daemon & time substrate, grounded in docs/design/grounded-assumptions.md (Time & posture, Stack)
2. Link spec to this task via spec-bridge:link
3. speckit-plan + speckit-tasks to derive design artifacts and ordered tasks
4. Implement on the feature branch (one task, one PR): Go daemon, tick loop, clock/speeds/pause, SQLite event log + snapshots, per-world save dir, attach/detach protocol
5. Verify ACs end-to-end, spec-bridge:sync, open PR
<!-- SECTION:PLAN:END -->
