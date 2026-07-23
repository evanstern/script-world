# Implementation Plan: Extract the Intent-Landing Ladder into Named Rungs

**Branch**: `022-landing-ladder-rungs` | **Date**: 2026-07-23 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/022-landing-ladder-rungs/spec.md`

## Summary

The `inject_intent` case in `(*Loop).handleCommand` (internal/sim/loop.go:437-632) inlines
the whole intent-landing ladder: unavailable pre-checks, superseded/stale rejection, guard
evaluation with the TASK-47 hail relaxation spliced inside the loop, plan/goal landing, and
outcome emission — coordinated through three cross-loop flags (`adapted`, `failed`,
`hailTarget`). This plan extracts the case body into `(*Loop).landIntent` in a new
`internal/sim/landing.go`, decomposes the guard loop into named rung helpers matching the
doctrine (unavailable / superseded / stale / guard-failed / hail-relaxed / adapted / fresh),
and replaces the flag soup with an explicit decision value. Pure refactor: event kinds,
payloads, ordering, error text, and state mutations are bit-identical; the existing
determinism/replay suite is the gate, and new `landing_test.go` unit tests exercise each
rung in isolation.

## Technical Context

**Language/Version**: Go (module `github.com/evanstern/promptworld`; toolchain per go.mod)

**Primary Dependencies**: standard library + internal packages only (`internal/sim`,
`internal/store`, `internal/cognition`, `internal/sim/tool` alias `tool`)

**Storage**: event log via `l.st.AppendEvents` — untouched; no schema change permitted

**Testing**: `go test -race ./...`; determinism gate = existing same-seed timeline test
(`TestDeterminismSameSeedSameTimeline`), replay byte-identity tests
(`TestReplayByteIdentity*`, `TestReplayDeterminismWithHails`, …) — all must pass unedited

**Target Platform**: same as project (daemon on macOS/Linux); no platform change

**Project Type**: single Go module, internal refactor confined to `internal/sim`

**Performance Goals**: no regression on the command path; extraction is call-structure
only (no new allocations in the hot loop beyond one small decision struct)

**Constraints**: behavior bit-identical — same events (kind, payload fields, ordering),
same error strings, same state mutations, same rejection/err pairing (rejected
inject_intent both emits and errors); no event-schema, doctrine-constant, or guard
vocabulary change

**Scale/Scope**: one case body (~195 lines) → 1 new file + 1 new test file + a shrunk
switch case; no other call sites (ladder logic exists only here)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. Artifact-Grounded Action — PASS**: spec/plan/tasks under `specs/022-landing-ladder-rungs/`;
  board task TASK-70 linked via spec-bridge before implementation; tier choice recorded on
  the task.
- **II. One Task, One PR — PASS**: TASK-70 → one branch (`task-70-landing-ladder-rungs`) in
  `.worktrees/task-70`, one PR; spec phases are internal breakdown.
- **III. Gates Over Assertions — PASS**: determinism suite + `go test -race` are the
  physical gate; spec-bridge gate keeps board status ≤ artifacts.
- **IV. Grounding Freshness — PASS (planned)**: `docs/wiki/sim-loop.md` lists loop.go as a
  source; re-pin via `/grounding-wiki:wiki-update` after merge is FR-009 / board AC #4.
- **V. Model-Tiered Workflow — PASS**: planning on Fable 5; implementation delegated to the
  `spec-implementer` agent at **Opus 4.8** — rubric: core sim-loop change, doctrine-adjacent
  (the ladder IS doctrine enforcement), review-flagged worst complexity hotspot where a
  behavioral slip ships a live defect the determinism gate may only catch after the fact.
  Justification to be recorded on TASK-70.

**Post-Phase-1 re-check**: design adds one file and one decision type inside `internal/sim`
— no new projects, patterns, or dependencies. No Complexity Tracking entries needed.

## Project Structure

### Documentation (this feature)

```text
specs/022-landing-ladder-rungs/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

(`contracts/` intentionally omitted: internal refactor with an explicit no-interface-change
constraint — the determinism/replay suite is the behavioral contract; see research.md D5.)

### Source Code (repository root)

```text
internal/sim/
├── loop.go          # inject_intent case shrinks to: validate index → l.landIntent(in, emit)
├── landing.go       # NEW: landIntent + named rung helpers + landingDecision type
├── landing_test.go  # NEW: per-rung unit tests incl. hail special-cases
├── hail.go          # untouched (hailable/hailWindowTicks consumed as-is)
├── guard.go         # untouched
└── cognition.go     # untouched (Outcome* constants consumed as-is)
```

**Structure Decision**: single new source file + test file inside `internal/sim`, keeping
the extraction package-local so rungs can remain unexported and tests live alongside the
existing sim test conventions (state builders in `*_test.go`).

## Design sketch (Phase 1 input)

- `(*Loop).landIntent(in InjectIntent-args, emit func(string, any)) error` — the whole
  former case body except the agent-index bounds check decision (kept identical wherever it
  lands; see research D2).
- `landingDecision` value: `{outcome string, reason string, adapted bool→folded into outcome,
  hailTarget int}` — explicit result consumed in exactly one place; no cross-rung flags.
- Named rungs (unexported funcs, doctrine names):
  - `rungUnavailable` — dead/asleep pre-checks (ordering preserved: dead, then asleep)
  - `rungSuperseded` — generation mismatch
  - `rungStale` — staleness vs class budget
  - `rungGuards` — the guard walk; per-guard sub-rungs:
    - `rungHailRelaxed` — failing target_present on talk_to, alive target:
      mutual-hailer sub-case (adapted, no hail) then `hailable` sub-case (adapted + hail)
    - `rungGuardFailed` — plain rejection with Eval's reason
    - `rungAdapted` — holding target_present whose target moved from (g.X, g.Y)
    - in-radius hail marking — holding target_present, talk_to, hailable
  - fresh = the fall-through outcome (`OutcomeLanded`)
- Emission (reject events, plan/goal landing, final cog.outcome, social.hailed) stays in
  `landIntent`, driven by the decision value — same events, same order, same err pairing.

## Complexity Tracking

No constitution violations — table not needed.
