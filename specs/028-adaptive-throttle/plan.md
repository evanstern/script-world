# Implementation Plan: Adaptive Time Throttling

**Branch**: `task-33-adaptive-throttle` | **Date**: 2026-07-23 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/028-adaptive-throttle/spec.md`

## Summary

The player's speed setting becomes a ceiling: a daemon-owned governor samples aggregate staleness debt (budget-
fraction sum over the orchestrator's pending model-bound thoughts) once per second and, through a new sim-loop
`govern` command, sheds one notch on the capped speed ladder under sustained breach and recovers notch-by-notch
under projected headroom — every move a recorded `clock.governor_*` event. `State.Speed` keeps its meaning as
"what the loop paces at" and becomes the effective speed, which makes the spec-007 router and the auto-slow
observer compose with zero changes (verified: both read `State.Speed`); an additive `RequestedSpeed` field carries
the ceiling while governed. Controller and debt math are pure functions in `internal/cognition`; the pending-
thought inventory is a mutex-guarded job registry in `internal/llm`; TUI/status surface requested-vs-effective
with the reason. No format bump (additive state field, new event types only in new logs).

## Technical Context

**Language/Version**: Go 1.26.4 (module `github.com/evanstern/promptworld`)

**Primary Dependencies**: stdlib only for all new logic; existing internal packages (`clock`, `cognition`, `sim`,
`llm`, `ipc`, `tui`, daemon wiring). No new third-party dependencies.

**Storage**: existing event log + snapshots (SQLite store); no new files; no calibration/profile changes

**Testing**: `go test ./...` with `-race` on the orchestrator registry; table-driven pure-function tests;
byte-identical replay tests (standing harness)

**Target Platform**: the existing daemon/CLI/TUI on macOS/Linux (darwin dev host)

**Project Type**: single Go project — daemon + CLI + TUI (existing layout)

**Performance Goals**: governor sampling ≤ 1 Hz with O(pending-jobs) work per sample (tens); zero overhead in
no-LLM worlds; no loop-goroutine blocking (governor enters via the non-blocking command door)

**Constraints**: byte-identical replay (FR-014/SC-001); `internal/sim` must not import `internal/llm`;
`internal/cognition` stays a leaf (may import `internal/clock` only); doctrine constants, no runtime knobs

**Scale/Scope**: ~6 packages touched; 2 new event types; 1 new state field; 3 additive status fields; no
migration

## Constitution Check

*Constitution v1.1.0. Gate: PASS (pre-Phase-0) · re-checked PASS (post-Phase-1). No Complexity Tracking entries
needed.*

- **I. Artifact-Grounded Action — PASS**: session decisions recorded on TASK-33 and in spec.md; plan/research/
  contracts/quickstart are committed files; doctrine questions were resolved FROM artifacts (spec 007, auto-slow
  precedent), not re-asked.
- **II. One Task, One PR — PASS**: implementation lands on `task-33-adaptive-throttle` in `.worktrees/task-33`
  as TASK-33's single PR; planning artifacts ride main per the spec-027 precedent; root stays on main.
- **III. Gates Over Assertions — PASS**: spec-bridge gate is green and will mirror tasks.md phases as ACs;
  status moves only via `spec-bridge:sync` from artifacts.
- **IV. Grounding Freshness — PASS (obligation scheduled)**: touched sources are listed in wiki notes
  `sim-loop`, `cognition`, `llm-orchestrator`, `game-clock` (connections), `ipc-protocol`/`ipc-server`,
  `tui-client`, `event-types` — post-merge `/grounding-wiki:wiki-update` + player-docs refresh are explicit
  polish-phase tasks.
- **V. Model-Tiered Workflow — PASS**: this plan implements nothing. Tier ruling per the rubric: the governor
  controller, debt math, orchestrator job registry (concurrency), and sim-loop command/reducer/replay slices are
  **Opus 4.8** ("concurrency/scheduling/governor logic; cross-package") ; TUI rendering, status plumbing, and
  doc reconciliation are Sonnet. Recorded on TASK-33 when implementation is dispatched.

## Project Structure

### Documentation (this feature)

```text
specs/028-adaptive-throttle/
├── plan.md              # This file
├── research.md          # Phase 0 — decisions R1–R9
├── data-model.md        # Phase 1 — state, events, registry, controller
├── quickstart.md        # Phase 1 — runnable validation scenarios
├── checklists/
│   └── requirements.md  # spec quality gate (all pass)
├── contracts/
│   ├── events.md            # clock.governor_shed / _recovered + speed_set amendment
│   ├── status-protocol.md   # Status fields + TUI surface
│   └── internal-api.md      # PendingCognition, Debt, Governor, Loop.Govern, daemon wiring
└── tasks.md             # Phase 2 (/speckit-tasks — not created by /speckit-plan)
```

### Source Code (repository root)

```text
internal/
├── cognition/
│   ├── governor.go          # NEW: Debt() + Governor state machine + doctrine constants
│   └── governor_test.go     # NEW: table-driven controller/debt tests
├── llm/
│   ├── llm.go               # job registry hooks in Submit/worker paths; PendingCognition()
│   └── pending_test.go      # NEW: registry lifecycle under -race
├── sim/
│   ├── state.go             # RequestedSpeed field; GovernorPayload; reducer arms; speed_set amendment
│   ├── loop.go              # govern command (Loop.Govern) + boundary validation
│   └── *_test.go            # reducer/command/replay coverage
├── daemon (existing daemon wiring file(s))
│   └── …                    # governor goroutine construction + GovernorSnapshot for status
├── ipc/
│   ├── protocol.go          # Status: RequestedSpeed, GovernorDebt, GovernorJobs
│   └── server.go            # fold governor snapshot into status
└── tui/
    ├── views.go             # governed header segment
    └── digest.go            # render the two new event types
```

**Structure Decision**: no new packages except code files within existing ones; `internal/cognition` gains the
one place where governor doctrine lives (beside the router it composes with), keeping the leaf-dependency rule
intact (`cognition` → `clock` only; `sim` never sees `llm`; the daemon is the sole composer — research R1).

## Complexity Tracking

No constitution violations; table intentionally empty.
