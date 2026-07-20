---
id: TASK-32
title: >-
  The cognition horizon: LLM latency vs time compression, and pause semantics —
  design session
status: In Progress
assignee: []
created_date: '2026-07-20 20:30'
updated_date: '2026-07-20 22:20'
labels:
  - design
dependencies: []
priority: high
ordinal: 27000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Fundamental substrate design (user, 2026-07-20). PROBLEM — Stimulus -> LLM turn -> action treats thought as instant, but a local planner call runs ~50s wall (live finding, docs/wiki/agent-mind.md). Latency in GAME time scales with speed: 50s wall = 50 game-seconds at 1x but ~27 game-minutes at 32x. Intents land on stale world state; today's only defenses are the immutable prompt snapshot, resolveGoal re-resolving coordinates at the landing tick (drops impossible goals), and the reflex grace floor. Nothing catches a goal that is still possible but no longer sensible, and nothing scopes WHAT the LLM may decide by how stale its answer will be. DOCTRINE CANDIDATE — the cognition horizon: don't cap speed, cap LLM authority by decision timescale. The LLM may only own decisions whose natural timescale exceeds its expected turn latency in game time at current speed; everything faster belongs to the deterministic layer permanently. IDEA MENU (for the session): (A) Staleness stamp — snapshot tick recorded on every prompt; InjectIntent computes now-snapshotTick and drops intents over a game-time staleness budget per goal class. (B) Guard re-validation — intents carry the assumptions they were formed under (target location/aliveness, not-in-emergency); executor checks guards at landing, same silent-failure contract. (C) Supersede-in-flight — per-agent generation counter bumped by high-salience stimulus; stale-generation planner results discarded on arrival. (D) Trigger-class gating — suppress trigger classes (e.g. first-adjacency encounter) whose response window is shorter than measured latency in game time. (E) Cadence scales with speed — planner cadence is tick-denominated (1800), so 32x fires 8x more wall-frequent calls than 4x, saturating the tier and worsening the very latency that causes staleness; scale cadence so wall-rate stays roughly constant (fewer, higher-altitude thoughts at high speed — fiction-coherent). (F) Future-dated prompts — tell the model when its decision will land ('it is 09:00; your decision takes effect ~09:30') so it plans for the future state. (G) Conditional plans — model returns a guarded mini-policy (goal X; if Y then Z) evaluated deterministically, buying latency tolerance. (H) Adaptive throttle — speed setting becomes a ceiling; the loop sheds speed when aggregate in-flight staleness debt exceeds budget, recovers when drained. (I) Staleness telemetry FIRST — record snapshot tick + landing tick on every injected intent so all tuning is measured, not guessed. PAUSE SEMANTICS (verified against code, 2026-07-20): mind scheduling is entirely tick-driven (replica.Tick advances only on absorbed events), so pause stops all NEW planner/musing/conversation jobs — but in-flight work is wall-clock: planner calls (180s timeout) and conversations (10-min deadline, dozens of calls) keep running while paused, and inject_intent/inject_social have NO pause gate — results land at the frozen tick (confirmed: internal/sim/loop.go command handlers, internal/mind/consolidate_test.go relies on inject-while-paused). So pause is currently 'world freezes, minds catch up' — an entire conversation scene can land instantaneously at the paused tick. The session must decide whether that's a feature (pause = zero-staleness think time, the one point where thought fidelity is perfect) or a leak (pause should freeze cognition too). Interacts with TASK-24 (tier contention worsens latency) and TASK-23 (interaction system shapes what intents exist). Output: a spec under specs/ linked to the board via spec-bridge, plus a decision doc (sibling to decision-3) stating the cognition-horizon doctrine.

Spec: specs/007-cognition-horizon
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 A design session produces a spec directory for the cognition-horizon substrate, linked on the board via spec-bridge
- [x] #2 The spec states the cognition-horizon doctrine as a decision doc (sibling to decision-3): LLM authority is scoped by decision timescale vs turn latency in game time, not by capping speed
- [x] #3 The spec defines pause semantics explicitly: what happens to in-flight planner calls and conversations when the world pauses
- [x] #4 Staleness telemetry (snapshot tick + landing tick on every injected intent) is specified as the first deliverable so tuning is measured, not guessed
- [x] #5 Spec phase: Setup
- [x] #6 Spec phase: Foundational (Blocking Prerequisites)
- [x] #7 Spec phase: User Story 1 — Staleness is measured, never guessed (Priority: P1) 🎯 MVP
- [x] #8 Spec phase: User Story 2 — Doomed thoughts are never attempted (Priority: P2)
- [x] #9 Spec phase: User Story 3 — Stale intents never act (Priority: P3)
- [ ] #10 Spec phase: User Story 4 — Thoughts aim at the world they will land in (Priority: P4)
- [ ] #11 Spec phase: User Story 5 — Pause has defined cognition semantics (Priority: P5)
- [ ] #12 Spec phase: Polish & Cross-Cutting Concerns
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1) Cut worktree ../script-world-task-32 (branch task-32-cognition-horizon) from fresh origin/main. 2) Write decision-4 (cognition-horizon doctrine) via backlog CLI. 3) Run speckit-specify for the cognition-horizon substrate spec (registry+router, calibration, landing ladder, future-dating+conditional plans, pause semantics, telemetry-first). 4) spec-bridge:link the spec dir to TASK-32. 5) Check ACs as artifacts land, sync, commit, open PR.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Pre-session decisions (user, 2026-07-20): (1) DETERMINISTIC ROUTER — the thing that decides whether a decision goes to the LLM must never be an LLM. Mechanism: intentional event-type categorization; every event/decision type carries a static complexity score on the Fibonacci scale (1,2,3,5,8,...) expressing thought cost ordinally, NOT wall-seconds — host/model-independent. A setup+calibration stage benchmarks the configured host+LLM on a uniform reference workload to derive seconds-per-point for this deployment; the scale is universal, the calibration is local. (2) STALENESS STAMP is predictive, not just forensic: 'start at T, round-trip predicted at N points ≈ Δ seconds, so plan for T+Δ and consume that context' — merges with future-dating. Open question the spec MUST answer: what happens when the prediction is drastically wrong, systemically (drift) or one-shot (network lag spike)? Drift must be accounted for regardless — proposal: calibration is not one-time; telemetry continuously re-estimates seconds-per-point with a robust/spike-rejecting estimator (EWMA over a window, outliers excluded and counted separately); sustained spike rate is itself drift signal -> recalibrate. Prediction is used for ROUTING and PROMPT future-dating only; landing-time validation is the ENFORCEMENT, so a wrong prediction can never cause a wrong action, only a wasted or adapted thought. (3) GUARD REJECTION must have a defined outcome, never a silent void: adapt -> reject+record -> learn. Adapt: cheap deterministic repair when the intent's spirit survives (re-resolve moved target within budget). Reject: fall to reflex as today BUT emit agent.intent_rejected{reason, staleness, predicted_vs_actual} — silent failure ends where telemetry doctrine begins. Learn: rejection events are classified (prediction wrong = infra signal, kept out of heuristics as spikes; world changed via high-salience interrupt = supersede working as intended); persistent guard-failure rate on a decision class means its points or budget are mistuned — surfaced for human retune, never auto-widened. (4) FUTURE-GATING accepted contingent on a real 'act at time T' mechanism existing; today intents execute at landing with no hold-until. Proposal: conditional plans subsume it — a timed guard (when tick >= T / when at location) is just one guard type in the guarded-plan vocabulary. (5) CONDITIONAL PLANS accepted. (6) ADAPTIVE THROTTLING split out to its own task (TASK-33). (7) TELEMETRY accepted; future extension noted — thought-chain graphs linking stimulus event -> thought -> intent -> executor events. Schema decision NOW, cheap now hard to retrofit: new telemetry events carry causality/correlation ids (trigger event id on prompts, snapshot+landing ticks on intents) so chains are linkable later.

Design session complete (2026-07-20): spec specs/007-cognition-horizon authored (5 prioritized stories: telemetry+calibration P1, deterministic router P2, landing ladder P3, future-dating+conditional plans P4, pause semantics P5; 20 FRs, 7 SCs; quality checklist all-pass). Doctrine recorded as decision-4. Pause decided: world freezes, minds catch up — in-flight thought lands at the frozen tick at zero game-tick staleness; no new cognition while paused; cancelling in-flight work rejected as wasteful. Spec linked via spec-bridge (marker + this note). Next: speckit-plan / speckit-tasks on this branch.

spec-bridge sync: Setup: 0/2 · Foundational (Blocking Prerequisites): 0/6 · User Story 1 — Staleness is measured, never guessed (Priority: P1) 🎯 MVP: 0/6 · User Story 2 — Doomed thoughts are never attempted (Priority: P2): 0/3 · User Story 3 — Stale intents never act (Priority: P3): 0/5 · User Story 4 — Thoughts aim at the world they will land in (Priority: P4): 0/4 · User Story 5 — Pause has defined cognition semantics (Priority: P5): 0/2 · Polish & Cross-Cutting Concerns: 0/4

spec-bridge sync: Setup: 2/2 · Foundational (Blocking Prerequisites): 6/6 · User Story 1 — Staleness is measured, never guessed (Priority: P1) 🎯 MVP: 0/6 · User Story 2 — Doomed thoughts are never attempted (Priority: P2): 0/3 · User Story 3 — Stale intents never act (Priority: P3): 0/5 · User Story 4 — Thoughts aim at the world they will land in (Priority: P4): 0/4 · User Story 5 — Pause has defined cognition semantics (Priority: P5): 0/2 · Polish & Cross-Cutting Concerns: 0/4

spec-bridge sync: Setup: 2/2 · Foundational (Blocking Prerequisites): 6/6 · User Story 1 — Staleness is measured, never guessed (Priority: P1) 🎯 MVP: 6/6 · User Story 2 — Doomed thoughts are never attempted (Priority: P2): 0/3 · User Story 3 — Stale intents never act (Priority: P3): 0/5 · User Story 4 — Thoughts aim at the world they will land in (Priority: P4): 0/4 · User Story 5 — Pause has defined cognition semantics (Priority: P5): 0/2 · Polish & Cross-Cutting Concerns: 0/4

spec-bridge sync: Setup: 2/2 · Foundational (Blocking Prerequisites): 6/6 · User Story 1 — Staleness is measured, never guessed (Priority: P1) 🎯 MVP: 6/6 · User Story 2 — Doomed thoughts are never attempted (Priority: P2): 3/3 · User Story 3 — Stale intents never act (Priority: P3): 0/5 · User Story 4 — Thoughts aim at the world they will land in (Priority: P4): 0/4 · User Story 5 — Pause has defined cognition semantics (Priority: P5): 0/2 · Polish & Cross-Cutting Concerns: 0/4

spec-bridge sync: Setup: 2/2 · Foundational (Blocking Prerequisites): 6/6 · User Story 1 — Staleness is measured, never guessed (Priority: P1) 🎯 MVP: 6/6 · User Story 2 — Doomed thoughts are never attempted (Priority: P2): 3/3 · User Story 3 — Stale intents never act (Priority: P3): 5/5 · User Story 4 — Thoughts aim at the world they will land in (Priority: P4): 0/4 · User Story 5 — Pause has defined cognition semantics (Priority: P5): 0/2 · Polish & Cross-Cutting Concerns: 0/4
<!-- SECTION:NOTES:END -->
