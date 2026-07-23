# Implementation Plan: Behavioral Test Coverage for Metatron and Persona Packages

**Branch**: `task-74-metatron-persona-tests` | **Date**: 2026-07-23 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/023-metatron-persona-tests/spec.md`

## Summary

Close the verified coverage gaps in `internal/metatron` and `internal/persona` with
behavioral tests in the packages' established white-box style (scripted stubs,
temp-dir fixtures, no network, race-clean), then record the coverage in the wiki
testing-strategy note. The Phase 0 inventory (research.md R1) found the TASK-64
instruction surface already thoroughly tested; the real gaps are: metatron's
soul/transcript tail windows, the charge cap/regeneration replica seam, true
concurrent turn serialization, Observe backpressure, and the absorb-mirror
pipeline; persona's index-aligned map sweep (Anchors/DriftMarkers/Secrets),
anchor≡temperament invariant, unreadable-file load degrade, Genesis
charter/journal seeding, and SecretEvents. Tests-only: no production source
changes (FR-011).

## Technical Context

**Language/Version**: Go (repo toolchain, `go.mod`)

**Primary Dependencies**: standard library `testing` only (no testify); existing
in-package stubs `mockOrch`, `stateInjector`, `newTestAngel`, scripted `runLoop`
drivers

**Storage**: `t.TempDir()` world dirs (charter.md, skills/, metatron/soul.md,
metatron/transcript.md, agents/<name>/persona.md)

**Testing**: `go test -race ./internal/metatron/ ./internal/persona/` and the full
`go test -race ./...`

**Target Platform**: developer machines + CI (darwin/linux); tests hermetic, no
network

**Project Type**: single Go module, per-package white-box test suites

**Performance Goals**: no material suite-time regression (full suite ~3 min, e2e
dominates — the ~25 s figure cited at planning time was stale;
new tests are channel-gated, never sleep-gated)

**Constraints**: FR-011 tests-and-docs only; FR-009 behavioral (survive
behavior-preserving refactors); no new test infrastructure (R5); concurrency tests
must be deterministic under `-race` (no timing races — the TASK-69 lesson)

**Scale/Scope**: ~8 new metatron tests + ~5 new persona tests across two packages;
one wiki note update

## Constitution Check

*GATE: evaluated against constitution v1.1.0 before Phase 0; re-checked after
Phase 1 design.*

- **I. Artifact-Grounded Action** — PASS: spec/plan/tasks under
  `specs/023-metatron-persona-tests/`, linked to board TASK-74 via spec-bridge
  before implementation; gap analysis grounded in the inventoried test files.
- **II. One Task, One PR** — PASS: one branch `task-74-metatron-persona-tests` in
  `.worktrees/task-74`, one PR; spec phases are internal breakdown.
- **III. Gates Over Assertions** — PASS: board status driven by spec-bridge sync
  from artifacts; validation is `go test -race` output, not assertion.
- **IV. Grounding Freshness** — PASS: testing-strategy note update + re-pin is in
  scope (research.md R6: the only note whose subject matter this PR changes; no
  production sources touched, so no other re-pins fall due).
- **V. Model-Tiered Workflow** — PASS: this plan is planning-tier work;
  implementation delegates to the `spec-implementer` agent on **Sonnet** (routine
  tier: tests alongside code, two leaf packages, no concurrency/scheduling
  *production* logic — writing a race test does not modify governor logic).
  Escalation rubric consulted; no Opus trigger applies.

**Post-Phase-1 re-check**: PASS — design adds no projects, no new dependencies, no
production surface; Complexity Tracking empty.

## Project Structure

### Documentation (this feature)

```text
specs/023-metatron-persona-tests/
├── plan.md              # This file
├── research.md          # Phase 0: gap analysis + conventions (R1–R6)
├── data-model.md        # Phase 1: contracts under test + fixtures
├── quickstart.md        # Phase 1: validation scenarios
├── checklists/
│   └── requirements.md  # Spec quality checklist (all pass)
└── tasks.md             # Phase 2 (/speckit-tasks output)
```

### Source Code (repository root)

```text
internal/metatron/
├── metatron_test.go     # EXISTING — extended (or a sibling *_test.go added) with:
│                        #   tail-window, charge-mirror, concurrent-busy,
│                        #   observe-backpressure, absorb-mirror tests
internal/persona/
├── persona_test.go      # EXISTING — extended with: map sweep, anchor invariant,
│                        #   unreadable-load, genesis seeding, SecretEvents tests
docs/wiki/
└── testing-strategy.md  # updated narrative + sources + re-pin
```

**Structure Decision**: no new packages or directories; new tests live in the two
packages' existing white-box suites (same-package tests, per R5). Whether to
extend the existing files or add sibling `_test.go` files is the implementer's
call by size — both are conventional here.

**Contracts note**: `contracts/` is intentionally absent — the feature exposes no
new interface; the contracts under test are cataloged in data-model.md §1–2.

## Complexity Tracking

No constitution violations — table intentionally empty.
