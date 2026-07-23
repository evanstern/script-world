---
id: decision-6
title: >-
  Classroom mode: curriculum-staged horizon posture — paused chain-completion
  for authoring, calibrated soft speed cap for ambient running; budgets stay
  doctrine
date: '2026-07-23 16:58'
status: accepted
---
## Context

Classroom-mode design session (client, 2026-07-23; TASK-66, from the 2026-07-22 team
review). A learner's loop is *tweak the charter → speed up → watch the effect*, but the
cognition horizon (decision-4) deterministically suppresses exactly the evidence the
learner sped up to observe. Grounded arithmetic (docs/design/horizon-vs-learner-iteration-speed.md):
the metatron class (5pt/86,400t) never suppresses at watchable speeds — charter edits
always reach the angel; what dies at 16–32× is the downstream villager planner
(3pt/1,200t: suppressed above 20× at bootstrap 20 s/pt, above 16× at 17 s/pt) and
conversation (13pt/7,200t: suppressed at 32× at bootstrap). The pain is
calibration-sensitive (a 12.3 s/pt host clears 32× for both) and today invisible to a
learner (TASK-41 open).

A code-grounded finding reshaped the session: while paused, the operator's mediated
chain *already* works almost end to end — `metatron_chat` has no pause gate
(ipc/server.go:312), the angel's nudges land at the frozen tick (blessed by decision-4),
and the nudge becomes a memory — but it breaks at the last two links: nudges are not on
the planner's wake-stimulus list (mind.go absorb()), and `routeVerdict`
(mind/telemetry.go) computes drift at the world's *set* speed, suppressing a thought
whose real drift while frozen is zero.

## Decision

**Curriculum-staged posture, carried by two mechanisms; per-class staleness-budget
overrides rejected.** Client decisions 2026-07-23 (TASK-66 session, PR #50):

- **Paused authoring sandbox = chain-completion only.** Two fixes, no new mode, no new
  verbs, no single-stepping in v1: (1) a landed nudge arms the nudged villager's one
  planner round at the frozen tick — bounded by construction, since the 300-tick planner
  debounce is game-time and cannot reopen while frozen (the same shape as decision-4's
  blessed catch-up round); (2) pause-aware routing — paused ⇒ predicted drift 0 ≤ any
  budget ⇒ allow. This is not an override of the horizon; it makes the arithmetic tell
  the truth. Doctrine door: decision-4's "no new cognition starts while paused," already
  refined to bless landing-triggered catch-up, extends to landings the operator caused
  via Metatron — pause changes meaning from "the minds are quiet" to "the world is
  frozen, but responds to the angel." Villagers stay sealed; influence stays mediated.
- **Teaching-world speed posture = soft cap, warn-with-override.** Teaching worlds
  default to the highest calibrated planner-safe speed (`calibrate` already computes it
  — horizonSummary, cmd/promptworld/calibrate.go); exceeding it is allowed and surfaces
  the horizon arithmetic. Overriding the cap is itself a lesson about the horizon.
  Derived per world from the calibration profile, never hard-coded — survives spec 024's
  per-provider seconds-per-point divergence.
- **Budget overrides rejected.** Widening BudgetTicks loosens both the router and the
  stale-landing door: learners would study a drift-degraded sim exactly while being
  taught cause→effect. Registry values remain doctrine (decision-4).
- **Staging:** stage 1 of the client's progression (conversational Metatron) needs no
  mechanism — the metatron class never suppresses at watchable speeds. The mechanisms
  above exist for stages 2–3 (charter/tool authoring).

## Consequences

- Follow-up implementation tasks are cut from this decision (TASK-66 AC#4): paused
  chain-completion (doctrine-adjacent — Opus-tier rubric, spec required) and the
  teaching-world soft speed posture (consumed by TASK-68's stage presets; interacts with
  TASK-40's uncalibrated-world warning).
- Horizon legibility in the learner-facing surface (TASK-41) is a prerequisite for
  classroom mode either way: a suppressed planner without a visible verdict reads as
  "the game is broken" — folded into TASK-41's scope, not a new task.
- decision-4 stands unamended; this record is the scoped extension of its pause
  semantics. Replay determinism holds: paused-routing verdicts are reproducible
  arithmetic, and every frozen-tick thought enters the log as recorded events.
- TASK-68's stage presets carry the posture (per-world stage/speed fields), not engine
  rules — the engine still refuses nothing (decision-4: speed is never capped to protect
  cognition; this caps a *teaching posture* to protect feedback legibility).
