---
id: TASK-67
title: 'World forking and what-if A/B runs (same village, two prompts, two stories)'
status: To Do
assignee: []
created_date: '2026-07-23 03:28'
updated_date: '2026-07-24 02:42'
labels:
  - review-2026-07-22
  - teaching-game
dependencies: []
priority: medium
ordinal: 16000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
From the 2026-07-22 team review (new-ideas item 5) — the killer teaching feature. Replay is model-free (LLM outputs are recorded inputs), so you cannot re-run yesterday under a new prompt. But the persistence substrate makes world FORKING cheap: save dirs are fully self-contained and copyable (proven by e2e scenario: copied worlds run), snapshots bound recovery, and each world is its own daemon. Fork the world at a point, diverge the charter/skills, run both live, and compare the chronicles: the most direct way a learner SEES what their prompt change did.

Scope: (a) promptworld fork <world> <new-name> [--at latest-snapshot] — copy the save dir truncated to a chosen snapshot boundary, assign a fresh world identity (name registration, socket, any world-scoped ids) so both run side by side; document the semantics of forking mid-log vs at-snapshot (simplest v1: latest snapshot only). (b) Lineage recorded in the fork (parent world + fork tick) as an event/metadata so provenance is durable. (c) A comparison surface: v1 can be CLI — promptworld compare <a> <b> [--since tick] rendering the two chronicles side-by-side or interleaved with divergence markers; a TUI view can come later. (d) Doctrine note: forks are independent worlds afterward (no merging, ever). Design question to settle in spec: does the fork inherit the LLM budget meter or get its own (interacts with the global monthly ceiling — review flagged cost attribution as coarse).

Depends on nothing, but pairs naturally with the decision-trace view (TASK-63): trace explains one run, fork contrasts two.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 promptworld fork creates a runnable copy at the latest snapshot with fresh identity; both worlds run simultaneously
- [ ] #2 Fork lineage (parent, fork tick) durably recorded in the new world
- [ ] #3 A compare surface renders two chronicles against each other with divergence visible
- [ ] #4 Forked world passes the determinism harness independently (replay to identical hash)
- [ ] #5 Budget-meter semantics for forks decided and documented in the spec
- [ ] #6 Spec Kit spec written and linked via spec-bridge before implementation (non-trivial task)
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Drift audit 2026-07-23: premises verified — save dirs self-contained/copyable (world-save-directory.md:15-16), snapshots bound recovery (snapshots.md:14-17), replay never re-calls a model (llm-orchestrator.md:20), and no fork/compare subcommand exists yet (main.go:52-88).
<!-- SECTION:NOTES:END -->
