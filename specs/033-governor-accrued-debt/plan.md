# Implementation Plan: Governor Accrued-Drift Debt

**Branch**: `task-87-governor-accrued-debt` | **Date**: 2026-07-24 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/033-governor-accrued-debt/spec.md`

## Summary

Change one arm of the governor's per-thought debt arithmetic in
`internal/cognition/governor.go` (`Debt`): the contribution becomes
`max(PredictedSec, ElapsedSec) × ticksPerSecond / BudgetTicks`, so an overdue
in-flight thought contributes its accrued, growing drift instead of flooring to
zero. Thoughts within prediction are bit-identical to today. The world-01 zero-shed
saturation shape lands as a red-first regression test; spec 028's debt doctrine text
is updated; a deliberately saturated run must show `clock.governor_shed` landing,
and the deployed world-01 binary is verified to contain the governor at all
(rebuild + restart, recorded on TASK-87).

## Technical Context

**Language/Version**: Go (module github.com/evanstern/promptworld; toolchain per go.mod)

**Primary Dependencies**: stdlib only in `internal/cognition` (leaf package
constraint); the daemon sampler (`internal/daemon/governor.go`) and pending
registry (`internal/llm/pending.go`) already provide the exact inputs — no new
plumbing

**Storage**: none; governor state is wall-side observer state (spec 028), never
persisted; shed decisions land as existing `clock.governor_shed` events

**Testing**: `go test ./...`; `internal/cognition/governor_test.go` (pure
arithmetic + hysteresis) and `internal/daemon/governor_test.go` (sampler
scenarios); red-first regression for the world-01 shape

**Target Platform**: daemon host (darwin/linux)

**Project Type**: single Go module; one-line arithmetic change + doctrine + tests

**Performance Goals**: none affected — same O(pending) sum at the same 1 s cadence

**Constraints**: deterministic pure arithmetic (FR-003 — no wall-clock reads, no
randomness, no new inputs); hysteresis constants and ladder semantics untouched
(FR-004); event shapes unchanged

**Scale/Scope**: ~1 code line + doc comments in `governor.go`; tests in 2 packages;
doctrine text in specs/028; operational verification on world-01

## Constitution Check

- **I. Artifact-Grounded Action** — PASS: defect evidence on TASK-87; this spec dir
  is the plan of record; the shed event is the run-time audit artifact; the
  binary-check result is recorded on the board task (FR-006).
- **II. One Task, One PR** — PASS: TASK-87 → `.worktrees/task-87`, branch
  `task-87-governor-accrued-debt`, one PR. Independent of TASK-86's PR #56
  (disjoint files); the board dependency is soft (protection story completeness,
  not code).
- **III. Gates Over Assertions** — PASS: spec-bridge links this dir to TASK-87;
  status follows artifacts; the red-first test is the gate for the arithmetic
  claim.
- **IV. Grounding Freshness** — PLANNED: wiki notes sourcing `governor.go`
  (cognition note and any spec-028-era notes) re-pinned post-merge (FR-007).
- **V. Model-Tiered Workflow** — PASS: planning here; implementation delegated to
  `spec-implementer` on **Opus 4.8** — governor logic in `internal/cognition` is
  explicitly a senior-tier slice (rubric: "concurrency/scheduling/governor logic",
  doctrine-adjacent). Recorded on TASK-87 at delegation.

No violations.

## Project Structure

### Documentation (this feature)

```text
specs/033-governor-accrued-debt/
├── spec.md
├── plan.md              # This file
├── research.md          # decisions: max() arm, tick-rate approximation, jobs count
├── data-model.md        # Debt input/arithmetic before/after
├── quickstart.md        # validation: unit red-test, full gates, live probe
├── contracts/
│   └── debt-formula.md  # the debt arithmetic contract (before/after, invariants)
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 (/speckit-tasks)
```

### Source Code (repository root)

```text
internal/cognition/
├── governor.go          # Debt(): overdue arm max(Predicted, Elapsed); doctrine comments
└── governor_test.go     # red-first world-01 shape; monotonic stuck-thought; bit-identical-within-prediction

internal/daemon/
└── governor_test.go     # sampler-level saturation scenario (shed within breach window)

internal/llm/
└── pending.go           # verify-only: inputs already carry PredictedSec/ElapsedSec at read time

specs/028-adaptive-throttle/
└── (spec.md / plan.md doctrine text)  # debt definition updated to accrued-drift

docs/wiki/…              # re-pin post-merge (wiki-update gate)
```

**Structure Decision**: single leaf-package arithmetic change; the daemon sampler
and pending registry are pass-throughs and need no modification (verified against
`internal/daemon/governor.go` sample() and `internal/llm/pending.go` — both already
snapshot PredictedSec/ElapsedSec at read time).

## Complexity Tracking

Not applicable — no constitution violations.
