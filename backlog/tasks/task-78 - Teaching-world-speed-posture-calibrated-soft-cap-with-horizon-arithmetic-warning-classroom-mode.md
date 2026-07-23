---
id: TASK-78
title: >-
  Teaching-world speed posture: calibrated soft cap with horizon-arithmetic
  warning (classroom mode)
status: To Do
assignee: []
created_date: '2026-07-23 17:00'
labels:
  - teaching-game
  - classroom-mode
dependencies: []
references:
  - >-
    backlog/decisions/decision-6 -
    Classroom-mode-curriculum-staged-horizon-posture-—-paused-chain-completion-for-authoring-calibrated-soft-speed-cap-for-ambient-running-budgets-stay-doctrine.md
  - docs/design/horizon-vs-learner-iteration-speed.md
priority: medium
ordinal: 71000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Follow-up cut from TASK-66 / decision-6 (client decision 2026-07-23). Teaching-mode worlds default their speed to the highest calibrated planner-safe rung of the ladder — the number calibrate already computes (horizonSummary, cmd/promptworld/calibrate.go:173-197: 'planner suppressed above 16x'). SOFT posture, decided explicitly over a hard cap: exceeding the default is allowed and surfaces the horizon arithmetic (e.g. '3pt × 17.0s/pt × 32x = 1632 ticks > budget 1200 — villagers will stop deep-thinking'), so overriding the cap is itself a lesson about the horizon.

Shape (decision-6): a per-world teaching/config posture consumed by TASK-68's stage presets — NOT an engine rule (decision-4 stands: the engine never caps speed to protect cognition; this caps a teaching posture to protect feedback legibility). Derived per world from the calibration profile at world creation/speed-change time, never hard-coded — must survive spec 024's per-provider seconds-per-point divergence (recompute from the profile the planner class actually routes to).

Interactions: TASK-40 (uncalibrated worlds silently over-suppress — an uncalibrated teaching world must prompt calibrate before the posture can be honest; bootstrap 20 s/pt would cap at 16x pessimistically); TASK-68 (stage presets carry the posture field); TASK-41 (horizon legibility in the TUI is the always-on counterpart of the one-shot warning). Stage 1 worlds (conversational Metatron) need no posture — the metatron class never suppresses at watchable speeds.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Teaching worlds default to the highest calibrated planner-safe ladder speed, derived from the world's calibration profile (not hard-coded)
- [ ] #2 Setting a speed above the posture succeeds and surfaces the horizon arithmetic for the classes it suppresses
- [ ] #3 An uncalibrated teaching world prompts for calibrate rather than silently adopting the pessimistic bootstrap cap (aligns with TASK-40)
- [ ] #4 Posture lives as per-world config consumable by TASK-68 stage presets; non-teaching worlds are unchanged
<!-- AC:END -->
