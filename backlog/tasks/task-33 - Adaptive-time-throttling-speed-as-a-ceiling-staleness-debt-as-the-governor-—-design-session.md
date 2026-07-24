---
id: TASK-33
title: >-
  Adaptive time throttling: speed as a ceiling, staleness debt as the governor —
  design session
status: Done
assignee: []
created_date: '2026-07-20 20:48'
updated_date: '2026-07-24 02:04'
labels:
  - design
dependencies:
  - TASK-32
ordinal: 11000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Split from TASK-32 (user, 2026-07-20). PROBLEM — even with the cognition horizon scoping LLM authority by speed, there are moments where the player wants high speed AND high thought fidelity (a crisis unfolding at 32x). Rather than forcing a manual speed drop, the loop could govern itself. CANDIDATE DESIGN — the speed setting becomes a CEILING, not a promise: the sim tracks aggregate in-flight staleness debt (sum over pending planner/conversation jobs of predicted game-time drift, using TASK-32's calibrated seconds-per-point and telemetry) and sheds a speed notch when debt exceeds a budget, recovering when it drains. A feedback controller over a measurable signal — RimWorld-style adaptive time, but driven by cognition load instead of frame rate. QUESTIONS for the session: shed policy (notch-down vs proportional vs micro-pause at decision-critical moments); hysteresis so speed doesn't oscillate; whether debt is global or per-agent-weighted by salience; interaction with the existing SpeedMax-refused-with-LLM rule (does adaptive throttle subsume it?); how the TUI communicates 'you asked for 32x, running at 16x because 3 minds are in flight'; determinism boundary — throttling changes tick pacing (wall-side, like pause) and must never change tick CONTENT, so replay stays byte-identical. DEPENDS on TASK-32: needs the staleness telemetry, seconds-per-point calibration, and debt definition to exist before a governor can act on them. Output: a spec under specs/ linked via spec-bridge.

Spec: specs/028-adaptive-throttle
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 A design session produces a spec directory for the adaptive time throttle, linked on the board via spec-bridge
- [x] #2 The spec resolves all six session questions: shed policy, hysteresis, debt scoping, SpeedMax interaction, TUI communication, determinism boundary
- [x] #3 Spec phase: Setup
- [x] #4 Spec phase: Foundational (Blocking Prerequisites)
- [x] #5 Spec phase: User Story 1 — Debt is measured and visible (Priority: P1) 🎯 MVP
- [x] #6 Spec phase: User Story 2 — The world sheds speed under debt (Priority: P2)
- [x] #7 Spec phase: User Story 3 — Speed recovers without oscillating (Priority: P3)
- [x] #8 Spec phase: User Story 4 — The player sees it and stays in charge (Priority: P4)
- [x] #9 Spec phase: Polish & Cross-Cutting Concerns
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1) Ground in TASK-32 outputs: specs/007-cognition-horizon (telemetry, calibration, debt definition), internal/cognition code, docs/wiki notes. 2) Cut worktree .worktrees/task-33 (branch task-33-adaptive-throttle) from fresh origin/main. 3) Design session: resolve the enumerated questions (shed policy, hysteresis, debt scoping, SpeedMax interaction, TUI surface, determinism boundary) — user decisions via clarify where artifacts don't already answer. 4) speckit-specify the adaptive-throttle spec (028). 5) spec-bridge:link to TASK-33; ACs, sync, commit, PR.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Re-grounding 2026-07-22: dependency TASK-32 is Done — the staleness/sec-per-point telemetry and calibration this design needs now exist (internal/cognition). Unblocked.

Session start 2026-07-23: began design session. GitHub #33 is an unrelated merged PR (TASK-51); this is board TASK-33.

Design session decisions (user, 2026-07-23): (1) SHED POLICY — notch-down on the existing six-value speed ladder (32x→16x→8x→4x→1x), one notch per breach window, notch-by-notch recovery as debt drains; proportional pacing and auto micro-pause rejected (ladder legibility wins; micro-pause may return as a future opt-in). (2) DEBT SCOPE — global sum: one world-level debt = Σ predicted game-tick drift over all in-flight/queued planner+conversation jobs (from orchestrator queue+inflight × estimator sec/pt × ticksPerSecond); salience weighting deferred until telemetry shows need. (3) SPEEDMAX — refusal with LLM configured retained unchanged (spec 007 assumption stands); governor governs only the capped ladder, floor 1x. Doctrine-answered (not re-asked): determinism boundary — governing is wall-side pacing like pause; sheds/recoveries land as recorded clock.* events per the auto-slow precedent, tick CONTENT never changes, replay byte-identical. Hysteresis: asymmetric (fast shed, slow recover) with distinct thresholds+windows, spec'd not asked.

spec-bridge sync: Setup: 0/1 · Foundational (Blocking Prerequisites): 0/2 · User Story 1 — Debt is measured and visible (Priority: P1) 🎯 MVP: 0/3 · User Story 2 — The world sheds speed under debt (Priority: P2): 0/5 · User Story 3 — Speed recovers without oscillating (Priority: P3): 0/2 · User Story 4 — The player sees it and stays in charge (Priority: P4): 0/2 · Polish & Cross-Cutting Concerns: 0/3

spec-bridge sync: Setup: 1/1 · Foundational (Blocking Prerequisites): 2/2 · User Story 1 — Debt is measured and visible (Priority: P1) 🎯 MVP: 0/3 · User Story 2 — The world sheds speed under debt (Priority: P2): 0/5 · User Story 3 — Speed recovers without oscillating (Priority: P3): 0/2 · User Story 4 — The player sees it and stays in charge (Priority: P4): 0/2 · Polish & Cross-Cutting Concerns: 0/3

Implementation dispatch (2026-07-23): tier ruling per constitution V rubric — T002 (cognition debt+constants), T003 (llm job registry, concurrency), T004-T006 (daemon sampler goroutine + status), T007-T011 (sim state/loop/reducer/replay + controller shed), T012-T013 (recovery/hysteresis), T015 (pause/override proofs) → Opus 4.8 (governor/scheduling/concurrency logic, cross-package, doctrine-adjacent). T014 (TUI render), T016-T017 (validation runs) → Sonnet (view code, mechanical gates). Executing via spec-implementer agents on .worktrees/task-33; planning model gates each phase. [Re-recorded: original note landed in the worktree board copy by mistake and was discarded.]

spec-bridge sync: Setup: 1/1 · Foundational (Blocking Prerequisites): 2/2 · User Story 1 — Debt is measured and visible (Priority: P1) 🎯 MVP: 3/3 · User Story 2 — The world sheds speed under debt (Priority: P2): 0/5 · User Story 3 — Speed recovers without oscillating (Priority: P3): 0/2 · User Story 4 — The player sees it and stays in charge (Priority: P4): 0/2 · Polish & Cross-Cutting Concerns: 0/3

spec-bridge sync: Setup: 1/1 · Foundational (Blocking Prerequisites): 2/2 · User Story 1 — Debt is measured and visible (Priority: P1) 🎯 MVP: 3/3 · User Story 2 — The world sheds speed under debt (Priority: P2): 5/5 · User Story 3 — Speed recovers without oscillating (Priority: P3): 0/2 · User Story 4 — The player sees it and stays in charge (Priority: P4): 0/2 · Polish & Cross-Cutting Concerns: 0/3

spec-bridge sync: Setup: 1/1 · Foundational (Blocking Prerequisites): 2/2 · User Story 1 — Debt is measured and visible (Priority: P1) 🎯 MVP: 3/3 · User Story 2 — The world sheds speed under debt (Priority: P2): 5/5 · User Story 3 — Speed recovers without oscillating (Priority: P3): 2/2 · User Story 4 — The player sees it and stays in charge (Priority: P4): 0/2 · Polish & Cross-Cutting Concerns: 0/3

spec-bridge sync: Setup: 1/1 · Foundational (Blocking Prerequisites): 2/2 · User Story 1 — Debt is measured and visible (Priority: P1) 🎯 MVP: 3/3 · User Story 2 — The world sheds speed under debt (Priority: P2): 5/5 · User Story 3 — Speed recovers without oscillating (Priority: P3): 2/2 · User Story 4 — The player sees it and stays in charge (Priority: P4): 2/2 · Polish & Cross-Cutting Concerns: 0/3

Implementation complete (2026-07-23): all four stories + T016/T017 polish proven; PR #55 open (https://github.com/evanstern/promptworld/pull/55) — 15 commits on task-33-adaptive-throttle. Live validation captured real shed {32x→16x, debt 1.29, jobs 5} and recovery on gemma4:12b-mlx; full suite + replay byte-identity + no-format-bump gates green. Remaining before Done: merge PR, then T018 (wiki re-pin + player-docs + final sync + worktree cleanup). Follow-up filed: TASK-83 (pre-existing gofmt drift found by the T017 gate).

spec-bridge sync: Setup: 1/1 · Foundational (Blocking Prerequisites): 2/2 · User Story 1 — Debt is measured and visible (Priority: P1) 🎯 MVP: 3/3 · User Story 2 — The world sheds speed under debt (Priority: P2): 5/5 · User Story 3 — Speed recovers without oscillating (Priority: P3): 2/2 · User Story 4 — The player sees it and stays in charge (Priority: P4): 2/2 · Polish & Cross-Cutting Concerns: 3/3 — status In Progress → Done
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
All spec tasks complete (Setup: 1/1 · Foundational (Blocking Prerequisites): 2/2 · User Story 1 — Debt is measured and visible (Priority: P1) 🎯 MVP: 3/3 · User Story 2 — The world sheds speed under debt (Priority: P2): 5/5 · User Story 3 — Speed recovers without oscillating (Priority: P3): 2/2 · User Story 4 — The player sees it and stays in charge (Priority: P4): 2/2 · Polish & Cross-Cutting Concerns: 3/3). Derived Done by spec-bridge sync.
<!-- SECTION:FINAL_SUMMARY:END -->
