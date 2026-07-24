---
id: TASK-40
title: >-
  Calibration UX: uncalibrated worlds silently over-suppress at speed —
  warn/auto-suggest calibrate; revisit bootstrap default
status: To Do
assignee: []
created_date: '2026-07-21 13:47'
updated_date: '2026-07-24 02:42'
labels:
  - ux
dependencies: []
priority: high
ordinal: 2000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
From TASK-39: BootstrapLocalSecPerPt=20.0 is ~20x slower than this rig's measured 0.94 s/pt, so every uncalibrated world silently loses conversations (above ~27x) and planners at high speed, with no signal. Options per TASK-39: lower the bootstrap, or make high-speed launch of an uncalibrated world warn loudly / auto-suggest scriptworld calibrate. Pessimism-toward-reflex is intentional doctrine (decision-4) — changing the default is a doctrine-adjacent call, review against specs/007-cognition-horizon.
<!-- SECTION:DESCRIPTION:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Live finding (demo world, post-calibration): scriptworld calibrate measures SEQUENTIAL latency (8.1 s/pt on this rig) but runtime load is CONCURRENT — 8 agents' jobs queue on one tier and effective rate ran ~24 s/pt (musing predicted 12.1s, actual 24.4s; planner landed 2422 ticks stale on a 19s call because ~130s was queue wait). The router predicts from per-call latency and cannot see queue depth, so post-restart thundering herds and high-speed cadence saturate the tier and produce rejected-stale landings the prediction said were safe (TASK-32 idea E, deferred by 007). Calibration UX should either measure under representative concurrency, inflate by a concurrency factor, or the router should account for queue depth. Interim operational guidance: with 8 agents on one local gemma, 8x is the comfortable ceiling (planner 576/1200, conversation 2496/7200 at load-rate); 16x puts planners on the edge (1152/1200).

Drift audit 2026-07-23: verified intact — BootstrapLocalSecPerPt=20.0 at internal/cognition/estimate.go:14; only a generic (not speed-gated) calibrate reminder at daemon.go:160; promptworld calibrate exists (main.go:81, calibrate.go); wiki cognition.md:77 agrees.
<!-- SECTION:NOTES:END -->
