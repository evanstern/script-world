---
id: TASK-65
title: >-
  Operator identity and attribution groundwork (deferred pending multiplayer
  decision)
status: To Do
assignee: []
created_date: '2026-07-23 03:27'
labels:
  - review-2026-07-22
  - teaching-game
  - deferred
dependencies: []
priority: low
ordinal: 58000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
From the 2026-07-22 team review (new-ideas item 3, RESCOPED by client answers 2026-07-22): the original "one agent per coworker" idea (each player authors one villager persona) is retired — the client ruled villagers stay sealed; indirect influence via the angel is the entire point. The client also said single-player on-laptop is the likely v1 posture, with multiplayer (self-host / modest paid hosting) undecided.

What survives is the groundwork both multiplayer shapes need: identity and attribution. Today event provenance is Source: "planner"/"meeting"/"metatron" with no operator identity anywhere; IPC sessions are anonymous (ipc/server.go:205 — no name, no id); and the cost meter is one global monthly ceiling with no per-operator attribution. If multiplayer ever happens — parallel villages on a shared host, or one shared village with one angel console per coworker — every design needs "whose prompt caused this" answerable from the log, and per-operator spend/charge accounting.

Scope when activated: (a) optional operator identity on IPC sessions and threaded through commands into event provenance; (b) per-operator attribution on metatron turns, nudges, miracles, and LLM spend; (c) design note choosing the multiplayer shape (parallel villages vs shared village with per-player angels) — the shape decision gates everything else. DELIBERATELY DEFERRED: do not start until the client picks a multiplayer direction; single-player v1 does not need this. Kept on the board so the decision has a durable home.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Multiplayer shape decision recorded (parallel villages vs shared village with per-player angels) with client sign-off
- [ ] #2 IPC sessions can carry an operator identity; events caused by operator input record it in provenance
- [ ] #3 Metatron turns, nudges, miracles, and LLM spend are attributable per operator
- [ ] #4 Anonymous/identity-less operation still works unchanged for single-player worlds
<!-- AC:END -->
