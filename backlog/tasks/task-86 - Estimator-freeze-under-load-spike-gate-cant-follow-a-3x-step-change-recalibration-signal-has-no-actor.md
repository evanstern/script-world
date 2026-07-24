---
id: TASK-86
title: >-
  Estimator freeze under load: spike gate can't follow a >3x step change;
  recalibration signal has no actor
status: In Progress
assignee: []
created_date: '2026-07-24 03:18'
updated_date: '2026-07-24 03:31'
labels:
  - cognition
  - bug
dependencies: []
references:
  - internal/cognition/estimate.go
  - specs/007-cognition-horizon/contracts/calibration.md
priority: high
ordinal: 74000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
FOUND (world-01, 2026-07-23, 8x-32x run): calibrate seeded gemma at 0.52 s/pt against an idle endpoint; under live 8x load planner tool-loops actually ran 21-50s (~7-17 s/pt). Every live sample exceeded SpikeFactor(3.0) x estimate (~1.6 s/pt), so Estimator.Sample (internal/cognition/estimate.go:74) excluded 100% of samples from the EWMA — the estimate froze at the seed forever. Evidence in world-01 world.db: 81 cog.recalibration_recommended events, gemma spike_rate 1.0 with estimate pinned at 0.524; cog.outcome rows show predicted_wall_ms 1573 vs actual_wall_ms 21256-50348.

CASCADE of the frozen estimate: (1) Route predicted ~13 ticks of drift vs a 1200-tick planner budget, so the router admitted every thought — the suppression layer was disarmed; (2) the landing door then rejected what the router admitted: since the 8x+ window (tick 263390+), 17 of 31 planner thoughts landed rejected-stale (avg predicted 1.6s, actual 36.9s, staleness ~1493t > 1200 budget), plus 930 lifetime rejected-guard 'world-change' outcomes from plans landing against a moved world; (3) governor debt also keys off the same frozen prediction (see companion governor task).

ROOT CAUSE: spike rejection conflates MAGNITUDE with PERSISTENCE. A step change larger than SpikeFactor is indistinguishable from an endless run of one-shot spikes, so the doctrine 'one-shot lag spikes rejected, systemic drift followed' is violated exactly in the systemic case it promises to follow — and load-induced slowdown is always a step change. The breach signal (cog.recalibration_recommended) fires but has no actor: nothing adapts, nothing acts.

CANDIDATE FIXES (design decision, doctrine-adjacent to specs/007 contracts/calibration.md):
(A) RECOMMENDED — breach-adoption: store sample VALUES in the existing WindowSize ring (not just spike booleans); when the rolling spike rate breaches BreachRate over a full window, re-seed the estimate to the window median and reset. The breach detector already IS the lag-vs-systemic classifier (1-2 spikes in 20 never breach; sustained load always does) — this makes it the actor instead of an unread signal. Deterministic, small diff, preserves spike rejection for genuine one-shots.
(B) Clamped feed: always feed min(sample, SpikeFactor x estimate) into the EWMA so the estimate can walk up under sustained overload. Simpler, but weakens one-shot rejection (a single clamped spike moves the estimate +24% at alpha 0.2) — a tuning tradeoff rather than a classification.
(C) Rolling-median estimator: replace the EWMA with a window median — robust to <=50 percent outliers and follows steps within ~half a window, but discards the EWMA doctrine entirely.

DECISION (user, 2026-07-24): option A — breach-adoption. Specified in specs/031-estimator-breach-adoption (spec, plan, research, data-model, event contract, tasks).

RELATED: TASK-40 logs the sibling bias (calibrate measures SEQUENTIAL latency, runtime load is CONCURRENT — queue wait invisible to prediction). Fixing the estimator to follow live load makes the calibration seed only a starting point, which also softens TASK-40's failure mode. Companion defect: TASK-87 (governor overdue-blindness).

Spec: specs/031-estimator-breach-adoption
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A sustained step change in observed sec/pt (>3x the current estimate) is followed by the live estimate within one full window (20 samples) — no permanent freeze; regression test reproduces the world-01 shape (seed 0.52, sustained ~12 s/pt samples) and asserts the estimate converges
- [ ] #2 Genuine one-shot spikes (1-2 spikes within a window) still barely move the estimate — existing spike-rejection tests stay green or are consciously retuned
- [ ] #3 The recalibration/adoption behavior is visible in telemetry (event emitted when the estimator re-seeds or adapts) and the wiki cognition note + specs/007 calibration contract are updated to the new doctrine
- [ ] #4 Router admission returns truthful: in a saturated-load scenario predicted drift tracks actual within the spike factor, so suppression fires before landing-door rejection dominates
- [ ] #5 Spec phase: Setup
- [ ] #6 Spec phase: Foundational (Blocking Prerequisites)
- [ ] #7 Spec phase: User Story 1 — The estimate follows a sustained slowdown (Priority: P1) 🎯 MVP
- [ ] #8 Spec phase: User Story 2 — One-shot lag spikes are still rejected (Priority: P2)
- [ ] #9 Spec phase: User Story 3 — Adoption is auditable (Priority: P3)
- [ ] #10 Spec phase: Polish & Cross-Cutting Concerns
<!-- AC:END -->
