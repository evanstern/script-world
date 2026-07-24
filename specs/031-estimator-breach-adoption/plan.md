# Implementation Plan: Estimator Breach Adoption

**Branch**: `task-86-estimator-breach-adoption` | **Date**: 2026-07-24 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/031-estimator-breach-adoption/spec.md`

## Summary

Make the live seconds-per-point `Estimator` (internal/cognition/estimate.go) follow a
sustained load-induced slowdown instead of freezing: retain sample values in the
existing WindowSize ring, and on the sample that first drives the rolling spike rate
over BreachRate (the existing `cog.recalibration_recommended` condition), adopt the
window median as the new estimate, reset the window, and re-arm. The breach signal —
today an unread event — becomes the actor. The adoption's arithmetic (prior estimate,
adopted estimate) rides ADDITIVE fields on the existing
`cog.recalibration_recommended` payload, so the event catalog, reducer whitelist, and
replay compatibility are untouched. Doctrine lands in specs/007's calibration
contract; the wiki cognition note is re-pinned post-merge.

## Technical Context

**Language/Version**: Go (module github.com/evanstern/promptworld; toolchain per go.mod)

**Primary Dependencies**: stdlib only in `internal/cognition` (hard constraint — the
package is a leaf; `internal/llm`, `internal/mind`, `internal/sim` depend on it,
never the reverse)

**Storage**: none in the estimator (process-lifetime state); the adoption record
lands in the world event log (SQLite `events` table) via the existing
`Mind.emitCog` → inject_social door path

**Testing**: `go test ./...`; table-driven unit tests in
`internal/cognition/estimate_test.go`; digest catalog sweep
(`internal/tui`, TestCatalogSweep) guards event rendering; existing concurrency test
TestEstimatorSampleCountUnderConcurrency exercises the mutex path

**Target Platform**: daemon host (darwin/linux), no platform-specific code

**Project Type**: single Go module, internal package change + telemetry field addition

**Performance Goals**: Sample() stays O(WindowSize)=O(20) worst case (median over the
ring at breach only); no allocation on the non-breach hot path beyond today's

**Constraints**: deterministic (no wall-clock reads, no randomness — FR-006);
process-lifetime only (never writes calibration.json); backward-compatible event
payload (additive omitempty fields only)

**Scale/Scope**: one estimator per provider (≤ a handful); ~4 files touched in code
(`estimate.go`, `llm.go` hook plumbing, `telemetry.go` payload, `estimate_test.go`)
plus contract/doc updates

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. Artifact-Grounded Action** — PASS: defect evidence and design decision live on
  TASK-86; this spec dir is the plan of record; adoption itself emits an audit event
  (FR-005).
- **II. One Task, One PR** — PASS: TASK-86 → one branch
  (`task-86-estimator-breach-adoption` in `.worktrees/task-86`) → one PR. Spec phases
  are internal breakdown.
- **III. Gates Over Assertions** — PASS: spec-bridge links this spec to TASK-86; the
  board task's status follows the artifacts; the digest catalog sweep test is the
  gate for the payload change.
- **IV. Grounding Freshness** — PLANNED: `docs/wiki/cognition.md` lists
  `internal/cognition/estimate.go` as a source; the wiki re-pin
  (`/grounding-wiki:wiki-update`) is an explicit post-merge task, and player-docs
  freshness check follows it.
- **V. Model-Tiered Workflow** — PASS: this plan is authored on the planning tier;
  implementation is delegated to the `spec-implementer` agent. **Tier: Opus 4.8** —
  the rubric names `internal/cognition` scheduling/governor-adjacent logic and
  doctrine-adjacent behavior changes as senior-tier slices; this changes routing
  doctrine's input arithmetic. Recorded on TASK-86 at delegation time.

No violations; Complexity Tracking not needed.

## Project Structure

### Documentation (this feature)

```text
specs/031-estimator-breach-adoption/
├── spec.md              # Feature specification
├── plan.md              # This file
├── research.md          # Phase 0: decisions (adoption value, event shape, API seam)
├── data-model.md        # Phase 1: Estimator state, window ring, adoption record
├── quickstart.md        # Phase 1: how to validate (unit tests + live-world probe)
├── contracts/
│   └── adoption-event.md  # cog.recalibration_recommended payload v2 (additive)
├── checklists/
│   └── requirements.md  # Spec quality checklist (complete)
└── tasks.md             # Phase 2 output (/speckit-tasks — not created by /speckit-plan)
```

### Source Code (repository root)

```text
internal/cognition/
├── estimate.go          # Estimator: ring gains values; Sample() adopts on breach
└── estimate_test.go     # freeze-regression, adoption, one-shot-preservation tests

internal/llm/
└── llm.go               # feedEstimate: hook signature gains prior/adopted values

internal/mind/
└── telemetry.go         # RecalibrateSignal: payload gains additive adoption fields

internal/sim/
└── cognition.go         # RecalibrationPayload: additive PriorSPerPt/AdoptedSPerPt

internal/tui/
└── digest.go            # cog.recalibration_recommended renderer: show adoption

internal/daemon/
└── daemon.go            # (no change expected: hook install site keeps signature via
                         #  the mind's method — verify only)

specs/007-cognition-horizon/contracts/
└── calibration.md       # doctrine: breach-adoption semantics appended

docs/wiki/cognition.md   # re-pin post-merge (wiki-update, separate gate)
```

**Structure Decision**: single-module Go change centered on the leaf package
`internal/cognition`, with the telemetry surface following the existing
breach-signal path (`llm.feedEstimate` → mind hook → `sim.RecalibrationPayload` →
digest renderer). No new packages, no new event types.

## Complexity Tracking

Not applicable — no constitution violations.
