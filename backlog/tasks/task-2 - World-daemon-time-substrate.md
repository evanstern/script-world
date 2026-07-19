---
id: TASK-2
title: World daemon & time substrate
status: To Do
assignee: []
created_date: '2026-07-19 01:13'
labels:
  - spec-candidate
  - engine
dependencies: []
ordinal: 2000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Go daemon skeleton for the always-on world: deterministic tick loop; game clock at default 1 game-min = 15 real-sec with speed range real-time up to as-fast-as-affordable; pause as a first-class verb; SQLite append-only event log + snapshots; per-world save directory (per-run flat files, cleanly separable runs); client attach/detach protocol. Grounding: docs/design/grounded-assumptions.md (Time & posture, Stack). Spec candidate #1 — write the Spec Kit spec before implementing.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Daemon runs detached 24/7; TUI client can attach/detach without stopping the world
- [ ] #2 Pause/speed controls work over the client protocol; default speed is 4x compression
- [ ] #3 Every sim event lands in the SQLite event log; world resumes from snapshot+log after restart
<!-- AC:END -->
