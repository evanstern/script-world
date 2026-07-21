---
id: TASK-39
title: >-
  Investigate: did recent PRs regress TASK-32 cognition-horizon behavior? (fewer
  conversations, speed issues)
status: Done
assignee: []
created_date: '2026-07-21 13:40'
updated_date: '2026-07-21 13:46'
labels:
  - investigation
dependencies: []
ordinal: 33000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
User-reported symptoms: conversations rarer than expected, speed-related issues. Live evidence: at 32x every conversation job suppressed by governor (13pt x ~25s/pt x 32 > 7200-tick budget); under LLM contention at 4x-16x one conversation scheduled and was abandoned at turn 3 with an empty utterance; planner 'context deadline exceeded' under load. Hypotheses: H1 working-as-designed (horizon doctrine suppressing at high speed, a tuning/visibility issue), H2 regression in a post-TASK-32 PR (cognition/mind/sim), H3 environmental (contention-inflated latency estimates; check persistence/decay). Delegated to Opus investigation agent (read-only PR/commit archaeology + implementation map). Related: TASK-32, specs/007-cognition-horizon. Also checking whether suppression has any user-facing surface (feeds TUI follow-up noted on TASK-34).
<!-- SECTION:DESCRIPTION:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Verdict: NOT a regression — TASK-32's cognition horizon is working as designed; the symptom is operational. Root cause: worlds were never calibrated, so they boot on BootstrapLocalSecPerPt=20.0 (internal/cognition/estimate.go) while this hardware's real measured speed is 0.94 s/pt (task37-verify/calibration.json) — 20x pessimistic. Conversations (13pt, 7200-tick budget) need s/pt <= 17.3 at 32x, so uncalibrated worlds suppress them above ~27x; under LLM contention the live EWMA drifted to 24.5, suppressing harder. PR archaeology (PRs #21-#24 + TASK-34): zero post-TASK-32 changes to budgets, registry constants (byte-identical to contracts/registry.md), estimator, cadence, or abandonment. The abandoned-at-turn-3-empty-utterance failure is TASK-8-era all-or-nothing fragility (never had a retry) under a starved tier, aggravated possibly by TASK-37's max_tokens=128 if the MLX endpoint ignores reasoning_effort:none. Estimator does NOT poison permanently (process-lifetime EWMA, spike rejection, negative feedback, re-seeded from profile at boot). UX gap confirmed: suppression is invisible outside raw cog.outcome payloads, and the narrator itself is router-gated so the story feed goes quiet with no explanation. Fix: scriptworld calibrate <world> (being run on the demo world now to prove the number). Follow-ups spawned as tasks: bootstrap/warn UX, live horizon surface in TUI, utterance retry robustness.
<!-- SECTION:FINAL_SUMMARY:END -->
