---
id: TASK-40
title: >-
  Calibration UX: uncalibrated worlds silently over-suppress at speed —
  warn/auto-suggest calibrate; revisit bootstrap default
status: In Progress
assignee: []
created_date: '2026-07-21 13:47'
updated_date: '2026-07-24 13:57'
labels:
  - ux
dependencies: []
priority: high
ordinal: 2000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
From TASK-39: BootstrapLocalSecPerPt=20.0 is ~20x slower than this rig's measured 0.94 s/pt, so every uncalibrated world silently loses conversations (above ~27x) and planners at high speed, with no signal. Options per TASK-39: lower the bootstrap, or make high-speed launch of an uncalibrated world warn loudly / auto-suggest scriptworld calibrate. Pessimism-toward-reflex is intentional doctrine (decision-4) — changing the default is a doctrine-adjacent call, review against specs/007-cognition-horizon.

Spec: specs/035-calibration-ux
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Spec phase: Setup
- [x] #2 Spec phase: Foundational (Blocking Prerequisites)
- [x] #3 Spec phase: User Story 1 - Raising speed on an uncalibrated world warns loudly (Priority: P1) 🎯 MVP
- [x] #4 Spec phase: User Story 2 - Boot warning states the concrete consequence (Priority: P2)
- [x] #5 Spec phase: User Story 3 - Calibration state visible in status (Priority: P3)
- [x] #6 Spec phase: User Story 4 - Calibrate discloses its sequential-measurement bias (Priority: P3)
- [ ] #7 Spec phase: Polish & Cross-Cutting Concerns
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Spec Kit flow complete: specs/035-calibration-ux (spec + plan + research + data-model + contracts/warnings.md + quickstart + tasks.md, 17 tasks / 7 phases). Doctrine review in spec closes the 'revisit bootstrap default' question: pessimistic bootstrap stands (decision-4). Implementation: one worktree .worktrees/task-40, branch task-40-calibration-ux, one PR; T001–T015 delegated to spec-implementer; T016–T017 (wiki re-pin + player-docs freshness) run at root after merge, then final sync to Done.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Live finding (demo world, post-calibration): scriptworld calibrate measures SEQUENTIAL latency (8.1 s/pt on this rig) but runtime load is CONCURRENT — 8 agents' jobs queue on one tier and effective rate ran ~24 s/pt (musing predicted 12.1s, actual 24.4s; planner landed 2422 ticks stale on a 19s call because ~130s was queue wait). The router predicts from per-call latency and cannot see queue depth, so post-restart thundering herds and high-speed cadence saturate the tier and produce rejected-stale landings the prediction said were safe (TASK-32 idea E, deferred by 007). Calibration UX should either measure under representative concurrency, inflate by a concurrency factor, or the router should account for queue depth. Interim operational guidance: with 8 agents on one local gemma, 8x is the comfortable ceiling (planner 576/1200, conversation 2496/7200 at load-rate); 16x puts planners on the edge (1152/1200).

Drift audit 2026-07-23: verified intact — BootstrapLocalSecPerPt=20.0 at internal/cognition/estimate.go:14; only a generic (not speed-gated) calibrate reminder at daemon.go:160; promptworld calibrate exists (main.go:81, calibrate.go); wiki cognition.md:77 agrees.

Cross-ref 2026-07-24: TASK-86 (estimator freeze under load — spike gate can't follow a >3x step, world-01 evidence) and TASK-87 (governor debt floors overdue jobs to zero) are the live-defect siblings of this task's sequential-vs-concurrent calibration bias. If TASK-86 lands (live estimator follows load), the calibration seed becomes just a starting point and this task reduces to pure UX (warn on uncalibrated + concurrency disclosure).

Model-tier decision (constitution V rubric): SONNET (default tier). Justification: pure UX/visibility slice — additive omitempty wire fields, CLI rendering, boot-output composition, moving existing horizonSummary arithmetic into internal/cognition unchanged; no concurrency/scheduling/governor logic modified (the two internal/llm touches are additive bookkeeping on existing structs). Escalate to Opus 4.8 only if gates fail.

spec-bridge sync: Setup: 1/1 · Foundational (Blocking Prerequisites): 4/4 · User Story 1 - Raising speed on an uncalibrated world warns loudly (Priority: P1) 🎯 MVP: 4/4 · User Story 2 - Boot warning states the concrete consequence (Priority: P2): 2/2 · User Story 3 - Calibration state visible in status (Priority: P3): 2/2 · User Story 4 - Calibrate discloses its sequential-measurement bias (Priority: P3): 1/1 · Polish & Cross-Cutting Concerns: 1/3

PR opened: https://github.com/evanstern/promptworld/pull/61 (branch task-40-calibration-ux, rebased on 9f38b9a). Full suite green incl. e2e; quickstart-results.md recorded. Remaining after merge: T016 wiki re-pin, T017 player-docs freshness, spec-bridge sync to Done, worktree cleanup.
<!-- SECTION:NOTES:END -->
