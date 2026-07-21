---
id: TASK-43
title: 'World instance manager — ps/start/stop by name, default worlds home'
status: Done
assignee: []
created_date: '2026-07-21 14:01'
updated_date: '2026-07-21 20:26'
labels: []
dependencies: []
ordinal: 37500
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Users lose track of concurrently running world daemons and clobber the shared LLM host. Deliver docker/ollama-style instance management: scriptworld ps lists every running world machine-wide (name, state, pid, tick, game time, speed, LLM on/off); new <worldname> creates in ~/.scriptworld/worlds by default with an explicit-path escape hatch; every per-world command accepts a name or a path (paths keep today's exact behavior). Worlds stay self-contained copyable directories; any manager state is advisory and self-healing, liveness always re-proven from live evidence.

Spec: specs/008-instance-manager
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 scriptworld ps lists every running world machine-wide with name, state, pid, tick, game time, speed, LLM on/off — no false 'running' from stale files
- [x] #2 new <worldname> creates in the default worlds home; explicit paths still work everywhere; worlds remain self-contained copyable directories
- [x] #3 All per-world commands accept a name or a path; path invocations behave exactly as before
- [x] #4 Manager state is advisory and self-healing; a world runs with no manager state present
- [x] #5 Spec phase: Setup
- [x] #6 Spec phase: Foundational (Blocking Prerequisites)
- [x] #7 Spec phase: User Story 1 — See everything that is running (Priority: P1) 🎯 MVP
- [x] #8 Spec phase: User Story 2 — Create and address worlds by name (Priority: P2)
- [x] #9 Spec phase: User Story 3 — Manage custom-path worlds by name (Priority: P3)
- [x] #10 Spec phase: Polish & Cross-Cutting Concerns
<!-- AC:END -->



## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Full Spec Kit: spec+checklist (112deea) → plan/research/data-model/contracts/quickstart (b6ba9cf) → tasks.md 17 tasks (64ac639). Implementation: worktree .worktrees/task-43, branch task-43-instance-manager, one PR. Phases: Foundational internal/worlds pkg → US1 ps (MVP) → US2 names → US3 custom-path → polish. Post-merge: wiki-update (cmd/scriptworld, internal/daemon touched).
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
spec-bridge sync:  — status To Do → In Progress

spec-bridge sync: Setup: 0/1 · Foundational (Blocking Prerequisites): 0/4 · User Story 1 — See everything that is running (Priority: P1) 🎯 MVP: 0/5 · User Story 2 — Create and address worlds by name (Priority: P2): 0/3 · User Story 3 — Manage custom-path worlds by name (Priority: P3): 0/2 · Polish & Cross-Cutting Concerns: 0/2

Model tier (constitution V rubric): Sonnet — new leaf package internal/worlds + CLI plumbing; only concurrency is a bounded fan-out status probe with no shared mutable state, below the governor/scheduler bar reserved for Opus. Escalation one-way to Opus if a slice fails gates.

Implementation complete: spec 008 T001–T017 all done on branch task-43-instance-manager (9 commits). PR #30 open (https://github.com/evanstern/script-world/pull/30), mergeable/clean, awaiting merge (merge action gated to user). Full suite green fresh (-count=1) incl. e2e; quickstart walkthrough 8/8 PASS. Post-merge remaining: wiki-update (cmd/scriptworld, internal/daemon, internal/world touched), spec-bridge sync to Done, worktree cleanup.

spec-bridge sync: Setup: 1/1 · Foundational (Blocking Prerequisites): 4/4 · User Story 1 — See everything that is running (Priority: P1) 🎯 MVP: 5/5 · User Story 2 — Create and address worlds by name (Priority: P2): 3/3 · User Story 3 — Manage custom-path worlds by name (Priority: P3): 2/2 · Polish & Cross-Cutting Concerns: 2/2 — status In Progress → Done
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
All spec tasks complete (Setup: 1/1 · Foundational (Blocking Prerequisites): 4/4 · User Story 1 — See everything that is running (Priority: P1) 🎯 MVP: 5/5 · User Story 2 — Create and address worlds by name (Priority: P2): 3/3 · User Story 3 — Manage custom-path worlds by name (Priority: P3): 2/2 · Polish & Cross-Cutting Concerns: 2/2). Derived Done by spec-bridge sync.
<!-- SECTION:FINAL_SUMMARY:END -->
