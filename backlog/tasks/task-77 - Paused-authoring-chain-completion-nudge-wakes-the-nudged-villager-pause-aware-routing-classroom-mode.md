---
id: TASK-77
title: >-
  Paused authoring chain-completion: nudge wakes the nudged villager +
  pause-aware routing (classroom mode)
status: To Do
assignee: []
created_date: '2026-07-23 17:00'
updated_date: '2026-07-24 02:42'
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
ordinal: 8000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Follow-up cut from TASK-66 / decision-6 (client decision 2026-07-23). While paused, the operator's mediated chain already works up to the nudge becoming a memory: metatron_chat has no pause gate (ipc/server.go:312), the angel's landed effects inject at the frozen tick (blessed by decision-4), and the nudge lands as a SalDream memory. It breaks at the last two links, and this task is exactly those two fixes — no new mode, no new verbs, no single-stepping (explicitly deferred by the client):

(1) Wake: a landed nudge arms the nudged villager's planner for one round at the frozen tick — add the nudge event to absorb()'s arm switch (internal/mind/mind.go:203-228). Bounded by construction: the 300-tick planner debounce is game-time and cannot reopen while frozen, so one nudge buys exactly one round (same shape as decision-4's blessed catch-up round).

(2) Truth: pause-aware routing — routeVerdict (internal/mind/telemetry.go:61-71) computes drift at the world's SET speed even while frozen, suppressing a thought whose real drift is zero. Paused ⇒ predicted drift 0 ≤ any budget ⇒ allow; the recorded arithmetic string should say the world was paused. Not an override of the horizon — it makes the arithmetic tell the truth.

Doctrine door (decision-6): extends decision-4's landing-triggered catch-up blessing to landings the operator caused via Metatron — pause changes meaning from 'the minds are quiet' to 'the world is frozen, but responds to the angel.' Villagers stay sealed; influence stays mediated. Replay determinism holds: paused verdicts are reproducible arithmetic; frozen-tick thoughts enter the log as recorded events.

DOCTRINE-ADJACENT BEHAVIOR CHANGE in internal/mind — Opus 4.8 rubric tier per constitution V; full Spec Kit (specify → plan → tasks → spec-bridge:link) before implementation. The learner loop it buys: pause → edit charter → 'Metatron, nudge Aldric' → watch Aldric's one thought land under the new charter → resume. Diagram from the design session: docs/design/horizon-vs-learner-iteration-speed.md.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A landed nudge arms the nudged villager's planner for exactly one bounded round at the frozen tick; the debounce still prevents any second round while frozen
- [ ] #2 routeVerdict treats a paused world as zero predicted drift (allow); the verdict's recorded arithmetic names the paused state
- [ ] #3 Frozen-tick thoughts land at zero staleness, fully recorded (cog.thought/cog.outcome); replay determinism harness green
- [ ] #4 Unpaused behavior is byte-identical to today: no new wake stimuli or routing changes apply while running
- [ ] #5 Spec Kit spec written and linked via spec-bridge before implementation (doctrine-adjacent, non-trivial)
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Drift audit 2026-07-23: pins re-verified; one moved — metatron_chat handler (no pause gate) is ipc/server.go:334-355, not :312 (:312 is llm_call). absorb() arm switch mind.go:206-228 and routeVerdict telemetry.go:61-71 hold exactly.
<!-- SECTION:NOTES:END -->
