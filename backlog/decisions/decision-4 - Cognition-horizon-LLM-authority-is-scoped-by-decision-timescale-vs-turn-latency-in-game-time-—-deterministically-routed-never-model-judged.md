---
id: decision-4
title: >-
  Cognition horizon: LLM authority is scoped by decision timescale vs turn
  latency in game time — deterministically routed, never model-judged
date: '2026-07-20 21:13'
status: proposed
---
## Context

Cognition-latency design session (user, 2026-07-20; TASK-32). The mind pipeline treats thought as instant — stimulus → LLM turn → action — but a local planner call runs ~50s wall (live finding, docs/wiki/agent-mind.md). Latency measured in *game time* scales with speed: 50s wall is 50 game-seconds of drift at 1x but ~27 game-minutes at 32x. Intents therefore land on a world that has moved on. Existing defenses (immutable prompt snapshots, coordinate re-resolution at the landing tick, the 120-tick reflex grace) catch goals that became *impossible*, but nothing catches goals that are still possible yet no longer *sensible*, and nothing scopes *what the model may decide* by how stale its answer will be. Separately, pause semantics were accidental: tick-driven scheduling stops new cognition, but in-flight calls and conversations complete on the wall clock and inject into the frozen world with no pause gate.

## Decision

**The cognition horizon.** Speed is never capped to protect cognition; cognition is scoped to survive speed. The model may only own decisions whose natural timescale exceeds its expected turn latency in game time at the current speed; everything faster belongs to the deterministic layer permanently.

- **Deterministic routing, never model-judged.** The determination of whether a decision goes to the model is made by a pure function — never by a model. Mechanism: every model-reaching decision type is registered with a thought cost in **Fibonacci points** (1, 2, 3, 5, 8, 13 — ordinal, host-independent, a property of prompt shape) and a **staleness budget in game time** (a property of the fiction). A calibration stage benchmarks the configured host+model to seconds-per-point; route to the model only if points × seconds-per-point × speed fits the budget. The scale is universal; the calibration is local.
- **Prediction is advisory; landing is authoritative.** Predictions (routing, future-dated prompts) can be wrong at the cost of a wasted thought, never a wrong action: at landing, measured staleness (game ticks actually elapsed) and deterministic guards are enforced against the current world. Calibration is continuously re-estimated with a spike-rejecting estimator — one-shot lag spikes are excluded from heuristics but counted; sustained spike rate is drift signal.
- **No silent voids.** Every requested thought terminates in exactly one recorded outcome (landed, adapted, rejected, superseded, expired, suppressed, unusable). Guard failure follows adapt → reject+record → learn; rejections are classified prediction-miss vs world-change; budgets and points are retuned by humans from telemetry, never self-adjusted.
- **Timed guards subsume scheduling.** Future-dated prompts tell the model when its decision lands; guarded conditional plans (including "at time T" guards) give act-in-the-future semantics with deterministic evaluation — no separate scheduler.
- **Pause is "world freezes, minds catch up" — by choice.** In-flight thought completes and lands at the frozen tick at zero game-tick staleness; no new cognition starts while paused. Cancelling in-flight work was considered and rejected: it discards completed thought that is, by tick arithmetic, perfectly fresh.

Spec: specs/007-cognition-horizon (TASK-32). Adaptive throttling (speed as a ceiling governed by staleness debt) is deliberately split out to TASK-33 and depends on this substrate's telemetry.

## Consequences

- Event types become intentional: a model-reaching decision type without a registry entry (points, budget, degrade action) is a startup failure, not a runtime surprise.
- The reflex layer is permanently load-bearing — it is the degrade action under every routed-away or rejected thought, at every speed.
- High speed changes what agents think about, not whether the sim is correct: at 32x the model is strategic-only (fewer, higher-altitude thoughts); at 1x it may be nearly tactical. Watchability tuning happens in budgets and cadence, not by capping speed.
- Telemetry with causality ids (stimulus → thought → intent → action) becomes part of the event schema from day one; thought-chain visualization becomes possible later without retrofit.
- Byte-identical replay is preserved: model output enters deterministic space only as recorded events; router verdicts, guards, and staleness checks are reproducible from recorded data.
- TASK-33 (adaptive throttling) and any future cognition feature must state their behavior in terms of this doctrine (points, budgets, staleness), not wall-clock seconds.
- Refinement from the live validation run (2026-07-20, qwen3:4b @ 12.3 s/pt): pause's "minds catch up" is literal — a landing batch during a pause wakes one debounce-bounded round of catch-up thought, snapshotted at the frozen tick at zero staleness, then the mind quiesces (the debounce is game-time and cannot reopen while frozen). Bounded (≤ one planner round), convergent (observed live), and maximally fresh; blessed rather than suppressed. FR-018 wording refined accordingly.
