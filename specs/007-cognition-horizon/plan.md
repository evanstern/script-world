# Implementation Plan: The Cognition Horizon

**Branch**: `task-32-cognition-horizon` | **Date**: 2026-07-20 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/007-cognition-horizon/spec.md`

## Summary

Scope LLM authority by decision timescale vs turn latency in game time, deterministically. A new pure package `internal/cognition` owns the decision-class registry (Fibonacci points + game-tick staleness budgets + degrade actions), the seconds-per-point calibration profile (baseline from a `scriptworld calibrate` CLI stage, live-re-estimated with spike rejection from real call durations), and the pure routing function. The mind driver consults the router before enqueueing any model work; the sim loop enforces staleness and guards at the two injection doors (`inject_intent`, `inject_social`); every thought terminates in exactly one recorded outcome event carrying causality references. Prompts become future-dated and the planner vocabulary grows guarded conditional steps (timed guards subsume act-at-time-T). Pause is codified as world-freezes-minds-catch-up. Replay stays byte-identical: model output and all cognition telemetry enter deterministic space only as recorded events.

## Technical Context

**Language/Version**: Go (module `scriptworld`, toolchain per go.mod)

**Primary Dependencies**: stdlib + existing deps only (`modernc.org/sqlite` via internal/store, `anthropic-sdk-go` via internal/llm). No new external dependencies.

**Storage**: existing SQLite event log (`world.db`) for telemetry events; new `calibration.json` in the world save directory (sibling of `llm.json`, path helper on `internal/world`).

**Testing**: `go test ./...`; httptest mock providers (existing pattern) with injectable artificial latency; byte-identical replay harness (existing); e2e under `e2e/`.

**Target Platform**: the always-on daemon (darwin/linux), same as today.

**Project Type**: single Go daemon + CLI; changes span `internal/cognition` (new), `internal/mind`, `internal/sim`, `internal/llm`, `internal/world`, `internal/clock` (read-only use), `cmd/scriptworld`.

**Performance Goals**: router is a pure O(1) arithmetic check per decision (called at planner cadence, not per tick); zero measurable impact on tick throughput (~1.65M ticks/s max-speed pure-sim baseline); injection-door checks are per-command, not per-tick.

**Constraints**: byte-identical replay (model output and telemetry enter only as recorded events through the existing two injection doors); registry is static Go data validated against the world `format_version`; a model call must never block the absorb loop (existing invariant, unchanged); `SpeedMax` + LLM stays refused.

**Scale/Scope**: 8 agents, ~3,800 local calls/day at 4x today; telemetry adds ≤2 events per thought (request + outcome) — trivial for SQLite (<1M rows/30-day run today).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

`.specify/memory/constitution.md` is the unratified template — no project constitution gates exist. Applied project doctrine instead (all pass):

- **Determinism contract** (specs/001, wiki [[sim-loop]]): model-derived content enters only via `inject_intent`/`inject_social` as recorded events; router/guard/staleness verdicts are recorded, replay never re-decides. ✔ (design holds this: see research R2/R3)
- **Reflex floor** (decision under TASK-7): degrade actions land on the existing deterministic reflex layer; nothing removes it. ✔
- **decision-4 (cognition horizon)**: routing is never model-judged; budgets/points never self-adjust. ✔
- **decision-3 (strife)**: not implicated (no economy constants touched).

## Project Structure

### Documentation (this feature)

```text
specs/007-cognition-horizon/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   ├── registry.md      # Decision classes: points, budgets, degrade actions
│   ├── events.md        # New event types + canonical payloads
│   ├── calibration.md   # calibration.json schema + estimator contract
│   └── cli.md           # scriptworld calibrate command
├── checklists/requirements.md
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root)

```text
internal/cognition/        # NEW: pure, deterministic, stdlib-only
├── registry.go            # DecisionClass table, completeness check
├── route.go               # Route(class, speed, estimate) verdict — pure function
├── estimate.go            # seconds-per-point estimator: baseline + EWMA + spike rejection
└── calibration.go         # calibration.json load/save/bootstrap

internal/llm/              # measure: per-call duration → estimator samples
├── llm.go                 # Submit result carries duration; orchestrator feeds estimator
└── (no tier/queue changes)

internal/mind/             # consult: router before enqueue; emit telemetry; future-dated prompts
├── mind.go                # route planner/musing jobs; suppressed→telemetry; job snapshot carries tick/generation/trigger ref
├── convo.go               # route conversation founding (scene = one 13-point decision)
├── prompt.go              # future-dating block ("decision lands ~HH:MM")
└── parse.go               # guarded conditional plan vocabulary

internal/sim/              # enforce: staleness + guards at the doors; generation counter; plan steps
├── loop.go                # inject_intent gains snapshot_tick/generation/guards; rejection events
├── state.go               # Agent.Generation; pending guarded plan steps; reducer cases
├── reflex.go              # unchanged floor (resolveGoal shared)
└── memory.go              # generation bump on high-salience set

internal/world/            # CalibrationPath() helper
cmd/scriptworld/           # calibrate subcommand (reference workload, writes calibration.json)
e2e/                       # latency-injection scenario, pause-landing scenario, replay byte-equality
```

**Structure Decision**: one new leaf package (`internal/cognition`) with zero imports from mind/sim/llm so both the mind (routing) and the loop (budget lookup at enforcement) can depend on it without cycles; all other changes are edits inside existing packages, matching the established layout.

## Complexity Tracking

No constitution violations to justify. One deliberate scope cut: trigger-class gating (idea D from the design session) falls out of routing for free (a trigger whose class is suppressed at current speed simply doesn't enqueue) — no separate mechanism is built.
