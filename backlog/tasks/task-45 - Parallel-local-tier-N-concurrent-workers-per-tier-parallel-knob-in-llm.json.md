---
id: TASK-45
title: >-
  Parallel local tier: N concurrent workers per tier ('parallel' knob in
  llm.json)
status: In Progress
assignee: []
created_date: '2026-07-21 14:11'
updated_date: '2026-07-21 15:29'
labels:
  - cognition
  - performance
dependencies: []
priority: high
ordinal: 39000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Implementation slice feeding TASK-35's routing design. Evidence (2026-07-21 live session): internal/llm/llm.go runs exactly ONE worker goroutine per tier — strict serialization is the root of today's queue-wait pathology (130s queue waits behind 19s calls → rejected-stale planners; 'tier busy' musing drops; TASK-44 herd collisions all fight over one slot). Server-side parallelism is already free: measured 4 concurrent cogito:3b calls completing in 0.98s wall vs 3.8s for one cold call — no extra instances needed, one loaded model serves N slots. Change: LocalConfig gains parallel (default 1); Orchestrator.New spawns N workers per tier; best-effort admission counts free slots instead of a single busy bit. Estimator note: samples then measure true concurrent-rate, closing TASK-40's sequential-calibration blind spot for free. Per-class MODEL routing (musing/turns→cogito:3b, planner/narrator→gemma4:12b-mlx) is deliberately NOT in scope here — that's TASK-35's design session; this task is the mechanical unlock it will sit on. Back-of-envelope: chatty classes go from ~20s×1-wide (loaded) to ~1s×4-wide ≈ 50-80x throughput; conversation admission at 32x regains huge margin. Related: TASK-24 (cross-world coordination), TASK-40, TASK-42, TASK-44.

Spec: specs/009-parallel-local-tier
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Spec phase: Setup
- [ ] #2 Spec phase: Foundational (Blocking Prerequisites)
- [ ] #3 Spec phase: User Story 1 — Timely village cognition at speed (Priority: P1) 🎯 MVP
- [ ] #4 Spec phase: User Story 2 — Best-effort thoughts stop losing every race (Priority: P2)
- [ ] #5 Spec phase: User Story 3 — Honest speed governance under concurrency (Priority: P3)
- [ ] #6 Spec phase: Polish & Cross-Cutting Concerns
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Full Spec Kit pipeline complete: spec.md + plan.md + research.md (R1-R8) + data-model.md + contracts/llm-config.md + quickstart.md + tasks.md (13 tasks, T001-T013) in specs/009-parallel-local-tier; linked via spec-bridge. Implementation: worktree .worktrees/task-45, branch task-45-parallel-local-tier, spec-implementer agent at model=opus. Rubric justification (constitution V, v1.1.0): concurrency/scheduling logic in internal/llm is explicitly senior-tier (Opus 4.8). Design core: N copies of existing worker loop (local slots from LocalConfig.Workers(), cloud pinned 1), parallel clamped to [1,16] with boot warning, slot-aware best-effort admission via atomic inflight counter, estimator/health/meter unchanged but proven under -race. Live validation seeds: ~/.scratch/calibration.json + ~/worlds/village03/llm.json with parallel:4 (research R8).
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
PIPELINE STATE (for any session picking this up): speckit-specify COMPLETE — spec at specs/009-parallel-local-tier/spec.md, quality checklist passing, zero NEEDS-CLARIFICATION (the concurrency-cap knob is explicitly delegated to the plan). .specify/feature.json already points at specs/009-parallel-local-tier, so /speckit-plan picks it up directly. NEXT: speckit-plan → speckit-tasks → spec-bridge:link (required before implementation, constitution v1.1.0) → implement in worktree .worktrees/task-45 on branch task-45-<slug> via the spec-implementer agent with model=opus (rubric: concurrency/orchestrator logic in internal/llm — senior tier). No clarify round needed. Related evidence and scope fences are in this task's description and the spec's Assumptions.

speckit-plan + speckit-tasks complete (2026-07-21). spec-bridge:link done: marker + 6 phase ACs seeded, status In Progress. Next: implement T001-T013 in worktree via spec-implementer (opus), then spec-bridge:sync + wiki-update post-merge.
<!-- SECTION:NOTES:END -->
