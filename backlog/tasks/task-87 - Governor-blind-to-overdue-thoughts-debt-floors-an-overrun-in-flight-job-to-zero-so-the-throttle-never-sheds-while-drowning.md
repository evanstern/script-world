---
id: TASK-87
title: >-
  Governor blind to overdue thoughts: debt floors an overrun in-flight job to
  zero, so the throttle never sheds while drowning
status: In Progress
assignee: []
created_date: '2026-07-24 03:19'
updated_date: '2026-07-24 04:05'
labels:
  - cognition
  - bug
dependencies:
  - TASK-86
references:
  - internal/cognition/governor.go
  - specs/028-adaptive-throttle
priority: high
ordinal: 5250
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
FOUND (world-01, 2026-07-23, same run as TASK-86): ZERO clock.governor_shed events in world.db despite 17 of 31 planner thoughts landing rejected-stale during the 8x-32x window. Debt (internal/cognition/governor.go:69) is Sigma max(0, PredictedSec - ElapsedSec) x tps / BudgetTicks: the max(0, ...) floor means a job whose elapsed time exceeds its prediction contributes ZERO debt. With predictions frozen low (TASK-86) every in-flight job went 'overdue' within ~2 wall seconds and vanished from the debt sum — so the moments of WORST drift produced the LEAST debt, inverting the throttle's purpose exactly when it was needed. This defect stands alone even with a healthy estimator: any single stuck call (endpoint hang, model swap-in) disappears from debt the moment elapsed > predicted, and a fleet of stuck calls reads as debt ~0.

DOCTRINE GAP: spec 028's 'an overdue thought invents no debt it cannot ground' treats the overrun as unknowable — but the overrun is a MEASUREMENT, not an invention: drift already accrued (elapsed x tps) is real staleness the reply will land with at minimum. A job guaranteed dead-on-arrival (accrued drift alone > budget) currently counts as zero.

CANDIDATE FIXES:
(A) RECOMMENDED, near one-liner — per-job fraction = max(PredictedSec, ElapsedSec) x tps / BudgetTicks, i.e. accrued drift plus predicted remaining. Overdue jobs then contribute their true, growing, fully grounded drift; jobs within prediction behave exactly as today. Pure arithmetic, deterministic, no new inputs.
(B) Complement — rejection-grounded breach: feed landing outcomes to the governor; a rejected-stale landing counts as an immediate breach sample (or injects 1.0 debt for one window). Ground truth from the injection door, independent of any estimator.
Either way the change is doctrine (spec 028 FR-001/FR-002, research R5) — reviewed spec update, not a knob.

ALSO VERIFY FIRST (cheap): world-01's running daemon binary was built 19:23 Jul 23 but the task-33 merge landed 21:53 — confirm the deployed binary even contains the governor (rebuild + restart), and confirm clock.governor_* events appear in a deliberately saturated run before/after the fix.

Depends on TASK-86 only softly: correct debt with a frozen estimator still under-counts (predicted remaining stays tiny), so the full protection story needs both; fix A here is still correct and testable standalone.

DECISION (user, 2026-07-24): option A. Specified in specs/033-governor-accrued-debt — with one planning-tier correction: plain max(Predicted, Elapsed) would break within-prediction behavior (today's arm counts REMAINING work, which drains); the normative arm is piecewise — remaining while elapsed < predicted, full accrued elapsed once overdue (research.md R1). Option B (rejection-grounded breach) recorded as future hardening, out of scope for 033.

Spec: specs/033-governor-accrued-debt
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 An in-flight job whose ElapsedSec exceeds PredictedSec contributes non-decreasing, grounded debt (unit test: stuck job's fraction grows with elapsed, never drops to zero while pending)
- [ ] #2 Saturation scenario test: sustained 20-50s planner calls at 8x with optimistic predictions drive debt past ShedThreshold and the governor sheds within BreachWindow (reproduces world-01's zero-shed failure as a red test first)
- [ ] #3 A live or e2e saturated run shows clock.governor_shed firing and effective speed stepping down the capped ladder; status/TUI reflects governed state
- [ ] #4 specs/028-adaptive-throttle doctrine (FR-001/FR-002 debt definition) and the wiki governor/cognition notes updated to the accrued-drift definition; running-binary-predates-merge check recorded on this task
- [ ] #5 Spec phase: Setup
- [ ] #6 Spec phase: Foundational (Blocking Prerequisites)
- [ ] #7 Spec phase: User Story 1 — Overdue thoughts contribute their true, growing drift (Priority: P1) 🎯 MVP
- [ ] #8 Spec phase: User Story 2 — The fix is provably live in a real run (Priority: P2)
- [ ] #9 Spec phase: Polish & Cross-Cutting Concerns
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Tier decision (constitution Principle V): implementation delegated to spec-implementer on OPUS 4.8 — governor logic in internal/cognition is an explicitly named senior-tier slice (scheduling/governor, doctrine-adjacent). Spec 033 authored, linked, phases seeded.
<!-- SECTION:NOTES:END -->
